package main

import (
	"context"
	"crypto/rand"
	"crypto/tls"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/redis/go-redis/v9"
)

// Redis access is read-only unless a connection was opened with write mode (see redis_write.go):
// read paths only issue SCAN, TYPE, TTL and read commands.

type redisConnReq struct {
	Addr     string `json:"addr"`
	Username string `json:"username,omitempty"`
	Password string `json:"password"`
	DB       int    `json:"db"`
	TLS      bool   `json:"tls"`
	// Write is never taken from the legacy /api/redis/connect body; only /api/redis/open sets it.
	Write bool `json:"-"`
}

const (
	redisIdleClose   = 5 * time.Minute // close sockets after this much inactivity (re-dialled lazily)
	redisSessionTTL  = 12 * time.Hour  // forget the session (and its in-memory password) after this
	maxRedisSessions = 16
)

// rconn is one browser connection. The password lives only here, in memory.
type rconn struct {
	opt   redisConnReq
	write bool
	ver   string
	mu    sync.Mutex
	c     *redis.Client
	idle  *time.Timer
	used  time.Time
}

func (s *rconn) client() *redis.Client {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.c == nil {
		s.c = newRedisClient(s.opt)
	}
	s.used = time.Now()
	s.idle.Reset(redisIdleClose)
	return s.c
}

func (s *rconn) closeIdle() {
	s.mu.Lock()
	c := s.c
	s.c = nil
	s.mu.Unlock()
	if c != nil {
		_ = c.Close()
	}
}

func (s *rconn) shutdown() {
	s.idle.Stop()
	s.closeIdle()
}

type redisConns struct {
	mu sync.Mutex
	m  map[string]*rconn
}

var rconns = &redisConns{m: map[string]*rconn{}}

var errRedisReadOnly = errors.New("this connection is read-only; reconnect with write mode enabled to change data")

func newRedisClient(r redisConnReq) *redis.Client {
	opt := &redis.Options{
		Addr: r.Addr, Username: r.Username, Password: r.Password, DB: r.DB,
		DialTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: 10 * time.Second,
		PoolSize: 2, MinIdleConns: 0, ConnMaxIdleTime: redisIdleClose,
	}
	if r.TLS {
		opt.TLSConfig = &tls.Config{MinVersion: tls.VersionTLS12}
	}
	return redis.NewClient(opt)
}

func (rc *redisConns) connect(r redisConnReq) (string, string, error) {
	if r.Addr == "" {
		r.Addr = "127.0.0.1:6379"
	}
	if r.DB < 0 || r.DB > 1024 {
		return "", "", errors.New("database index out of range")
	}
	cl := newRedisClient(r)
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
	defer cancel()
	if err := cl.Ping(ctx).Err(); err != nil {
		_ = cl.Close()
		return "", "", err
	}
	info, _ := cl.Info(ctx, "server").Result()
	ver := ""
	for _, l := range strings.Split(info, "\n") {
		if strings.HasPrefix(l, "redis_version:") {
			ver = strings.TrimSpace(strings.TrimPrefix(l, "redis_version:"))
		}
	}
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	id := hex.EncodeToString(b)
	s := &rconn{opt: r, write: r.Write, ver: ver, c: cl, used: time.Now()}
	s.idle = time.AfterFunc(redisIdleClose, s.closeIdle)
	rc.mu.Lock()
	rc.sweepLocked()
	rc.m[id] = s
	rc.mu.Unlock()
	return id, ver, nil
}

// sweepLocked drops expired sessions and, if still full, the least recently used one. No background goroutine.
func (rc *redisConns) sweepLocked() {
	var oldest string
	var oldestT time.Time
	for id, s := range rc.m {
		s.mu.Lock()
		used := s.used
		s.mu.Unlock()
		if time.Since(used) > redisSessionTTL {
			delete(rc.m, id)
			s.shutdown()
			continue
		}
		if oldest == "" || used.Before(oldestT) {
			oldest, oldestT = id, used
		}
	}
	if len(rc.m) >= maxRedisSessions && oldest != "" {
		rc.m[oldest].shutdown()
		delete(rc.m, oldest)
	}
}

func (rc *redisConns) session(id string) (*rconn, error) {
	rc.mu.Lock()
	defer rc.mu.Unlock()
	s := rc.m[id]
	if s == nil {
		return nil, errors.New("unknown redis connection; connect again")
	}
	s.mu.Lock()
	expired := time.Since(s.used) > redisSessionTTL
	s.mu.Unlock()
	if expired {
		delete(rc.m, id)
		s.shutdown()
		return nil, errors.New("redis connection expired; connect again")
	}
	return s, nil
}

func (rc *redisConns) get(id string) (*redis.Client, error) {
	s, err := rc.session(id)
	if err != nil {
		return nil, err
	}
	return s.client(), nil
}

// getW returns the client only for connections opened in write mode.
func (rc *redisConns) getW(id string) (*redis.Client, error) {
	s, err := rc.session(id)
	if err != nil {
		return nil, err
	}
	if !s.write {
		return nil, errRedisReadOnly
	}
	return s.client(), nil
}

func (rc *redisConns) close(id string) {
	rc.mu.Lock()
	s := rc.m[id]
	delete(rc.m, id)
	rc.mu.Unlock()
	if s != nil {
		s.shutdown()
	}
}

type KeyEntry struct {
	Key  string `json:"key"`
	Type string `json:"type"`
	TTL  int64  `json:"ttl"` // seconds, -1 no expiry, -2 missing
}

type ScanResult struct {
	Cursor uint64     `json:"cursor,string"`
	Keys   []KeyEntry `json:"keys"`
}

func redisScan(c *redis.Client, cursor uint64, match string, count int64) (*ScanResult, error) {
	if match == "" {
		match = "*"
	}
	if count <= 0 || count > 1000 {
		count = 200
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	keys, next, err := c.Scan(ctx, cursor, match, count).Result()
	if err != nil {
		return nil, err
	}
	pipe := c.Pipeline()
	types := make([]*redis.StatusCmd, len(keys))
	ttls := make([]*redis.DurationCmd, len(keys))
	for i, k := range keys {
		types[i] = pipe.Type(ctx, k)
		ttls[i] = pipe.TTL(ctx, k)
	}
	_, _ = pipe.Exec(ctx)
	res := &ScanResult{Cursor: next}
	for i, k := range keys {
		ttl := int64(-1)
		if d := ttls[i].Val(); d >= 0 {
			ttl = int64(d / time.Second)
		} else if d == -2 {
			ttl = -2
		}
		res.Keys = append(res.Keys, KeyEntry{Key: k, Type: types[i].Val(), TTL: ttl})
	}
	return res, nil
}

type KeyValue struct {
	Key       string      `json:"key"`
	Type      string      `json:"type"`
	TTL       int64       `json:"ttl"`
	Size      int64       `json:"size"`
	Value     interface{} `json:"value"`
	Truncated bool        `json:"truncated"`
	Binary    bool        `json:"binary"`
}

const (
	maxStringBytes = 64 * 1024
	maxItems       = 200
)

func display(s string) (string, bool) {
	if utf8.ValidString(s) {
		return s, false
	}
	return fmt.Sprintf("%q", s), true
}

func redisGet(c *redis.Client, key string) (*KeyValue, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	typ, err := c.Type(ctx, key).Result()
	if err != nil {
		return nil, err
	}
	kv := &KeyValue{Key: key, Type: typ, TTL: -1}
	if d, err := c.TTL(ctx, key).Result(); err == nil {
		if d >= 0 {
			kv.TTL = int64(d / time.Second)
		} else if d == -2 {
			kv.TTL = -2
		}
	}
	switch typ {
	case "none":
		return kv, nil
	case "string":
		n, _ := c.StrLen(ctx, key).Result()
		kv.Size = n
		end := int64(maxStringBytes - 1)
		s, err := c.GetRange(ctx, key, 0, end).Result()
		if err != nil {
			return nil, err
		}
		kv.Truncated = n > maxStringBytes
		kv.Value, kv.Binary = display(s)
	case "list":
		kv.Size, _ = c.LLen(ctx, key).Result()
		items, err := c.LRange(ctx, key, 0, maxItems-1).Result()
		if err != nil {
			return nil, err
		}
		kv.Truncated = kv.Size > maxItems
		kv.Value = displayAll(items)
	case "set":
		kv.Size, _ = c.SCard(ctx, key).Result()
		items, _, err := c.SScan(ctx, key, 0, "*", maxItems).Result()
		if err != nil {
			return nil, err
		}
		kv.Truncated = kv.Size > int64(len(items))
		kv.Value = displayAll(items)
	case "zset":
		kv.Size, _ = c.ZCard(ctx, key).Result()
		items, err := c.ZRangeWithScores(ctx, key, 0, maxItems-1).Result()
		if err != nil {
			return nil, err
		}
		kv.Truncated = kv.Size > maxItems
		rows := make([][2]interface{}, 0, len(items))
		for _, z := range items {
			m, _ := display(fmt.Sprint(z.Member))
			rows = append(rows, [2]interface{}{m, z.Score})
		}
		kv.Value = rows
	case "hash":
		kv.Size, _ = c.HLen(ctx, key).Result()
		items, _, err := c.HScan(ctx, key, 0, "*", maxItems*2).Result()
		if err != nil {
			return nil, err
		}
		rows := make([][2]string, 0, len(items)/2)
		for i := 0; i+1 < len(items); i += 2 {
			f, _ := display(items[i])
			v, _ := display(items[i+1])
			rows = append(rows, [2]string{f, v})
		}
		kv.Truncated = kv.Size > int64(len(rows))
		kv.Value = rows
	case "stream":
		kv.Size, _ = c.XLen(ctx, key).Result()
		items, err := c.XRevRangeN(ctx, key, "+", "-", 50).Result()
		if err != nil {
			return nil, err
		}
		kv.Truncated = kv.Size > int64(len(items))
		kv.Value = items
	default:
		kv.Value = "unsupported type: " + typ
	}
	return kv, nil
}

func displayAll(in []string) []string {
	out := make([]string, len(in))
	for i, s := range in {
		out[i], _ = display(s)
	}
	return out
}

func redisInfo(c *redis.Client) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
	defer cancel()
	return c.Info(ctx, "server", "memory", "clients", "keyspace").Result()
}
