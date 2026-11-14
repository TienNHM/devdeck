package main

import (
	"context"
	"database/sql"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
)

// makeSQLite creates a writable fixture database, then closes it.
func makeSQLite(t *testing.T) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "my data #1.db")
	db, err := sql.Open("sqlite", p)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for _, q := range []string{
		`CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT NOT NULL, email TEXT, "we""ird" TEXT DEFAULT 'x', blob BLOB)`,
		`CREATE UNIQUE INDEX users_email ON users(email)`,
		`CREATE INDEX users_name_email ON users(name, email)`,
		`CREATE VIEW v_users AS SELECT id, name FROM users`,
		`INSERT INTO users (name, email, blob) VALUES ('alice','a@x', x'00ff'), ('bob', NULL, NULL), ('carol','c@x', NULL), ('dave','d@x', NULL)`,
	} {
		if _, err := db.Exec(q); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	return p
}

func openTestSQLite(t *testing.T) (*dbSession, string) {
	t.Helper()
	p := makeSQLite(t)
	s, err := dbconns.open(dbProfile{Name: "t", Driver: "sqlite", Path: p}, "")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { dbconns.close(s.id) })
	return s, p
}

func TestSQLiteReadOnlyPath(t *testing.T) {
	s, _ := openTestSQLite(t)
	ctx := context.Background()
	if !strings.HasPrefix(s.version, "SQLite ") {
		t.Fatalf("version %q", s.version)
	}
	r, err := runReadOnly(ctx, s.db, "sqlite", "SELECT id, name, email, blob FROM users ORDER BY id", nil, 2, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Rows) != 2 || !r.Truncated || r.Columns[1].Name != "name" {
		t.Fatalf("cap/columns: %+v", r)
	}
	if r.Rows[0][3] != `\x00ff` || r.Rows[1][2] != nil {
		t.Fatalf("binary/NULL rendering: %#v", r.Rows)
	}

	// writes must fail even if they get past the SQL check (check=false here on purpose)
	for _, q := range []string{
		"INSERT INTO users (name) VALUES ('mallory')",
		"UPDATE users SET name = 'x'",
		"DELETE FROM users",
		"CREATE TABLE evil (a)",
		"DROP TABLE users",
	} {
		if _, err := runReadOnly(ctx, s.db, "sqlite", q, nil, 10, false); err == nil {
			t.Errorf("write statement succeeded in read-only path: %s", q)
		}
		if _, err := runReadOnly(ctx, s.db, "sqlite", q, nil, 10, true); err == nil {
			t.Errorf("write statement passed the SQL check: %s", q)
		}
	}
	// query_only cannot be switched off through the checked path
	if _, err := runReadOnly(ctx, s.db, "sqlite", "PRAGMA query_only = 0", nil, 10, true); err == nil {
		t.Error("PRAGMA assignment accepted")
	}
	r, err = runReadOnly(ctx, s.db, "sqlite", "SELECT count(*) FROM users", nil, 10, true)
	if err != nil || r.Rows[0][0] != int64(4) {
		t.Fatalf("data changed or count failed: %v %v", r, err)
	}
	if _, err := runReadOnly(ctx, s.db, "sqlite", "PRAGMA table_info(users)", nil, 10, true); err != nil {
		t.Errorf("read pragma rejected: %v", err)
	}
}

func TestSQLiteSchemaAndGrid(t *testing.T) {
	s, _ := openTestSQLite(t)
	ctx := context.Background()
	schemas, err := dbSchemas(ctx, s)
	if err != nil || len(schemas) != 1 || schemas[0] != "main" {
		t.Fatalf("schemas %v %v", schemas, err)
	}
	ts, err := dbTables(ctx, s, "main")
	if err != nil || len(ts) != 2 || ts[0].Name != "users" || ts[1].Type != "view" {
		t.Fatalf("tables %+v %v", ts, err)
	}
	info, err := dbDescribe(ctx, s, "main", "users")
	if err != nil {
		t.Fatal(err)
	}
	if len(info.Columns) != 5 || !info.Columns[0].PK || info.Columns[1].Nullable || !info.Columns[2].Nullable || info.Columns[3].Name != `we"ird` || info.Columns[3].Default == nil {
		t.Fatalf("columns %+v", info.Columns)
	}
	var multi *dbIndex
	for i, ix := range info.Indexes {
		if ix.Name == "users_name_email" {
			multi = &info.Indexes[i]
		}
	}
	if multi == nil || strings.Join(multi.Columns, ",") != "name,email" || multi.Unique {
		t.Fatalf("indexes %+v", info.Indexes)
	}

	q := dbRowsReq{Schema: "main", Table: "users", Limit: 2, Sort: "name", Desc: true}
	r, err := dbTableRows(ctx, s, q)
	if err != nil || len(r.Rows) != 2 || !r.Truncated || r.Rows[0][1] != "dave" {
		t.Fatalf("page 1 %+v %v", r, err)
	}
	q.Offset = 2
	r, err = dbTableRows(ctx, s, q)
	if err != nil || len(r.Rows) != 2 || r.Truncated || r.Rows[1][1] != "alice" {
		t.Fatalf("page 2 %+v %v", r, err)
	}
	q = dbRowsReq{Schema: "main", Table: "users", Filter: &dbFilter{Column: "name", Op: "like", Value: "o"}}
	if r, err = dbTableRows(ctx, s, q); err != nil || len(r.Rows) != 2 {
		t.Fatalf("like filter %+v %v", r, err)
	}
	if n, err := dbTableCount(ctx, s, dbRowsReq{Schema: "main", Table: "users", Filter: &dbFilter{Column: "email", Op: "null"}}); err != nil || n != 1 {
		t.Fatalf("count null %d %v", n, err)
	}
	if n, err := dbTableCount(ctx, s, dbRowsReq{Schema: "main", Table: "users", Filter: &dbFilter{Column: "id", Op: "ge", Value: "2"}}); err != nil || n != 3 {
		t.Fatalf("count ge %d %v", n, err)
	}
	// an injection attempt through an identifier is only a (missing) column name; SQLite even
	// treats unknown "quoted" names as string literals, so it may run, but nothing else executes
	_, _ = dbTableRows(ctx, s, dbRowsReq{Schema: "main", Table: "users", Sort: `name"; DROP TABLE users; --`})
	if _, err := dbTableRows(ctx, s, dbRowsReq{Schema: "main", Table: `users"; DROP TABLE users; --`}); err == nil {
		t.Fatal("bogus table name accepted")
	}
	if n, _ := dbTableCount(ctx, s, dbRowsReq{Schema: "main", Table: "users"}); n != 4 {
		t.Fatal("table damaged")
	}
}

func TestDBRoutesSQLite(t *testing.T) {
	t.Setenv("USERPROFILE", t.TempDir())
	t.Setenv("HOME", t.TempDir())
	p := makeSQLite(t)
	mux := http.NewServeMux()
	registerDBRoutes(mux)
	rec, m := doJSON(t, mux, "POST", "/api/db/open", map[string]any{"profile": map[string]any{"name": "x", "driver": "sqlite", "path": p}})
	if rec.Code != 200 {
		t.Fatalf("open %d %s", rec.Code, rec.Body)
	}
	id := m["id"].(string)
	defer dbconns.close(id)
	if m["schema"] != "main" {
		t.Fatalf("default schema %v", m)
	}
	if rec, m := doJSON(t, mux, "POST", "/api/db/query", map[string]any{"id": id, "sql": "DELETE FROM users"}); rec.Code != 400 || !strings.Contains(m["error"].(string), "read-only") {
		t.Fatalf("delete via query route: %d %v", rec.Code, m)
	}
	if rec, _ := doJSON(t, mux, "POST", "/api/db/query", map[string]any{"id": id, "sql": "select 1; select 2"}); rec.Code != 400 {
		t.Fatal("multi statement accepted")
	}
	if rec, m := doJSON(t, mux, "POST", "/api/db/query", map[string]any{"id": id, "sql": "-- hi\nselect name from users where id = 1;"}); rec.Code != 200 || m["rows"].([]any)[0].([]any)[0] != "alice" {
		t.Fatalf("select: %d %v", rec.Code, m)
	}
	// profiles: saved without the password
	rec, _ = doJSON(t, mux, "PUT", "/api/db/profiles", []map[string]any{{"name": "pg", "driver": "postgres", "host": "db", "user": "me", "password": "secret"}, {"name": "lite", "driver": "sqlite", "path": p}})
	if rec.Code != 200 || strings.Contains(rec.Body.String(), "secret") {
		t.Fatalf("profiles: %d %s", rec.Code, rec.Body)
	}
	ps := loadDBProfiles()
	if len(ps) != 2 || ps[0].Port != 5432 || ps[0].SSLMode != "prefer" {
		t.Fatalf("profile defaults %+v", ps)
	}
	if rec, _ := doJSON(t, mux, "PUT", "/api/db/profiles", []map[string]any{{"name": "bad", "driver": "oracle"}}); rec.Code != 400 {
		t.Fatal("bad driver accepted")
	}
	// MCP tools run against the saved profile
	var dbq, dbt tool
	for _, tl := range mcpExtra {
		switch tl.Name {
		case "db_query":
			dbq = tl
		case "db_tables":
			dbt = tl
		}
	}
	out, err := dbq.handler(map[string]any{"profile": "lite", "sql": "select name from users order by id limit 2"})
	if err != nil || !strings.Contains(out, "alice\nbob\n") {
		t.Fatalf("db_query: %q %v", out, err)
	}
	if _, err := dbq.handler(map[string]any{"profile": "lite", "sql": "drop table users"}); err == nil {
		t.Fatal("db_query ran a write")
	}
	out, err = dbt.handler(map[string]any{"profile": "lite"})
	if err != nil || !strings.Contains(out, "main.users\ttable") || !strings.Contains(out, "main.v_users\tview") {
		t.Fatalf("db_tables: %q %v", out, err)
	}
}

func TestQuoteIdent(t *testing.T) {
	cases := []struct{ d, in, want string }{
		{"postgres", "users", `"users"`},
		{"postgres", `we"ird`, `"we""ird"`},
		{"sqlite", `a"; DROP TABLE x; --`, `"a""; DROP TABLE x; --"`},
		{"mysql", "users", "`users`"},
		{"mysql", "we`ird", "`we``ird`"},
		{"mysql", `a"b`, "`a\"b`"},
	}
	for _, c := range cases {
		got, err := quoteIdent(c.d, c.in)
		if err != nil || got != c.want {
			t.Errorf("quoteIdent(%s, %q) = %q, %v; want %q", c.d, c.in, got, err, c.want)
		}
	}
	if _, err := quoteIdent("postgres", ""); err == nil {
		t.Error("empty identifier accepted")
	}
	if _, err := quoteIdent("postgres", "a\x00b"); err == nil {
		t.Error("NUL accepted")
	}
}

func TestBuildRowsSQLDialects(t *testing.T) {
	q := dbRowsReq{Schema: "public", Table: `or"ders`, Limit: 50, Offset: 100, Sort: "created at", Desc: true, Filter: &dbFilter{Column: "status", Op: "eq", Value: "x' OR 1=1 --"}}
	s, args, err := buildRowsSQL("postgres", q)
	if err != nil {
		t.Fatal(err)
	}
	want := `SELECT * FROM "public"."or""ders" WHERE "status" = $1 ORDER BY "created at" DESC LIMIT 51 OFFSET 100`
	if s != want || len(args) != 1 || args[0] != "x' OR 1=1 --" {
		t.Fatalf("postgres:\n got %s %v\nwant %s", s, args, want)
	}
	q.Filter = &dbFilter{Column: "note", Op: "like", Value: "50%"}
	s, args, _ = buildRowsSQL("mysql", q)
	want = "SELECT * FROM `public`.`or\"ders` WHERE CAST(`note` AS CHAR) LIKE ? ORDER BY `created at` DESC LIMIT 51 OFFSET 100"
	if s != want || args[0] != "%50%%" {
		t.Fatalf("mysql:\n got %s %v\nwant %s", s, args, want)
	}
	q.Filter = &dbFilter{Column: "x", Op: "notnull"}
	q.Sort = ""
	s, args, _ = buildRowsSQL("sqlite", q)
	if s != `SELECT * FROM "public"."or""ders" WHERE "x" IS NOT NULL LIMIT 51 OFFSET 100` || len(args) != 0 {
		t.Fatalf("sqlite: %s", s)
	}
	if _, _, err := buildRowsSQL("postgres", dbRowsReq{Table: "t", Filter: &dbFilter{Column: "a", Op: "; drop"}}); err == nil {
		t.Fatal("bad operator accepted")
	}
}

func TestCheckReadOnlySQL(t *testing.T) {
	ok := []struct{ d, q string }{
		{"postgres", "select 1"},
		{"postgres", "  (SELECT 1) UNION (SELECT 2);  "},
		{"postgres", "WITH x AS (SELECT 1) SELECT * FROM x"},
		{"postgres", "select ';' as semi, $$ ; drop table x $$ as body, $tag$;$tag$"},
		{"postgres", "select E'it\\'s ; fine'"},
		{"postgres", "/* c ; */ select 1 -- ; trailing"},
		{"postgres", `select "pg_sleep" from t`},
		{"postgres", "explain analyze select * from t"},
		{"postgres", "show search_path"},
		{"mysql", "SHOW TABLES"},
		{"mysql", "describe t"},
		{"mysql", "select 'a\\';b' from t"},
		{"mysql", "select /*+ MAX_EXECUTION_TIME(10) */ 1"},
		{"sqlite", "pragma table_info('t')"},
		{"sqlite", "select [a;b] from t"},
	}
	for _, c := range ok {
		if err := checkReadOnlySQL(c.q, c.d); err != nil {
			t.Errorf("%s %q rejected: %v", c.d, c.q, err)
		}
	}
	bad := []struct{ d, q string }{
		{"postgres", ""},
		{"postgres", "-- only a comment"},
		{"postgres", "insert into t values (1)"},
		{"postgres", "select 1; drop table t"},
		{"postgres", "select 1;; select 2"},
		{"postgres", "COMMIT"},
		{"postgres", "set transaction read write"},
		{"postgres", "select pg_terminate_backend(123)"},
		{"postgres", `select "PG_TERMINATE_BACKEND"(1)`},
		{"postgres", "select set_config('default_transaction_read_only','off',false)"},
		{"postgres", "select lo_export(1, '/tmp/x')"},
		{"postgres", "copy t to '/tmp/x'"},
		{"postgres", "select 'unterminated"},
		{"postgres", "select $$ open"},
		{"mysql", "select * from t into outfile '/tmp/x'"},
		{"mysql", "/*!50000 drop table t */"},
		{"mysql", "select 1 /*! , sleep(1) */"},
		{"mysql", "create table t (a int)"},
		{"mysql", "select 1 # ok\n; drop table t"},
		{"mysql", "load data infile 'x' into table t"},
		{"sqlite", "pragma query_only = 0"},
		{"sqlite", "attach database 'x' as y"},
		{"sqlite", "vacuum"},
		{"sqlite", "show tables"},
	}
	for _, c := range bad {
		if err := checkReadOnlySQL(c.q, c.d); err == nil {
			t.Errorf("%s %q accepted", c.d, c.q)
		}
	}
}

func TestDSN(t *testing.T) {
	p := dbProfile{Driver: "postgres", Host: "db.local", User: "me@corp", Database: "app"}
	if err := p.normalize(false); err != nil {
		t.Fatal(err)
	}
	dsn, _ := dbDSN(p, "p@ss:/word")
	if !strings.HasPrefix(dsn, "postgres://me%40corp:p%40ss%3A%2Fword@db.local:5432/app?") || !strings.Contains(dsn, "default_transaction_read_only=on") || !strings.Contains(dsn, "sslmode=prefer") {
		t.Fatalf("pg dsn %s", dsn)
	}
	p = dbProfile{Driver: "mysql", Host: "::1", User: "root", Database: "app", SSLMode: "false"}
	_ = p.normalize(false)
	dsn, _ = dbDSN(p, "pw")
	if !strings.HasPrefix(dsn, "root:pw@tcp([::1]:3306)/app?") || strings.Contains(dsn, "multiStatements=true") {
		t.Fatalf("mysql dsn %s", dsn)
	}
	if d := sqliteDSN("/tmp/a b?#%.db"); !strings.HasPrefix(d, "file://") || !strings.Contains(d, "mode=ro") || !strings.Contains(d, "query_only(1)") || strings.Contains(d, "?#") {
		t.Fatalf("sqlite dsn %s", d)
	}
	bad := []dbProfile{{Driver: "postgres", Host: "a/b"}, {Driver: "sqlite"}, {Driver: "postgres", SSLMode: "nope"}, {Driver: "mysql", Port: 70000}}
	for _, b := range bad {
		if err := b.normalize(false); err == nil {
			t.Errorf("accepted %+v", b)
		}
	}
	if _, _, err := openDB(dbProfile{Driver: "sqlite", Path: filepath.Join(t.TempDir(), "missing.db")}, ""); err == nil {
		t.Error("missing sqlite file opened (would create it)")
	}
}

func TestCellValue(t *testing.T) {
	if v, _, _ := cellValue(int64(1) << 60); v != "1152921504606846976" {
		t.Errorf("big int: %v", v)
	}
	if v, _, _ := cellValue([]byte("héllo")); v != "héllo" {
		t.Errorf("utf8 bytes: %v", v)
	}
	big := strings.Repeat("é", dbMaxCellBytes)
	if v, cut, _ := cellValue(big); !cut || len(v.(string)) > dbMaxCellBytes {
		t.Errorf("cut: %v", cut)
	}
}
