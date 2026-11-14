package main

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// Schema browsing and the paged table grid. All SQL here is generated: names go through quoteIdent,
// values through placeholders, and everything still runs inside runReadOnly.

type dbTable struct {
	Name string `json:"name"`
	Type string `json:"type"` // table | view
	Rows int64  `json:"rows"` // estimate; -1 when unknown
}

type dbColInfo struct {
	Name     string  `json:"name"`
	Type     string  `json:"type"`
	Nullable bool    `json:"nullable"`
	Default  *string `json:"default"`
	PK       bool    `json:"pk"`
}

type dbIndex struct {
	Name    string   `json:"name"`
	Columns []string `json:"columns"`
	Unique  bool     `json:"unique"`
	Primary bool     `json:"primary"`
}

type dbTableInfo struct {
	Columns []dbColInfo `json:"columns"`
	Indexes []dbIndex   `json:"indexes"`
}

func cellStr(v any) string {
	if v == nil {
		return ""
	}
	return fmt.Sprint(v)
}

func cellInt(v any) int64 {
	switch x := v.(type) {
	case int64:
		return x
	case float64:
		return int64(x)
	case bool:
		if x {
			return 1
		}
		return 0
	}
	n, err := strconv.ParseFloat(cellStr(v), 64)
	if err != nil {
		return -1
	}
	return int64(n)
}

func cellBool(v any) bool {
	switch x := v.(type) {
	case bool:
		return x
	case int64:
		return x != 0
	}
	s := strings.ToLower(cellStr(v))
	return s == "1" || s == "t" || s == "true" || s == "yes"
}

func (s *dbSession) run(ctx context.Context, q string, args ...any) (*dbResult, error) {
	return runReadOnly(ctx, s.db, s.profile.Driver, q, args, dbRowCap, false)
}

func dbSchemas(ctx context.Context, s *dbSession) ([]string, error) {
	var q string
	switch s.profile.Driver {
	case "postgres":
		q = `SELECT nspname FROM pg_namespace WHERE nspname NOT LIKE 'pg\_%' AND nspname <> 'information_schema' ORDER BY 1`
	case "mysql":
		q = `SELECT schema_name FROM information_schema.schemata WHERE schema_name NOT IN ('information_schema','mysql','performance_schema','sys') ORDER BY 1`
	default:
		q = `SELECT name FROM pragma_database_list WHERE name <> 'temp' ORDER BY seq`
	}
	r, err := s.run(ctx, q)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(r.Rows))
	for _, row := range r.Rows {
		out = append(out, cellStr(row[0]))
	}
	return out, nil
}

func dbTables(ctx context.Context, s *dbSession, schema string) ([]dbTable, error) {
	var r *dbResult
	var err error
	switch s.profile.Driver {
	case "postgres":
		r, err = s.run(ctx, `SELECT c.relname, CASE WHEN c.relkind IN ('v','m') THEN 'view' ELSE 'table' END, c.reltuples::bigint
			FROM pg_class c JOIN pg_namespace n ON n.oid = c.relnamespace
			WHERE n.nspname = $1 AND c.relkind IN ('r','p','v','m','f') ORDER BY 1`, schema)
	case "mysql":
		r, err = s.run(ctx, `SELECT table_name, CASE WHEN table_type = 'VIEW' THEN 'view' ELSE 'table' END, table_rows
			FROM information_schema.tables WHERE table_schema = ? ORDER BY 1`, schema)
	default:
		qs, qerr := quoteIdent("sqlite", schema)
		if qerr != nil {
			return nil, qerr
		}
		r, err = s.run(ctx, `SELECT name, type, -1 FROM `+qs+`.sqlite_master
			WHERE type IN ('table','view') AND name NOT LIKE 'sqlite\_%' ESCAPE '\' ORDER BY name`)
	}
	if err != nil {
		return nil, err
	}
	out := make([]dbTable, 0, len(r.Rows))
	for _, row := range r.Rows {
		n := int64(-1)
		if row[2] != nil {
			n = cellInt(row[2])
		}
		out = append(out, dbTable{Name: cellStr(row[0]), Type: cellStr(row[1]), Rows: n})
	}
	return out, nil
}

func dbDescribe(ctx context.Context, s *dbSession, schema, table string) (*dbTableInfo, error) {
	info := &dbTableInfo{Columns: []dbColInfo{}, Indexes: []dbIndex{}}
	var cols, idx *dbResult
	var err error
	switch s.profile.Driver {
	case "postgres":
		cols, err = s.run(ctx, `SELECT a.attname, format_type(a.atttypid, a.atttypmod), NOT a.attnotnull, pg_get_expr(d.adbin, d.adrelid),
				COALESCE((SELECT true FROM pg_index i WHERE i.indrelid = c.oid AND i.indisprimary AND a.attnum = ANY(i.indkey)), false)
			FROM pg_attribute a JOIN pg_class c ON c.oid = a.attrelid JOIN pg_namespace n ON n.oid = c.relnamespace
			LEFT JOIN pg_attrdef d ON d.adrelid = a.attrelid AND d.adnum = a.attnum
			WHERE n.nspname = $1 AND c.relname = $2 AND a.attnum > 0 AND NOT a.attisdropped ORDER BY a.attnum`, schema, table)
		if err == nil {
			idx, err = s.run(ctx, `SELECT ic.relname,
					array_to_string(ARRAY(SELECT a.attname FROM unnest(ix.indkey) WITH ORDINALITY AS k(n, o)
						JOIN pg_attribute a ON a.attrelid = ix.indrelid AND a.attnum = k.n ORDER BY k.o), ','),
					ix.indisunique, ix.indisprimary
				FROM pg_index ix JOIN pg_class t ON t.oid = ix.indrelid JOIN pg_class ic ON ic.oid = ix.indexrelid
				JOIN pg_namespace n ON n.oid = t.relnamespace
				WHERE n.nspname = $1 AND t.relname = $2 ORDER BY ix.indisprimary DESC, 1`, schema, table)
		}
	case "mysql":
		cols, err = s.run(ctx, `SELECT column_name, column_type, is_nullable = 'YES', column_default, column_key = 'PRI'
			FROM information_schema.columns WHERE table_schema = ? AND table_name = ? ORDER BY ordinal_position`, schema, table)
		if err == nil {
			idx, err = s.run(ctx, `SELECT index_name, GROUP_CONCAT(column_name ORDER BY seq_in_index SEPARATOR ','), MIN(non_unique) = 0, index_name = 'PRIMARY'
				FROM information_schema.statistics WHERE table_schema = ? AND table_name = ?
				GROUP BY index_name ORDER BY index_name = 'PRIMARY' DESC, index_name`, schema, table)
		}
	default:
		cols, err = s.run(ctx, `SELECT name, type, "notnull" = 0, dflt_value, pk > 0 FROM pragma_table_info(?, ?) ORDER BY cid`, table, schema)
		if err == nil {
			idx, err = s.run(ctx, `SELECT l.name, (SELECT group_concat(name, ',') FROM (SELECT ii.name FROM pragma_index_info(l.name, ?) ii ORDER BY ii.seqno)),
					l."unique", l.origin = 'pk'
				FROM pragma_index_list(?, ?) l ORDER BY l.origin = 'pk' DESC, l.name`, schema, table, schema)
		}
	}
	if err != nil {
		return nil, err
	}
	for _, r := range cols.Rows {
		c := dbColInfo{Name: cellStr(r[0]), Type: cellStr(r[1]), Nullable: cellBool(r[2]), PK: cellBool(r[4])}
		if r[3] != nil {
			d := cellStr(r[3])
			c.Default = &d
		}
		info.Columns = append(info.Columns, c)
	}
	if len(info.Columns) == 0 {
		return nil, fmt.Errorf("table %s.%s not found", schema, table)
	}
	for _, r := range idx.Rows {
		ix := dbIndex{Name: cellStr(r[0]), Unique: cellBool(r[2]), Primary: cellBool(r[3]), Columns: []string{}}
		if c := cellStr(r[1]); c != "" {
			ix.Columns = strings.Split(c, ",")
		}
		info.Indexes = append(info.Indexes, ix)
	}
	return info, nil
}

// ------------------------------------------------------------------ table grid

type dbFilter struct {
	Column string `json:"column"`
	Op     string `json:"op"` // eq ne lt gt le ge like null notnull
	Value  string `json:"value"`
}

type dbRowsReq struct {
	ID     string    `json:"id"`
	Schema string    `json:"schema"`
	Table  string    `json:"table"`
	Limit  int       `json:"limit"`
	Offset int64     `json:"offset"`
	Sort   string    `json:"sort"`
	Desc   bool      `json:"desc"`
	Filter *dbFilter `json:"filter"`
}

var dbFilterOps = map[string]string{"eq": "=", "ne": "<>", "lt": "<", "gt": ">", "le": "<=", "ge": ">="}

// buildTableFrom returns `FROM "schema"."table" [WHERE ...]` and its args.
func buildTableFrom(driver string, q dbRowsReq) (string, []any, error) {
	qt, err := quoteIdent(driver, q.Table)
	if err != nil {
		return "", nil, err
	}
	from := " FROM " + qt
	if q.Schema != "" {
		qs, err := quoteIdent(driver, q.Schema)
		if err != nil {
			return "", nil, err
		}
		from = " FROM " + qs + "." + qt
	}
	var args []any
	if f := q.Filter; f != nil && f.Column != "" {
		qc, err := quoteIdent(driver, f.Column)
		if err != nil {
			return "", nil, err
		}
		switch f.Op {
		case "null":
			from += " WHERE " + qc + " IS NULL"
		case "notnull":
			from += " WHERE " + qc + " IS NOT NULL"
		case "like":
			cast := "CAST(" + qc + " AS TEXT)"
			if driver == "mysql" {
				cast = "CAST(" + qc + " AS CHAR)"
			}
			from += " WHERE " + cast + " LIKE " + placeholder(driver, 1)
			args = append(args, "%"+f.Value+"%")
		default:
			op, ok := dbFilterOps[f.Op]
			if !ok {
				return "", nil, fmt.Errorf("unknown filter operator %q", f.Op)
			}
			from += " WHERE " + qc + " " + op + " " + placeholder(driver, 1)
			args = append(args, f.Value)
		}
	}
	return from, args, nil
}

func buildRowsSQL(driver string, q dbRowsReq) (string, []any, error) {
	if q.Limit <= 0 || q.Limit > 500 {
		q.Limit = 100
	}
	if q.Offset < 0 {
		return "", nil, errors.New("negative offset")
	}
	from, args, err := buildTableFrom(driver, q)
	if err != nil {
		return "", nil, err
	}
	sqlText := "SELECT *" + from
	if q.Sort != "" {
		qc, err := quoteIdent(driver, q.Sort)
		if err != nil {
			return "", nil, err
		}
		dir := " ASC"
		if q.Desc {
			dir = " DESC"
		}
		sqlText += " ORDER BY " + qc + dir
	}
	// one extra row tells the UI whether a next page exists
	sqlText += " LIMIT " + strconv.Itoa(q.Limit+1) + " OFFSET " + strconv.FormatInt(q.Offset, 10)
	return sqlText, args, nil
}

func dbTableRows(ctx context.Context, s *dbSession, q dbRowsReq) (*dbResult, error) {
	if q.Limit <= 0 || q.Limit > 500 {
		q.Limit = 100
	}
	sqlText, args, err := buildRowsSQL(s.profile.Driver, q)
	if err != nil {
		return nil, err
	}
	r, err := runReadOnly(ctx, s.db, s.profile.Driver, sqlText, args, q.Limit, false)
	if err != nil {
		return nil, err
	}
	return r, nil
}

func dbTableCount(ctx context.Context, s *dbSession, q dbRowsReq) (int64, error) {
	from, args, err := buildTableFrom(s.profile.Driver, q)
	if err != nil {
		return 0, err
	}
	r, err := s.run(ctx, "SELECT COUNT(*)"+from, args...)
	if err != nil {
		return 0, err
	}
	if len(r.Rows) == 0 {
		return 0, nil
	}
	return cellInt(r.Rows[0][0]), nil
}
