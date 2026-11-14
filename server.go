package main

import (
	"crypto/subtle"
	"embed"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"runtime"
	"runtime/debug"
	"strconv"
	"strings"
	"time"

	"github.com/shirou/gopsutil/v4/process"
)

//go:embed web
var webFS embed.FS

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(v)
}

func fail(w http.ResponseWriter, code int, err error) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
}

func decode(r *http.Request, v any) error {
	return json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(v)
}

// guard enforces: loopback Host header (DNS-rebinding), access token, same-origin for writes.
func guard(port int, token string, next http.Handler) http.Handler {
	hosts := map[string]bool{
		fmt.Sprintf("127.0.0.1:%d", port): true,
		fmt.Sprintf("localhost:%d", port): true,
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !hosts[r.Host] {
			http.Error(w, "forbidden host", http.StatusForbidden)
			return
		}
		if t := r.URL.Query().Get("t"); t != "" {
			if subtle.ConstantTimeCompare([]byte(t), []byte(token)) == 1 {
				http.SetCookie(w, &http.Cookie{Name: "dd_token", Value: token, Path: "/", HttpOnly: true, SameSite: http.SameSiteStrictMode})
				q := r.URL.Query()
				q.Del("t")
				u := url.URL{Path: r.URL.Path, RawQuery: q.Encode()}
				http.Redirect(w, r, u.String(), http.StatusFound)
				return
			}
		}
		ok := false
		if c, err := r.Cookie("dd_token"); err == nil && subtle.ConstantTimeCompare([]byte(c.Value), []byte(token)) == 1 {
			ok = true
		}
		if h := r.Header.Get("Authorization"); strings.HasPrefix(h, "Bearer ") &&
			subtle.ConstantTimeCompare([]byte(strings.TrimPrefix(h, "Bearer ")), []byte(token)) == 1 {
			ok = true
		}
		if !ok {
			http.Error(w, "unauthorized: open the URL printed by `devdeck` (contains ?t=...)", http.StatusUnauthorized)
			return
		}
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			if o := r.Header.Get("Origin"); o != "" && o != "http://"+r.Host {
				http.Error(w, "cross-origin request blocked", http.StatusForbidden)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

func runServe(args []string) {
	fs_ := flag.NewFlagSet("serve", flag.ExitOnError)
	port := fs_.Int("port", 7077, "port to listen on (127.0.0.1 only)")
	open := fs_.Bool("open", false, "open the dashboard in the default browser")
	_ = fs_.Parse(args)

	// Keep the footprint small: aggressive GC and a soft memory ceiling.
	debug.SetGCPercent(40)
	debug.SetMemoryLimit(48 << 20)
	go func() {
		for range time.Tick(2 * time.Minute) {
			debug.FreeOSMemory()
		}
	}()

	token := loadToken()
	mux := http.NewServeMux()
	sub, _ := fs.Sub(webFS, "web")
	mux.Handle("/", http.FileServer(http.FS(sub)))

	mux.HandleFunc("GET /api/ports", func(w http.ResponseWriter, r *http.Request) {
		ps, err := listPorts()
		if err != nil {
			fail(w, 500, err)
			return
		}
		writeJSON(w, ps)
	})
	mux.HandleFunc("GET /api/processes", func(w http.ResponseWriter, r *http.Request) {
		n, _ := strconv.Atoi(r.URL.Query().Get("limit"))
		ps, err := listProcesses(n)
		if err != nil {
			fail(w, 500, err)
			return
		}
		writeJSON(w, ps)
	})
	mux.HandleFunc("POST /api/kill", func(w http.ResponseWriter, r *http.Request) {
		var b struct {
			PID  int32 `json:"pid"`
			Tree bool  `json:"tree"`
		}
		if err := decode(r, &b); err != nil {
			fail(w, 400, err)
			return
		}
		if err := killProcess(b.PID, b.Tree); err != nil {
			fail(w, 500, err)
			return
		}
		writeJSON(w, map[string]bool{"ok": true})
	})

	mux.HandleFunc("GET /api/git/graph", func(w http.ResponseWriter, r *http.Request) {
		n, _ := strconv.Atoi(r.URL.Query().Get("limit"))
		g, err := gitGraph(r.URL.Query().Get("repo"), n)
		if err != nil {
			fail(w, 400, err)
			return
		}
		writeJSON(w, g)
	})
	mux.HandleFunc("GET /api/git/commit", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		var s string
		var err error
		if q.Get("patch") == "1" {
			s, err = gitDiff(q.Get("repo"), q.Get("hash"))
		} else {
			s, err = gitCommit(q.Get("repo"), q.Get("hash"))
		}
		if err != nil {
			fail(w, 400, err)
			return
		}
		writeJSON(w, map[string]string{"text": s})
	})

	mux.HandleFunc("POST /api/redis/connect", func(w http.ResponseWriter, r *http.Request) {
		var req redisConnReq
		if err := decode(r, &req); err != nil {
			fail(w, 400, err)
			return
		}
		id, ver, err := rconns.connect(req)
		if err != nil {
			fail(w, 400, err)
			return
		}
		writeJSON(w, map[string]string{"id": id, "version": ver})
	})
	mux.HandleFunc("POST /api/redis/close", func(w http.ResponseWriter, r *http.Request) {
		var b struct {
			ID string `json:"id"`
		}
		_ = decode(r, &b)
		rconns.close(b.ID)
		writeJSON(w, map[string]bool{"ok": true})
	})
	mux.HandleFunc("GET /api/redis/scan", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		c, err := rconns.get(q.Get("id"))
		if err != nil {
			fail(w, 400, err)
			return
		}
		cur, _ := strconv.ParseUint(q.Get("cursor"), 10, 64)
		cnt, _ := strconv.ParseInt(q.Get("count"), 10, 64)
		res, err := redisScan(c, cur, q.Get("match"), cnt)
		if err != nil {
			fail(w, 500, err)
			return
		}
		writeJSON(w, res)
	})
	mux.HandleFunc("GET /api/redis/key", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		c, err := rconns.get(q.Get("id"))
		if err != nil {
			fail(w, 400, err)
			return
		}
		kv, err := redisGet(c, q.Get("key"))
		if err != nil {
			fail(w, 500, err)
			return
		}
		writeJSON(w, kv)
	})
	mux.HandleFunc("GET /api/redis/info", func(w http.ResponseWriter, r *http.Request) {
		c, err := rconns.get(r.URL.Query().Get("id"))
		if err != nil {
			fail(w, 400, err)
			return
		}
		s, err := redisInfo(c)
		if err != nil {
			fail(w, 500, err)
			return
		}
		writeJSON(w, map[string]string{"text": s})
	})

	mux.HandleFunc("GET /api/config", func(w http.ResponseWriter, r *http.Request) { writeJSON(w, loadConfig()) })
	mux.HandleFunc("PUT /api/config", func(w http.ResponseWriter, r *http.Request) {
		var c Config
		if err := decode(r, &c); err != nil {
			fail(w, 400, err)
			return
		}
		if err := saveConfig(c); err != nil {
			fail(w, 500, err)
			return
		}
		writeJSON(w, c)
	})

	mux.HandleFunc("POST /api/hook", func(w http.ResponseWriter, r *http.Request) {
		var p map[string]any
		if err := decode(r, &p); err == nil {
			recordClaudeEvent(p)
		}
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("GET /api/claude/sessions", func(w http.ResponseWriter, r *http.Request) { writeJSON(w, listClaudeSessions()) })
	mux.HandleFunc("GET /api/self", func(w http.ResponseWriter, r *http.Request) {
		var rss uint64
		if p, err := process.NewProcess(int32(os.Getpid())); err == nil {
			if mi, err := p.MemoryInfo(); err == nil {
				rss = mi.RSS
			}
		}
		writeJSON(w, map[string]any{"rss": rss, "version": version, "os": runtime.GOOS})
	})
	mux.HandleFunc("/ws/term", handleTermWS)

	ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", *port))
	if err != nil {
		fmt.Fprintln(os.Stderr, "cannot listen:", err)
		os.Exit(1)
	}
	writePortFile(*port)
	u := fmt.Sprintf("http://127.0.0.1:%d/?t=%s", *port, token)
	fmt.Println("devdeck", version, "listening on 127.0.0.1 only")
	fmt.Println("open:", u)
	if *open {
		openBrowser(u)
	}
	srv := &http.Server{Handler: guard(*port, token, mux), ReadHeaderTimeout: 10 * time.Second}
	if err := srv.Serve(ln); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func openBrowser(u string) {
	switch runtime.GOOS {
	case "windows":
		_ = exec.Command("rundll32", "url.dll,FileProtocolHandler", u).Start()
	case "darwin":
		_ = exec.Command("open", u).Start()
	default:
		_ = exec.Command("xdg-open", u).Start()
	}
}
