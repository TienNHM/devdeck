package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
)

func init() {
	addRoutes(registerDBRoutes)
	registerDBMCPTools()
}

func registerDBRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/db/profiles", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, loadDBProfiles())
	})
	mux.HandleFunc("PUT /api/db/profiles", func(w http.ResponseWriter, r *http.Request) {
		var in []dbProfile
		if err := decode(r, &in); err != nil {
			fail(w, 400, err)
			return
		}
		ps, err := cleanDBProfiles(in)
		if err != nil {
			fail(w, 400, err)
			return
		}
		if err := saveJSON("db", dbProfileStore{Profiles: ps}); err != nil {
			fail(w, 500, err)
			return
		}
		writeJSON(w, ps)
	})
	mux.HandleFunc("POST /api/db/open", func(w http.ResponseWriter, r *http.Request) {
		var b struct {
			Profile  dbProfile `json:"profile"`
			Password string    `json:"password"`
		}
		if err := decode(r, &b); err != nil {
			fail(w, 400, err)
			return
		}
		s, err := dbconns.open(b.Profile, b.Password)
		if err != nil {
			fail(w, 400, err)
			return
		}
		writeJSON(w, map[string]any{"id": s.id, "driver": s.profile.Driver, "version": s.version, "schema": defaultSchema(s.profile)})
	})
	mux.HandleFunc("POST /api/db/close", func(w http.ResponseWriter, r *http.Request) {
		var b struct {
			ID string `json:"id"`
		}
		_ = decode(r, &b)
		dbconns.close(b.ID)
		writeJSON(w, map[string]bool{"ok": true})
	})
	mux.HandleFunc("GET /api/db/schemas", func(w http.ResponseWriter, r *http.Request) {
		s, err := dbconns.get(r.URL.Query().Get("id"))
		if err != nil {
			fail(w, 400, err)
			return
		}
		out, err := dbSchemas(r.Context(), s)
		if err != nil {
			fail(w, 500, err)
			return
		}
		writeJSON(w, out)
	})
	mux.HandleFunc("GET /api/db/tables", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		s, err := dbconns.get(q.Get("id"))
		if err != nil {
			fail(w, 400, err)
			return
		}
		out, err := dbTables(r.Context(), s, q.Get("schema"))
		if err != nil {
			fail(w, 500, err)
			return
		}
		writeJSON(w, out)
	})
	mux.HandleFunc("GET /api/db/table", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		s, err := dbconns.get(q.Get("id"))
		if err != nil {
			fail(w, 400, err)
			return
		}
		out, err := dbDescribe(r.Context(), s, q.Get("schema"), q.Get("table"))
		if err != nil {
			fail(w, 500, err)
			return
		}
		writeJSON(w, out)
	})
	mux.HandleFunc("POST /api/db/rows", func(w http.ResponseWriter, r *http.Request) {
		var q dbRowsReq
		if err := decode(r, &q); err != nil {
			fail(w, 400, err)
			return
		}
		s, err := dbconns.get(q.ID)
		if err != nil {
			fail(w, 400, err)
			return
		}
		out, err := dbTableRows(r.Context(), s, q)
		if err != nil {
			fail(w, 400, err)
			return
		}
		writeJSON(w, out)
	})
	mux.HandleFunc("POST /api/db/count", func(w http.ResponseWriter, r *http.Request) {
		var q dbRowsReq
		if err := decode(r, &q); err != nil {
			fail(w, 400, err)
			return
		}
		s, err := dbconns.get(q.ID)
		if err != nil {
			fail(w, 400, err)
			return
		}
		n, err := dbTableCount(r.Context(), s, q)
		if err != nil {
			fail(w, 400, err)
			return
		}
		writeJSON(w, map[string]int64{"count": n})
	})
	mux.HandleFunc("POST /api/db/query", func(w http.ResponseWriter, r *http.Request) {
		var b struct {
			ID  string `json:"id"`
			SQL string `json:"sql"`
		}
		if err := decode(r, &b); err != nil {
			fail(w, 400, err)
			return
		}
		s, err := dbconns.get(b.ID)
		if err != nil {
			fail(w, 400, err)
			return
		}
		out, err := runReadOnly(r.Context(), s.db, s.profile.Driver, b.SQL, nil, dbRowCap, true)
		if err != nil {
			fail(w, 400, err)
			return
		}
		writeJSON(w, out)
	})
}

// ------------------------------------------------------------------ MCP

const mcpDBRowCap = 500

// withProfileSession opens a short-lived session for a saved profile (password from DEVDECK_DB_PASSWORD).
func withProfileSession(name string, fn func(s *dbSession) (string, error)) (string, error) {
	if name == "" {
		return "", errors.New("profile is required (a name saved in the Database tab)")
	}
	p, err := findDBProfile(name)
	if err != nil {
		return "", err
	}
	db, ver, err := openDB(p, os.Getenv("DEVDECK_DB_PASSWORD"))
	if err != nil {
		return "", err
	}
	defer db.Close()
	_ = p.normalize(false)
	return fn(&dbSession{profile: p, db: db, version: ver})
}

func formatResultTSV(r *dbResult) string {
	var sb strings.Builder
	for i, c := range r.Columns {
		if i > 0 {
			sb.WriteByte('\t')
		}
		sb.WriteString(c.Name)
	}
	sb.WriteByte('\n')
	clean := strings.NewReplacer("\t", " ", "\r", " ", "\n", " ")
	for _, row := range r.Rows {
		for i, v := range row {
			if i > 0 {
				sb.WriteByte('\t')
			}
			if v == nil {
				sb.WriteString("NULL")
			} else {
				sb.WriteString(clean.Replace(fmt.Sprint(v)))
			}
		}
		sb.WriteByte('\n')
	}
	fmt.Fprintf(&sb, "(%d rows%s, %d ms)\n", len(r.Rows), map[bool]string{true: ", truncated", false: ""}[r.Truncated], r.ElapsedMs)
	return sb.String()
}

func registerDBMCPTools() {
	registerMCPTool(tool{Name: "db_query",
		Description: fmt.Sprintf("Run one read-only SQL statement (SELECT/WITH/SHOW/EXPLAIN…) against a database profile saved in the devdeck Database tab. Runs in a read-only transaction that is rolled back; max %d rows, 15 s timeout. Password comes from env DEVDECK_DB_PASSWORD.", mcpDBRowCap),
		InputSchema: obj(map[string]any{"profile": typ("string", "saved profile name"), "sql": typ("string", "one read-only SQL statement")}, "profile", "sql"),
		handler: func(a map[string]any) (string, error) {
			return withProfileSession(argStr(a, "profile", ""), func(s *dbSession) (string, error) {
				r, err := runReadOnly(context.Background(), s.db, s.profile.Driver, argStr(a, "sql", ""), nil, mcpDBRowCap, true)
				if err != nil {
					return "", err
				}
				return formatResultTSV(r), nil
			})
		}})
	registerMCPTool(tool{Name: "db_tables",
		Description: "List schemas, tables and views of a database profile saved in the devdeck Database tab. Password comes from env DEVDECK_DB_PASSWORD.",
		InputSchema: obj(map[string]any{"profile": typ("string", "saved profile name"), "schema": typ("string", "only this schema (optional)")}, "profile"),
		handler: func(a map[string]any) (string, error) {
			return withProfileSession(argStr(a, "profile", ""), func(s *dbSession) (string, error) {
				ctx := context.Background()
				schemas := []string{argStr(a, "schema", "")}
				if schemas[0] == "" {
					var err error
					if schemas, err = dbSchemas(ctx, s); err != nil {
						return "", err
					}
				}
				var sb strings.Builder
				fmt.Fprintf(&sb, "%s (%s)\n", s.profile.Name, s.version)
				n := 0
				for _, sc := range schemas {
					ts, err := dbTables(ctx, s, sc)
					if err != nil {
						return "", err
					}
					for _, t := range ts {
						if n >= 2000 {
							sb.WriteString("... more tables not shown\n")
							return sb.String(), nil
						}
						fmt.Fprintf(&sb, "%s.%s\t%s\t~%d rows\n", sc, t.Name, t.Type, t.Rows)
						n++
					}
				}
				return sb.String(), nil
			})
		}})
}
