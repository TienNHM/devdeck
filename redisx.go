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

// Redis access is strictly read-only: only SCAN, TYPE, TTL and read commands are issued.

type redisConnReq struct {
	Addr     string `json:"addr"`
	Password string `json:"password"`
	DB       int    `json:"db"`
	TLS      bool   `json:"tls"`
}

type redisConns struct {
	mu sync.Mutex
	m  map[string]*redis.Client
}

var rconns = &redisConns{m: map[string]*redis.Client{}}

func newRedisClient(r redisConnReq) *redis.Client {
	opt := &redis.Options{
		Addr: r.Addr, Password: r.Password, DB: r.DB,
		DialTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second,
		PoolSize: 2, MinIdleConns: 0,
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
	rc.mu.Lock()
	rc.m[id] = cl
	rc.mu.Unlock()
	return id, ver, nil
}

func (rc *redisConns) get(id string) (*redis.Client, error) {
	rc.mu.Lock()
	defer rc.mu.Unlock()
	c := rc.m[id]
	if c == nil {
		return nil, errors.New("unknown redis connection; connect again")
	}
	return c, nil
}

func (rc *redisConns) close(id string) {
	rc.mu.Lock()
	c := rc.m[id]
	delete(rc.m, id)
	rc.mu.Unlock()
	if c != nil {
		_ = c.Close()
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
