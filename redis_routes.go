package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

// New Redis endpoints: saved profiles, connect with optional write mode, server stats and writes.
// The legacy /api/redis/{connect,close,scan,key,info} routes stay in server.go.

func init() { addRoutes(registerRedisRoutes) }

// redisProfile is a saved connection. It never holds a password.
type redisProfile struct {
	Name     string `json:"name"`
	Addr     string `json:"addr"`
	DB       int    `json:"db"`
	TLS      bool   `json:"tls"`
	Username string `json:"username,omitempty"`
}

type redisProfileStore struct {
	Profiles []redisProfile `json:"profiles"`
}

func loadRedisProfiles() []redisProfile {
	var s redisProfileStore
	_ = loadJSON("redis", &s)
	if s.Profiles == nil {
		s.Profiles = []redisProfile{}
	}
	return s.Profiles
}

func cleanRedisProfiles(in []redisProfile) ([]redisProfile, error) {
	if len(in) > 100 {
		return nil, errors.New("too many profiles (max 100)")
	}
	seen := map[string]bool{}
	out := make([]redisProfile, 0, len(in))
	for _, p := range in {
		p.Name = strings.TrimSpace(p.Name)
		p.Addr = strings.TrimSpace(p.Addr)
		p.Username = strings.TrimSpace(p.Username)
		if p.Name == "" || len(p.Name) > 60 {
			return nil, errors.New("profile name must be 1-60 characters")
		}
		if seen[strings.ToLower(p.Name)] {
			return nil, fmt.Errorf("duplicate profile name %q", p.Name)
		}
		seen[strings.ToLower(p.Name)] = true
		if p.Addr == "" {
			p.Addr = "127.0.0.1:6379"
		}
		if len(p.Addr) > 255 || strings.ContainsAny(p.Addr, " \t\r\n/") {
			return nil, fmt.Errorf("bad address for %q", p.Name)
		}
		if p.DB < 0 || p.DB > 1024 {
			return nil, fmt.Errorf("bad database index for %q", p.Name)
		}
		if len(p.Username) > 128 {
			return nil, fmt.Errorf("username too long for %q", p.Name)
		}
		out = append(out, p)
	}
	return out, nil
}

// redisOpenReq extends the legacy connect body with the write-mode switch.
type redisOpenReq struct {
	redisConnReq
	Write bool `json:"write"`
}

func registerRedisRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/redis/profiles", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, loadRedisProfiles())
	})
	mux.HandleFunc("PUT /api/redis/profiles", func(w http.ResponseWriter, r *http.Request) {
		var in []redisProfile
		if err := decode(r, &in); err != nil {
			fail(w, 400, err)
			return
		}
		ps, err := cleanRedisProfiles(in)
		if err != nil {
			fail(w, 400, err)
			return
		}
		if err := saveJSON("redis", redisProfileStore{Profiles: ps}); err != nil {
			fail(w, 500, err)
			return
		}
		writeJSON(w, ps)
	})
	mux.HandleFunc("POST /api/redis/open", func(w http.ResponseWriter, r *http.Request) {
		var req redisOpenReq
		if err := decode(r, &req); err != nil {
			fail(w, 400, err)
			return
		}
		req.redisConnReq.Write = req.Write
		id, ver, err := rconns.connect(req.redisConnReq)
		if err != nil {
			fail(w, 400, err)
			return
		}
		writeJSON(w, map[string]any{"id": id, "version": ver, "write": req.Write})
	})
	mux.HandleFunc("GET /api/redis/stats", func(w http.ResponseWriter, r *http.Request) {
		c, err := rconns.get(r.URL.Query().Get("id"))
		if err != nil {
			fail(w, 400, err)
			return
		}
		st, err := redisStats(c)
		if err != nil {
			fail(w, 500, err)
			return
		}
		writeJSON(w, st)
	})
	mux.HandleFunc("POST /api/redis/write", func(w http.ResponseWriter, r *http.Request) {
		var req redisWriteReq
		if err := decode(r, &req); err != nil {
			fail(w, 400, err)
			return
		}
		c, err := rconns.getW(req.ID)
		if errors.Is(err, errRedisReadOnly) {
			fail(w, http.StatusForbidden, err)
			return
		}
		if err != nil {
			fail(w, 400, err)
			return
		}
		res, err := redisWrite(c, req)
		if err != nil {
			fail(w, 400, err)
			return
		}
		writeJSON(w, res)
	})
}

// ------------------------------------------------------------------ stats

type redisKeyspace struct {
	DB      int   `json:"db"`
	Keys    int64 `json:"keys"`
	Expires int64 `json:"expires"`
	AvgTTL  int64 `json:"avgTtl"`
}

type RedisStats struct {
	Version       string          `json:"version"`
	Mode          string          `json:"mode"`
	Role          string          `json:"role"`
	OS            string          `json:"os"`
	UptimeSec     int64           `json:"uptimeSec"`
	UsedMemory    int64           `json:"usedMemory"`
	UsedMemoryH   string          `json:"usedMemoryHuman"`
	PeakMemory    int64           `json:"peakMemory"`
	PeakMemoryH   string          `json:"peakMemoryHuman"`
	MaxMemory     int64           `json:"maxMemory"`
	Fragmentation float64         `json:"fragmentation"`
	Clients       int64           `json:"clients"`
	BlockedClient int64           `json:"blockedClients"`
	OpsPerSec     int64           `json:"opsPerSec"`
	TotalCommands int64           `json:"totalCommands"`
	Hits          int64           `json:"hits"`
	Misses        int64           `json:"misses"`
	HitRatio      float64         `json:"hitRatio"` // 0..1, -1 when there were no lookups
	Keyspace      []redisKeyspace `json:"keyspace"`
	Raw           string          `json:"raw"`
}

func redisStats(c *redis.Client) (*RedisStats, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
	defer cancel()
	// Plain INFO (default sections) works on every Redis version; multi-section INFO needs 7+.
	s, err := c.Info(ctx).Result()
	if err != nil {
		return nil, err
	}
	return parseRedisInfo(s), nil
}

func parseRedisInfo(s string) *RedisStats {
	kv := map[string]string{}
	st := &RedisStats{Keyspace: []redisKeyspace{}, HitRatio: -1}
	for _, l := range strings.Split(s, "\n") {
		l = strings.TrimSpace(l)
		if l == "" || l[0] == '#' {
			continue
		}
		k, v, ok := strings.Cut(l, ":")
		if !ok {
			continue
		}
		if strings.HasPrefix(k, "db") {
			if n, err := strconv.Atoi(k[2:]); err == nil {
				ks := redisKeyspace{DB: n}
				for _, part := range strings.Split(v, ",") {
					pk, pv, _ := strings.Cut(part, "=")
					x, _ := strconv.ParseInt(pv, 10, 64)
					switch pk {
					case "keys":
						ks.Keys = x
					case "expires":
						ks.Expires = x
					case "avg_ttl":
						ks.AvgTTL = x
					}
				}
				st.Keyspace = append(st.Keyspace, ks)
				continue
			}
		}
		kv[k] = v
	}
	n := func(k string) int64 { x, _ := strconv.ParseInt(kv[k], 10, 64); return x }
	st.Version = kv["redis_version"]
	st.Mode = kv["redis_mode"]
	st.Role = kv["role"]
	st.OS = kv["os"]
	st.UptimeSec = n("uptime_in_seconds")
	st.UsedMemory = n("used_memory")
	st.UsedMemoryH = kv["used_memory_human"]
	st.PeakMemory = n("used_memory_peak")
	st.PeakMemoryH = kv["used_memory_peak_human"]
	st.MaxMemory = n("maxmemory")
	st.Fragmentation, _ = strconv.ParseFloat(kv["mem_fragmentation_ratio"], 64)
	st.Clients = n("connected_clients")
	st.BlockedClient = n("blocked_clients")
	st.OpsPerSec = n("instantaneous_ops_per_sec")
	st.TotalCommands = n("total_commands_processed")
	st.Hits = n("keyspace_hits")
	st.Misses = n("keyspace_misses")
	if t := st.Hits + st.Misses; t > 0 {
		st.HitRatio = float64(st.Hits) / float64(t)
	}
	if len(s) > 32*1024 {
		s = s[:32*1024]
	}
	st.Raw = s
	return st
}

// ------------------------------------------------------------------ writes

type redisWriteReq struct {
	ID     string   `json:"id"`
	Op     string   `json:"op"`
	Key    string   `json:"key"`
	Keys   []string `json:"keys"`
	NewKey string   `json:"newKey"`
	Field  string   `json:"field"`
	Value  string   `json:"value"`
	Score  float64  `json:"score"`
	TTL    int64    `json:"ttl"`    // seconds; <= 0 removes the expiry
	Side   string   `json:"side"`   // "left" | "right" for list push
	Count  int64    `json:"count"`  // LREM count
	Create bool     `json:"create"` // op=set: fail if the key already exists
}

const maxDeleteKeys = 1000

// redisWrite performs one mutation. Callers must only pass clients of write-mode connections.
func redisWrite(c *redis.Client, q redisWriteReq) (map[string]any, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if q.Op != "del" && q.Key == "" {
		return nil, errors.New("key is required")
	}
	var n int64
	var err error
	switch q.Op {
	case "set":
		if q.Create {
			var ok bool
			ok, err = c.SetNX(ctx, q.Key, q.Value, 0).Result()
			if err == nil && !ok {
				err = fmt.Errorf("key %q already exists", q.Key)
			}
		} else {
			// KEEPTTL keeps an existing expiry; type mismatches are overwritten by SET by design, so refuse them.
			var typ string
			typ, err = c.Type(ctx, q.Key).Result()
			if err == nil && typ != "string" && typ != "none" {
				return nil, fmt.Errorf("key %q is a %s, not a string", q.Key, typ)
			}
			if err == nil {
				err = c.SetArgs(ctx, q.Key, q.Value, redis.SetArgs{KeepTTL: true}).Err()
			}
		}
		n = 1
	case "expire":
		var ok bool
		if q.TTL > 0 {
			ok, err = c.Expire(ctx, q.Key, time.Duration(q.TTL)*time.Second).Result()
		} else {
			ok, err = c.Persist(ctx, q.Key).Result()
		}
		if ok {
			n = 1
		}
	case "rename":
		if q.NewKey == "" {
			return nil, errors.New("new key name is required")
		}
		var ok bool
		ok, err = c.RenameNX(ctx, q.Key, q.NewKey).Result()
		if err == nil && !ok {
			err = fmt.Errorf("key %q already exists", q.NewKey)
		}
		n = 1
	case "del":
		keys := q.Keys
		if q.Key != "" {
			keys = append(keys, q.Key)
		}
		if len(keys) == 0 {
			return nil, errors.New("no keys given")
		}
		if len(keys) > maxDeleteKeys {
			return nil, fmt.Errorf("too many keys (max %d per request)", maxDeleteKeys)
		}
		n, err = c.Del(ctx, keys...).Result()
	case "hset":
		if q.Field == "" {
			return nil, errors.New("field is required")
		}
		err = redisCheckType(ctx, c, q.Key, "hash")
		if err == nil {
			n, err = c.HSet(ctx, q.Key, q.Field, q.Value).Result()
		}
	case "hdel":
		n, err = c.HDel(ctx, q.Key, q.Field).Result()
	case "lpush":
		err = redisCheckType(ctx, c, q.Key, "list")
		if err == nil {
			if q.Side == "left" {
				n, err = c.LPush(ctx, q.Key, q.Value).Result()
			} else {
				n, err = c.RPush(ctx, q.Key, q.Value).Result()
			}
		}
	case "lrem":
		cnt := q.Count
		if cnt == 0 {
			cnt = 1
		}
		n, err = c.LRem(ctx, q.Key, cnt, q.Value).Result()
	case "sadd":
		err = redisCheckType(ctx, c, q.Key, "set")
		if err == nil {
			n, err = c.SAdd(ctx, q.Key, q.Value).Result()
		}
	case "srem":
		n, err = c.SRem(ctx, q.Key, q.Value).Result()
	case "zadd":
		err = redisCheckType(ctx, c, q.Key, "zset")
		if err == nil {
			n, err = c.ZAdd(ctx, q.Key, redis.Z{Score: q.Score, Member: q.Value}).Result()
		}
	case "zrem":
		n, err = c.ZRem(ctx, q.Key, q.Value).Result()
	default:
		return nil, fmt.Errorf("unknown write op %q", q.Op)
	}
	if err != nil {
		return nil, err
	}
	return map[string]any{"ok": true, "n": n}, nil
}

// redisCheckType refuses to add members to a key of another type (Redis would error with WRONGTYPE anyway,
// but this gives a readable message). A missing key is fine: it is created.
func redisCheckType(ctx context.Context, c *redis.Client, key, want string) error {
	typ, err := c.Type(ctx, key).Result()
	if err != nil {
		return err
	}
	if typ != want && typ != "none" {
		return fmt.Errorf("key %q is a %s, not a %s", key, typ, want)
	}
	return nil
}
