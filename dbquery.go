package main

import (
	"context"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

// Every query, user-written or generated, runs through runReadOnly:
//   - PostgreSQL: BEGIN READ ONLY + SET LOCAL statement_timeout, always rolled back
//     (the session also has default_transaction_read_only=on).
//   - MySQL/MariaDB: START TRANSACTION READ ONLY (+ max_execution_time when supported), always rolled back.
//   - SQLite: file opened with mode=ro and PRAGMA query_only=ON; the transaction is rolled back too.
// User SQL is additionally inspected (checkReadOnlySQL): one statement, read-only leading keyword,
// no MySQL "INTO OUTFILE" or executable comments, no known side-effect functions on PostgreSQL.
// This is defence in depth; for hard guarantees use a database role without write grants.

const (
	dbRowCap        = 1000
	dbStmtTimeout   = 15 * time.Second
	dbMaxCellBytes  = 64 * 1024
	dbMaxResultSize = 8 << 20
	dbMaxSQLBytes   = 256 * 1024
)

type dbColumn struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

type dbResult struct {
	Columns   []dbColumn `json:"columns"`
	Rows      [][]any    `json:"rows"`
	Truncated bool       `json:"truncated"` // more rows exist than returned (cap or size budget)
	CellsCut  bool       `json:"cellsCut"`  // at least one cell value was shortened
	ElapsedMs int64      `json:"elapsedMs"`
	Cap       int        `json:"cap"`
}

// ------------------------------------------------------------------ SQL inspection

type sqlTok struct {
	kind byte   // 'w' bare word (lowercased), 'q' quoted identifier, 's' string, 'p' punctuation
	text string // for 'p' the single character
}

// sqlTokens is a small lexer that understands comments, strings and quoted identifiers well enough to find
// statement boundaries and keywords. It errs on the side of rejecting odd input.
func sqlTokens(q, driver string) ([]sqlTok, error) {
	var toks []sqlTok
	rs := []rune(q)
	n := len(rs)
	for i := 0; i < n; {
		c := rs[i]
		switch {
		case unicode.IsSpace(c):
			i++
		case c == '-' && i+1 < n && rs[i+1] == '-', c == '#' && driver == "mysql":
			for i < n && rs[i] != '\n' {
				i++
			}
		case c == '/' && i+1 < n && rs[i+1] == '*':
			if driver == "mysql" && i+2 < n && rs[i+2] == '!' {
				return nil, errors.New("MySQL executable comments (/*! ... */) are not allowed")
			}
			depth := 0
			for i < n {
				if i+1 < n && rs[i] == '/' && rs[i+1] == '*' {
					depth++
					i += 2
					if driver == "mysql" {
						depth = 1 // MySQL comments do not nest
					}
				} else if i+1 < n && rs[i] == '*' && rs[i+1] == '/' {
					depth--
					i += 2
					if depth == 0 {
						break
					}
				} else {
					i++
				}
			}
			if depth != 0 {
				return nil, errors.New("unterminated comment")
			}
		case c == '\'' || (c == '"' && driver == "mysql"):
			// string literal; backslash escapes in MySQL and in PostgreSQL E'' strings
			bs := driver == "mysql"
			if driver == "postgres" && len(toks) > 0 && i > 0 && (rs[i-1] == 'e' || rs[i-1] == 'E') {
				if t := toks[len(toks)-1]; t.kind == 'w' && t.text == "e" {
					bs = true
					toks = toks[:len(toks)-1]
				}
			}
			j := i + 1
			closed := false
			for j < n {
				if bs && rs[j] == '\\' {
					j += 2
					continue
				}
				if rs[j] == c {
					if j+1 < n && rs[j+1] == c {
						j += 2
						continue
					}
					closed = true
					break
				}
				j++
			}
			if !closed {
				return nil, errors.New("unterminated string literal")
			}
			toks = append(toks, sqlTok{'s', string(rs[i+1 : j])})
			i = j + 1
		case c == '"' || c == '`' || (c == '[' && driver == "sqlite"):
			end := c
			if c == '[' {
				end = ']'
			}
			j := i + 1
			var sb strings.Builder
			closed := false
			for j < n {
				if rs[j] == end {
					if end != ']' && j+1 < n && rs[j+1] == end {
						sb.WriteRune(end)
						j += 2
						continue
					}
					closed = true
					break
				}
				sb.WriteRune(rs[j])
				j++
			}
			if !closed {
				return nil, errors.New("unterminated quoted identifier")
			}
			toks = append(toks, sqlTok{'q', sb.String()})
			i = j + 1
		case c == '$' && driver == "postgres":
			// dollar-quoted string $tag$...$tag$ (or a $1 parameter)
			j := i + 1
			for j < n && (rs[j] == '_' || unicode.IsLetter(rs[j]) || (j > i+1 && unicode.IsDigit(rs[j]))) {
				j++
			}
			if j < n && rs[j] == '$' {
				tag := string(rs[i : j+1])
				rest := string(rs[j+1:])
				k := strings.Index(rest, tag)
				if k < 0 {
					return nil, errors.New("unterminated dollar-quoted string")
				}
				toks = append(toks, sqlTok{'s', rest[:k]})
				i = j + 1 + utf8.RuneCountInString(rest[:k]) + utf8.RuneCountInString(tag)
				continue
			}
			toks = append(toks, sqlTok{'p', "$"})
			i++
		case c == '_' || unicode.IsLetter(c):
			j := i
			for j < n && (rs[j] == '_' || rs[j] == '$' || unicode.IsLetter(rs[j]) || unicode.IsDigit(rs[j])) {
				j++
			}
			toks = append(toks, sqlTok{'w', strings.ToLower(string(rs[i:j]))})
			i = j
		case unicode.IsDigit(c):
			j := i
			for j < n && (unicode.IsDigit(rs[j]) || rs[j] == '.' || rs[j] == 'e' || rs[j] == 'E') {
				j++
			}
			toks = append(toks, sqlTok{'p', "0"})
			i = j
		default:
			toks = append(toks, sqlTok{'p', string(c)})
			i++
		}
	}
	return toks, nil
}

var readOnlyLead = map[string]map[string]bool{
	"postgres": {"select": true, "with": true, "values": true, "table": true, "explain": true, "show": true},
	"mysql":    {"select": true, "with": true, "values": true, "table": true, "explain": true, "show": true, "describe": true, "desc": true},
	"sqlite":   {"select": true, "with": true, "values": true, "explain": true, "pragma": true},
}

// PostgreSQL functions with side effects outside the transaction (signals, files, remote writes, config).
var pgDeniedFuncs = map[string]bool{
	"pg_terminate_backend": true, "pg_cancel_backend": true, "pg_reload_conf": true, "pg_rotate_logfile": true,
	"pg_promote": true, "pg_switch_wal": true, "pg_create_restore_point": true, "pg_log_backend_memory_contexts": true,
	"lo_import": true, "lo_export": true, "lo_unlink": true, "lo_create": true, "lo_from_bytea": true, "lo_put": true,
	"pg_file_write": true, "pg_file_rename": true, "pg_file_unlink": true, "pg_logdir_ls": true,
	"dblink": true, "dblink_exec": true, "dblink_connect": true, "dblink_send_query": true,
	"pg_advisory_lock": true, "pg_advisory_xact_lock": true, "pg_try_advisory_lock": true,
	"set_config": true, "pg_notify": true, "pg_sleep": true, "pg_sleep_for": true, "pg_sleep_until": true,
	"pg_create_logical_replication_slot": true, "pg_create_physical_replication_slot": true, "pg_drop_replication_slot": true,
	"pg_replication_origin_create": true, "pg_stat_reset": true, "pg_stat_statements_reset": true,
}

// checkReadOnlySQL rejects anything that is not a single read-only statement.
func checkReadOnlySQL(q, driver string) error {
	if len(q) > dbMaxSQLBytes {
		return errors.New("query too long")
	}
	toks, err := sqlTokens(q, driver)
	if err != nil {
		return err
	}
	// drop trailing semicolons, then any remaining ';' means a second statement
	for len(toks) > 0 && toks[len(toks)-1].kind == 'p' && toks[len(toks)-1].text == ";" {
		toks = toks[:len(toks)-1]
	}
	if len(toks) == 0 {
		return errors.New("empty query")
	}
	first := ""
	for _, t := range toks {
		if t.kind == 'p' && t.text == "(" {
			continue
		}
		if t.kind == 'w' {
			first = t.text
		}
		break
	}
	if !readOnlyLead[driver][first] {
		if first == "" {
			return errors.New("query must start with a keyword such as SELECT")
		}
		return fmt.Errorf("only read-only statements are allowed here (SELECT, WITH, SHOW, EXPLAIN…); got %s", strings.ToUpper(first))
	}
	for i, t := range toks {
		if t.kind == 'p' && t.text == ";" {
			return errors.New("only one statement per run")
		}
		if driver == "mysql" && t.kind == 'w' && t.text == "into" && i+1 < len(toks) &&
			toks[i+1].kind == 'w' && (toks[i+1].text == "outfile" || toks[i+1].text == "dumpfile") {
			return errors.New("SELECT ... INTO OUTFILE/DUMPFILE is not allowed")
		}
		if driver == "sqlite" && first == "pragma" && t.kind == 'p' && t.text == "=" {
			return errors.New("PRAGMA assignments are not allowed")
		}
		if driver == "postgres" && (t.kind == 'w' || t.kind == 'q') {
			name := t.text
			if t.kind == 'q' {
				name = strings.ToLower(name)
			}
			if pgDeniedFuncs[name] && i+1 < len(toks) && toks[i+1].kind == 'p' && toks[i+1].text == "(" {
				return fmt.Errorf("function %s() is not allowed in the read-only viewer", name)
			}
		}
	}
	return nil
}

// ------------------------------------------------------------------ execution

// runReadOnly executes one query inside a read-only transaction that is always rolled back.
// check=true applies checkReadOnlySQL (user SQL); generated SQL passes check=false.
func runReadOnly(ctx context.Context, db *sql.DB, driver, q string, args []any, rowCap int, check bool) (*dbResult, error) {
	if check {
		if err := checkReadOnlySQL(q, driver); err != nil {
			return nil, err
		}
	}
	if rowCap <= 0 || rowCap > dbRowCap {
		rowCap = dbRowCap
	}
	ctx, cancel := context.WithTimeout(ctx, dbStmtTimeout)
	defer cancel()
	start := time.Now()
	conn, err := db.Conn(ctx)
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	if driver == "mysql" {
		// MySQL 5.7.8+: SELECT timeout in ms. MariaDB uses max_statement_time in seconds. Best effort.
		if _, err := conn.ExecContext(ctx, "SET SESSION max_execution_time = "+strconv.Itoa(int(dbStmtTimeout/time.Millisecond))); err != nil {
			_, _ = conn.ExecContext(ctx, "SET SESSION max_statement_time = "+strconv.Itoa(int(dbStmtTimeout/time.Second)))
		}
	}
	// SQLite's driver has no read-only transaction mode; the connection itself is mode=ro + query_only.
	tx, err := conn.BeginTx(ctx, &sql.TxOptions{ReadOnly: driver != "sqlite"})
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	if driver == "postgres" {
		if _, err := tx.ExecContext(ctx, "SET LOCAL statement_timeout = "+strconv.Itoa(int(dbStmtTimeout/time.Millisecond))); err != nil {
			return nil, err
		}
	}
	rows, err := tx.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	res, err := collectRows(rows, rowCap)
	if err != nil {
		return nil, err
	}
	res.ElapsedMs = time.Since(start).Milliseconds()
	return res, nil
}

func collectRows(rows *sql.Rows, rowCap int) (*dbResult, error) {
	cts, err := rows.ColumnTypes()
	if err != nil {
		return nil, err
	}
	res := &dbResult{Columns: make([]dbColumn, len(cts)), Rows: [][]any{}, Cap: rowCap}
	for i, ct := range cts {
		res.Columns[i] = dbColumn{Name: ct.Name(), Type: strings.ToLower(ct.DatabaseTypeName())}
	}
	size := 0
	for rows.Next() {
		if len(res.Rows) >= rowCap || size > dbMaxResultSize {
			res.Truncated = true
			break
		}
		vals := make([]any, len(cts))
		ptrs := make([]any, len(cts))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return nil, err
		}
		for i, v := range vals {
			cv, cut, n := cellValue(v)
			vals[i] = cv
			size += n
			if cut {
				res.CellsCut = true
			}
		}
		res.Rows = append(res.Rows, vals)
	}
	return res, rows.Err()
}

// cellValue turns a driver value into something JSON can carry without loss of meaning.
// Returns the value, whether it was shortened, and its approximate size.
func cellValue(v any) (any, bool, int) {
	const maxSafe = 1 << 53
	switch x := v.(type) {
	case nil:
		return nil, false, 4
	case []byte:
		if utf8.Valid(x) {
			return cutString(string(x))
		}
		cut := false
		if len(x) > dbMaxCellBytes/2 {
			x, cut = x[:dbMaxCellBytes/2], true
		}
		return "\\x" + hex.EncodeToString(x), cut, 2 + 2*len(x)
	case string:
		return cutString(x)
	case time.Time:
		return x.Format(time.RFC3339Nano), false, 32
	case int64:
		if x > maxSafe || x < -maxSafe {
			return strconv.FormatInt(x, 10), false, 20
		}
		return x, false, 20
	case uint64:
		if x > maxSafe {
			return strconv.FormatUint(x, 10), false, 20
		}
		return x, false, 20
	case float64:
		if math.IsNaN(x) || math.IsInf(x, 0) {
			return strconv.FormatFloat(x, 'g', -1, 64), false, 8
		}
		return x, false, 24
	case float32:
		f := float64(x)
		if math.IsNaN(f) || math.IsInf(f, 0) {
			return strconv.FormatFloat(f, 'g', -1, 32), false, 8
		}
		return f, false, 16
	case bool, int, int8, int16, int32, uint8, uint16, uint32:
		return x, false, 8
	default:
		return cutString(fmt.Sprint(x))
	}
}

func cutString(s string) (any, bool, int) {
	if len(s) <= dbMaxCellBytes {
		return s, false, len(s) + 2
	}
	cut := s[:dbMaxCellBytes]
	for !utf8.ValidString(cut) && len(cut) > 0 {
		cut = cut[:len(cut)-1]
	}
	return cut, true, len(cut) + 2
}
