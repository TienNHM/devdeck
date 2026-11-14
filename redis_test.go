package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
)

func redisTestServer(t *testing.T) (*miniredis.Miniredis, *http.ServeMux) {
	t.Helper()
	mr := miniredis.RunT(t)
	mr.Set("user:1:name", "alice")
	mr.Set("user:2:name", `{"a":[1,2,{"b":null}]}`)
	mr.HSet("user:1:profile", "age", "30")
	mr.Lpush("queue:jobs", "j1")
	mr.SetAdd("tags", "x", "y")
	mr.ZAdd("scores", 1.5, "bob")
	mux := http.NewServeMux()
	registerRedisRoutes(mux)
	return mr, mux
}

func doJSON(t *testing.T, mux *http.ServeMux, method, path string, body any) (*httptest.ResponseRecorder, map[string]any) {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		_ = json.NewEncoder(&buf).Encode(body)
	}
	req := httptest.NewRequest(method, path, &buf)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	var m map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &m)
	return rec, m
}

func openRedis(t *testing.T, mux *http.ServeMux, addr string, write bool) string {
	t.Helper()
	rec, m := doJSON(t, mux, "POST", "/api/redis/open", map[string]any{"addr": addr, "write": write})
	if rec.Code != 200 {
		t.Fatalf("open: %d %s", rec.Code, rec.Body)
	}
	if m["write"] != write {
		t.Fatalf("write flag not echoed: %v", m)
	}
	id := m["id"].(string)
	t.Cleanup(func() { rconns.close(id) })
	return id
}

var allWriteOps = []map[string]any{
	{"op": "set", "key": "user:1:name", "value": "bob"},
	{"op": "set", "key": "newkey", "value": "v", "create": true},
	{"op": "expire", "key": "user:1:name", "ttl": 60},
	{"op": "expire", "key": "user:1:name", "ttl": 0},
	{"op": "rename", "key": "tags", "newKey": "tags2"},
	{"op": "hset", "key": "user:1:profile", "field": "city", "value": "Hue"},
	{"op": "hdel", "key": "user:1:profile", "field": "age"},
	{"op": "lpush", "key": "queue:jobs", "value": "j2", "side": "left"},
	{"op": "lpush", "key": "queue:jobs", "value": "j3", "side": "right"},
	{"op": "lrem", "key": "queue:jobs", "value": "j1"},
	{"op": "sadd", "key": "tags2", "value": "z"},
	{"op": "srem", "key": "tags2", "value": "x"},
	{"op": "zadd", "key": "scores", "value": "carol", "score": 2},
	{"op": "zrem", "key": "scores", "value": "bob"},
	{"op": "del", "keys": []string{"user:2:name", "missing"}},
}

func TestRedisWriteRejectedOnReadOnlyConnection(t *testing.T) {
	mr, mux := redisTestServer(t)
	id := openRedis(t, mux, mr.Addr(), false)
	before := mr.Keys()
	for _, op := range allWriteOps {
		body := map[string]any{"id": id}
		for k, v := range op {
			body[k] = v
		}
		rec, m := doJSON(t, mux, "POST", "/api/redis/write", body)
		if rec.Code != http.StatusForbidden {
			t.Fatalf("op %v on read-only connection: got %d %v, want 403", op["op"], rec.Code, m)
		}
	}
	if v, _ := mr.Get("user:1:name"); v != "alice" {
		t.Fatalf("value changed through a read-only connection: %q", v)
	}
	if len(mr.Keys()) != len(before) {
		t.Fatalf("keys changed through a read-only connection")
	}
	// the legacy connect route must never grant write mode, even if asked
	var req redisConnReq
	_ = json.Unmarshal([]byte(`{"addr":"x","write":true}`), &req)
	if req.Write {
		t.Fatal("legacy redisConnReq accepted write=true from JSON")
	}
}

func TestRedisWriteOps(t *testing.T) {
	mr, mux := redisTestServer(t)
	id := openRedis(t, mux, mr.Addr(), true)
	for _, op := range allWriteOps {
		body := map[string]any{"id": id}
		for k, v := range op {
			body[k] = v
		}
		rec, m := doJSON(t, mux, "POST", "/api/redis/write", body)
		if rec.Code != 200 {
			t.Fatalf("op %v: %d %v", op["op"], rec.Code, m)
		}
	}
	if v, _ := mr.Get("user:1:name"); v != "bob" {
		t.Errorf("set: got %q", v)
	}
	if mr.TTL("user:1:name") != 0 {
		t.Errorf("persist: ttl still %v", mr.TTL("user:1:name"))
	}
	if mr.Exists("tags") || !mr.Exists("tags2") {
		t.Errorf("rename failed")
	}
	if ok, _ := mr.SIsMember("tags2", "z"); !ok {
		t.Errorf("sadd failed")
	}
	if mr.HGet("user:1:profile", "city") != "Hue" || mr.HGet("user:1:profile", "age") != "" {
		t.Errorf("hset/hdel failed")
	}
	if l, _ := mr.List("queue:jobs"); len(l) != 2 || l[0] != "j2" || l[1] != "j3" {
		t.Errorf("list ops: %v", l)
	}
	if ms, _ := mr.ZMembers("scores"); len(ms) != 1 || ms[0] != "carol" {
		t.Errorf("zset ops: %v", ms)
	}
	if mr.Exists("user:2:name") {
		t.Errorf("del failed")
	}

	// guard rails
	cases := []map[string]any{
		{"op": "set", "key": "newkey", "value": "again", "create": true}, // exists
		{"op": "rename", "key": "user:1:name", "newKey": "newkey"},       // target exists
		{"op": "set", "key": "scores", "value": "x"},                     // wrong type
		{"op": "hset", "key": "user:1:name", "field": "f", "value": "v"}, // wrong type
		{"op": "nope", "key": "a"},
		{"op": "del"},
	}
	for _, c := range cases {
		c["id"] = id
		if rec, _ := doJSON(t, mux, "POST", "/api/redis/write", c); rec.Code != 400 {
			t.Errorf("%v: got %d, want 400", c, rec.Code)
		}
	}
	// set keeps an existing TTL
	mr.SetTTL("newkey", time.Hour)
	doJSON(t, mux, "POST", "/api/redis/write", map[string]any{"id": id, "op": "set", "key": "newkey", "value": "kept"})
	if mr.TTL("newkey") <= 0 {
		t.Errorf("set dropped the TTL")
	}
}

func TestRedisIdleCloseReconnects(t *testing.T) {
	mr, mux := redisTestServer(t)
	id := openRedis(t, mux, mr.Addr(), false)
	s, err := rconns.session(id)
	if err != nil {
		t.Fatal(err)
	}
	s.closeIdle() // what the idle timer does
	c, err := rconns.get(id)
	if err != nil {
		t.Fatal(err)
	}
	kv, err := redisGet(c, "user:1:name")
	if err != nil || kv.Value != "alice" {
		t.Fatalf("after idle close: %v %v", kv, err)
	}
}

func TestRedisProfilesNoPassword(t *testing.T) {
	t.Setenv("USERPROFILE", t.TempDir())
	t.Setenv("HOME", t.TempDir())
	_, mux := redisTestServer(t)
	rec, _ := doJSON(t, mux, "PUT", "/api/redis/profiles", []map[string]any{{"name": "local", "addr": "127.0.0.1:6379", "db": 2, "password": "secret"}})
	if rec.Code != 200 {
		t.Fatalf("save: %d %s", rec.Code, rec.Body)
	}
	if bytes.Contains(rec.Body.Bytes(), []byte("secret")) {
		t.Fatal("password echoed back")
	}
	ps := loadRedisProfiles()
	if len(ps) != 1 || ps[0].DB != 2 {
		t.Fatalf("profiles: %+v", ps)
	}
	if rec, _ := doJSON(t, mux, "PUT", "/api/redis/profiles", []map[string]any{{"name": "a"}, {"name": "A"}}); rec.Code != 400 {
		t.Fatal("duplicate names accepted")
	}
}

func TestParseRedisInfo(t *testing.T) {
	s := "# Server\r\nredis_version:7.2.4\r\nredis_mode:standalone\r\nuptime_in_seconds:100\r\n# Clients\r\nconnected_clients:3\r\n" +
		"# Memory\r\nused_memory:1048576\r\nused_memory_human:1.00M\r\nused_memory_peak:2097152\r\nused_memory_peak_human:2.00M\r\n" +
		"# Stats\r\ninstantaneous_ops_per_sec:42\r\nkeyspace_hits:75\r\nkeyspace_misses:25\r\n# Replication\r\nrole:master\r\n" +
		"# Keyspace\r\ndb0:keys=10,expires=2,avg_ttl=500\r\ndb3:keys=1,expires=0,avg_ttl=0\r\n"
	st := parseRedisInfo(s)
	if st.Version != "7.2.4" || st.Clients != 3 || st.OpsPerSec != 42 || st.UsedMemoryH != "1.00M" || st.PeakMemory != 2097152 || st.Role != "master" {
		t.Fatalf("parse: %+v", st)
	}
	if st.HitRatio != 0.75 {
		t.Fatalf("hit ratio %v", st.HitRatio)
	}
	if len(st.Keyspace) != 2 || st.Keyspace[1].DB != 3 || st.Keyspace[0].Keys != 10 || st.Keyspace[0].Expires != 2 {
		t.Fatalf("keyspace %+v", st.Keyspace)
	}
	if parseRedisInfo("").HitRatio != -1 {
		t.Fatal("no lookups should give -1")
	}
}
