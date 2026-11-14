package main

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
)

// Config is persisted in ~/.devdeck/config.json. It never contains passwords.
type Config struct {
	Hosts []SSHHost      `json:"hosts"`
	Redis []RedisProfile `json:"redis"`
	Repos []string       `json:"repos"`
}

type SSHHost struct {
	Name    string `json:"name"`
	Host    string `json:"host"`
	Port    int    `json:"port"`
	User    string `json:"user"`
	KeyPath string `json:"keyPath"`
}

type RedisProfile struct {
	Name string `json:"name"`
	Addr string `json:"addr"`
	DB   int    `json:"db"`
	TLS  bool   `json:"tls"`
}

var cfgMu sync.Mutex

func dataDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		home = "."
	}
	d := filepath.Join(home, ".devdeck")
	_ = os.MkdirAll(d, 0o700)
	return d
}

func loadConfig() Config {
	cfgMu.Lock()
	defer cfgMu.Unlock()
	var c Config
	b, err := os.ReadFile(filepath.Join(dataDir(), "config.json"))
	if err == nil {
		_ = json.Unmarshal(b, &c)
	}
	return c
}

func saveConfig(c Config) error {
	cfgMu.Lock()
	defer cfgMu.Unlock()
	b, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dataDir(), "config.json"), b, 0o600)
}

// loadToken returns the persistent random access token, creating it on first use.
func loadToken() string {
	p := filepath.Join(dataDir(), "token")
	if b, err := os.ReadFile(p); err == nil {
		if t := strings.TrimSpace(string(b)); len(t) >= 32 {
			return t
		}
	}
	buf := make([]byte, 24)
	_, _ = rand.Read(buf)
	t := hex.EncodeToString(buf)
	_ = os.WriteFile(p, []byte(t), 0o600)
	return t
}

func writePortFile(port int) {
	_ = os.WriteFile(filepath.Join(dataDir(), "port"), []byte(strconv.Itoa(port)), 0o600)
}

func readPortFile() int {
	b, err := os.ReadFile(filepath.Join(dataDir(), "port"))
	if err != nil {
		return 0
	}
	n, _ := strconv.Atoi(strings.TrimSpace(string(b)))
	return n
}
