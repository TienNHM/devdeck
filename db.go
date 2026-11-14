package main

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-sql-driver/mysql"
	_ "github.com/jackc/pgx/v5/stdlib" // registers "pgx"
	_ "modernc.org/sqlite"             // registers "sqlite" (pure Go, no cgo)
)

// Read-only database viewer for PostgreSQL, MySQL/MariaDB and SQLite.
// Passwords are typed per connection and kept in memory only; profiles on disk never contain them.

const (
	dbIdleClose   = 5 * time.Minute // idle pool connections are closed after this
	dbSessionTTL  = 12 * time.Hour  // forget an unused session (and its in-memory password)
	maxDBSessions = 16
)

type dbProfile struct {
	Name     string `json:"name"`
	Driver   string `json:"driver"` // postgres | mysql | sqlite
	Host     string `json:"host,omitempty"`
	Port     int    `json:"port,omitempty"`
	User     string `json:"user,omitempty"`
	Database string `json:"database,omitempty"`
	SSLMode  string `json:"sslmode,omitempty"` // postgres: disable|prefer|require|verify-ca|verify-full; mysql: false|preferred|true|skip-verify
	Path     string `json:"path,omitempty"`    // sqlite file
}

type dbProfileStore struct {
	Profiles []dbProfile `json:"profiles"`
}

func loadDBProfiles() []dbProfile {
	var s dbProfileStore
	_ = loadJSON("db", &s)
	if s.Profiles == nil {
		s.Profiles = []dbProfile{}
	}
	return s.Profiles
}

func findDBProfile(name string) (dbProfile, error) {
	for _, p := range loadDBProfiles() {
		if strings.EqualFold(p.Name, name) {
			return p, nil
		}
	}
	return dbProfile{}, fmt.Errorf("no saved database profile named %q", name)
}

var pgSSLModes = map[string]bool{"disable": true, "allow": true, "prefer": true, "require": true, "verify-ca": true, "verify-full": true}
var mysqlTLSModes = map[string]bool{"false": true, "preferred": true, "true": true, "skip-verify": true}

// normalize validates a profile and fills defaults. checkFile also verifies a SQLite file exists.
func (p *dbProfile) normalize(checkFile bool) error {
	p.Name = strings.TrimSpace(p.Name)
	p.Host = strings.TrimSpace(p.Host)
	p.User = strings.TrimSpace(p.User)
	p.Database = strings.TrimSpace(p.Database)
	p.Path = strings.TrimSpace(p.Path)
	if len(p.Name) > 60 {
		return errors.New("profile name too long (max 60)")
	}
	if len(p.Host) > 255 || len(p.User) > 128 || len(p.Database) > 128 || len(p.Path) > 1024 {
		return errors.New("a profile field is too long")
	}
	if strings.ContainsAny(p.Host, " \t\r\n/?#@") {
		return errors.New("bad host name")
	}
	if p.Port < 0 || p.Port > 65535 {
		return errors.New("bad port")
	}
	switch p.Driver {
	case "postgres", "mysql":
		if p.Host == "" {
			p.Host = "127.0.0.1"
		}
		if p.Port == 0 {
			p.Port = map[string]int{"postgres": 5432, "mysql": 3306}[p.Driver]
		}
		p.Path = ""
		if p.Driver == "postgres" {
			if p.SSLMode == "" {
				p.SSLMode = "prefer"
			}
			if !pgSSLModes[p.SSLMode] {
				return fmt.Errorf("bad sslmode %q", p.SSLMode)
			}
		} else {
			if p.SSLMode == "" {
				p.SSLMode = "preferred"
			}
			if !mysqlTLSModes[p.SSLMode] {
				return fmt.Errorf("bad tls mode %q", p.SSLMode)
			}
		}
	case "sqlite":
		if p.Path == "" {
			return errors.New("SQLite needs a file path")
		}
		p.Host, p.Port, p.User, p.Database, p.SSLMode = "", 0, "", "", ""
		if checkFile {
			fi, err := os.Stat(p.Path)
			if err != nil {
				return fmt.Errorf("cannot open SQLite file: %w", err)
			}
			if !fi.Mode().IsRegular() {
				return errors.New("SQLite path is not a regular file")
			}
		}
	default:
		return fmt.Errorf("unsupported driver %q (postgres, mysql, sqlite)", p.Driver)
	}
	return nil
}

func cleanDBProfiles(in []dbProfile) ([]dbProfile, error) {
	if len(in) > 100 {
		return nil, errors.New("too many profiles (max 100)")
	}
	seen := map[string]bool{}
	out := make([]dbProfile, 0, len(in))
	for _, p := range in {
		if err := p.normalize(false); err != nil {
			return nil, fmt.Errorf("%s: %w", p.Name, err)
		}
		if p.Name == "" {
			return nil, errors.New("every profile needs a name")
		}
		if seen[strings.ToLower(p.Name)] {
			return nil, fmt.Errorf("duplicate profile name %q", p.Name)
		}
		seen[strings.ToLower(p.Name)] = true
		out = append(out, p)
	}
	return out, nil
}

// ------------------------------------------------------------------ dialects

// sqlDriverName maps a profile driver to the database/sql driver name.
func sqlDriverName(d string) string {
	switch d {
	case "postgres":
		return "pgx"
	case "mysql":
		return "mysql"
	}
	return "sqlite"
}

// dbDSN builds the connection string. SQLite is always opened read-only with query_only.
func dbDSN(p dbProfile, password string) (string, error) {
	switch p.Driver {
	case "postgres":
		u := url.URL{Scheme: "postgres", Host: net.JoinHostPort(p.Host, strconv.Itoa(p.Port)), Path: "/" + p.Database}
		if p.User != "" {
			if password != "" {
				u.User = url.UserPassword(p.User, password)
			} else {
				u.User = url.User(p.User)
			}
		}
		q := url.Values{}
		q.Set("sslmode", p.SSLMode)
		q.Set("connect_timeout", "8")
		q.Set("application_name", "devdeck")
		// Belt and braces: every transaction on this session defaults to read-only too.
		q.Set("default_transaction_read_only", "on")
		u.RawQuery = q.Encode()
		return u.String(), nil
	case "mysql":
		c := mysql.NewConfig()
		c.User = p.User
		c.Passwd = password
		c.Net = "tcp"
		c.Addr = net.JoinHostPort(p.Host, strconv.Itoa(p.Port))
		c.DBName = p.Database
		c.Timeout = 8 * time.Second
		c.ReadTimeout = 30 * time.Second
		c.WriteTimeout = 30 * time.Second
		c.TLSConfig = p.SSLMode
		c.MultiStatements = false
		c.AllowNativePasswords = true
		return c.FormatDSN(), nil
	case "sqlite":
		return sqliteDSN(p.Path), nil
	}
	return "", fmt.Errorf("unsupported driver %q", p.Driver)
}

func sqliteDSN(path string) string {
	abs, err := filepath.Abs(path)
	if err == nil {
		path = abs
	}
	path = filepath.ToSlash(path)
	r := strings.NewReplacer("%", "%25", "?", "%3f", "#", "%23")
	path = r.Replace(path)
	if !strings.HasPrefix(path, "/") {
		path = "/" + path // file:///C:/x.db on Windows
	}
	return "file://" + path + "?mode=ro&_pragma=query_only(1)&_pragma=busy_timeout(5000)"
}

// quoteIdent quotes an identifier for the dialect. It is the only way user-supplied names enter generated SQL.
func quoteIdent(driver, s string) (string, error) {
	if s == "" {
		return "", errors.New("empty identifier")
	}
	if strings.ContainsRune(s, 0) || len(s) > 256 {
		return "", errors.New("invalid identifier")
	}
	if driver == "mysql" {
		return "`" + strings.ReplaceAll(s, "`", "``") + "`", nil
	}
	return `"` + strings.ReplaceAll(s, `"`, `""`) + `"`, nil
}

// placeholder returns the n-th (1-based) bind parameter marker.
func placeholder(driver string, n int) string {
	if driver == "postgres" {
		return "$" + strconv.Itoa(n)
	}
	return "?"
}

// ------------------------------------------------------------------ sessions

type dbSession struct {
	id      string
	profile dbProfile
	db      *sql.DB
	version string
	mu      sync.Mutex
	used    time.Time
}

func (s *dbSession) touch() {
	s.mu.Lock()
	s.used = time.Now()
	s.mu.Unlock()
}

func (s *dbSession) lastUsed() time.Time {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.used
}

type dbSessions struct {
	mu sync.Mutex
	m  map[string]*dbSession
}

var dbconns = &dbSessions{m: map[string]*dbSession{}}

// openDB opens a pool for the profile: at most 2 connections, idle ones closed after dbIdleClose.
func openDB(p dbProfile, password string) (*sql.DB, string, error) {
	if err := p.normalize(true); err != nil {
		return nil, "", err
	}
	dsn, err := dbDSN(p, password)
	if err != nil {
		return nil, "", err
	}
	db, err := sql.Open(sqlDriverName(p.Driver), dsn)
	if err != nil {
		return nil, "", err
	}
	db.SetMaxOpenConns(2)
	db.SetMaxIdleConns(1)
	db.SetConnMaxIdleTime(dbIdleClose)
	db.SetConnMaxLifetime(30 * time.Minute)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, "", err
	}
	var ver string
	q := map[string]string{"postgres": "SELECT version()", "mysql": "SELECT VERSION()", "sqlite": "SELECT 'SQLite ' || sqlite_version()"}[p.Driver]
	_ = db.QueryRowContext(ctx, q).Scan(&ver)
	if p.Driver == "mysql" && !strings.Contains(strings.ToLower(ver), "mariadb") {
		ver = "MySQL " + ver
	}
	if len(ver) > 120 {
		ver = ver[:120]
	}
	return db, ver, nil
}

func (ds *dbSessions) open(p dbProfile, password string) (*dbSession, error) {
	db, ver, err := openDB(p, password)
	if err != nil {
		return nil, err
	}
	_ = p.normalize(false)
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	s := &dbSession{id: hex.EncodeToString(b), profile: p, db: db, version: ver, used: time.Now()}
	ds.mu.Lock()
	ds.sweepLocked()
	ds.m[s.id] = s
	ds.mu.Unlock()
	return s, nil
}

func (ds *dbSessions) sweepLocked() {
	var oldest *dbSession
	for id, s := range ds.m {
		if time.Since(s.lastUsed()) > dbSessionTTL {
			delete(ds.m, id)
			_ = s.db.Close()
			continue
		}
		if oldest == nil || s.lastUsed().Before(oldest.lastUsed()) {
			oldest = s
		}
	}
	if len(ds.m) >= maxDBSessions && oldest != nil {
		delete(ds.m, oldest.id)
		_ = oldest.db.Close()
	}
}

func (ds *dbSessions) get(id string) (*dbSession, error) {
	ds.mu.Lock()
	defer ds.mu.Unlock()
	s := ds.m[id]
	if s == nil {
		return nil, errors.New("unknown database connection; connect again")
	}
	if time.Since(s.lastUsed()) > dbSessionTTL {
		delete(ds.m, id)
		_ = s.db.Close()
		return nil, errors.New("database connection expired; connect again")
	}
	s.touch()
	return s, nil
}

func (ds *dbSessions) close(id string) {
	ds.mu.Lock()
	s := ds.m[id]
	delete(ds.m, id)
	ds.mu.Unlock()
	if s != nil {
		_ = s.db.Close()
	}
}

func defaultSchema(p dbProfile) string {
	switch p.Driver {
	case "postgres":
		return "public"
	case "mysql":
		return p.Database
	}
	return "main"
}
