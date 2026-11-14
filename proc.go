package main

import (
	"errors"
	"fmt"
	"math"
	"net/http"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/shirou/gopsutil/v4/net"
	"github.com/shirou/gopsutil/v4/process"
)

// procRow is one entry of a process snapshot (procSnapshot is OS specific).
type procRow struct {
	PID, PPID int32
	Name      string
	RSS       uint64
	CPUSec    float64 // user+kernel seconds; -1 when not read
	Create    int64   // unix ms, 0 when unknown
	Threads   int32
}

// ------------------------------------------------------------------ cmdline / cwd cache

// Command line and working directory need several syscalls per process, so they are cached per
// (pid, start time) for a minute and pruned against each snapshot. The map is hard-capped.
type procDetail struct {
	Create  int64
	Cmdline string
	Cwd     string
	at      time.Time
}

const (
	detailTTL = time.Minute
	detailMax = 512
)

var (
	detailMu    sync.Mutex
	detailCache = map[int32]procDetail{}
)

func clip(s string, n int) string {
	if len(s) > n {
		return s[:n] + "…"
	}
	return s
}

func procDetails(pid int32, create int64) procDetail {
	detailMu.Lock()
	d, ok := detailCache[pid]
	detailMu.Unlock()
	if ok && d.Create == create && time.Since(d.at) < detailTTL {
		return d
	}
	d = procDetail{Create: create, at: time.Now()}
	if p, err := process.NewProcess(pid); err == nil {
		if s, err := p.Cmdline(); err == nil {
			d.Cmdline = clip(strings.TrimSpace(s), 4096)
		}
		if s, err := p.Cwd(); err == nil { // fails (or is empty) for elevated processes on Windows
			d.Cwd = clip(s, 1024)
		}
	}
	detailMu.Lock()
	if len(detailCache) >= detailMax {
		clear(detailCache)
	}
	detailCache[pid] = d
	detailMu.Unlock()
	return d
}

func pruneDetails(alive map[int32]procRow) {
	detailMu.Lock()
	defer detailMu.Unlock()
	for pid, d := range detailCache {
		if r, ok := alive[pid]; !ok || (r.Create != 0 && d.Create != 0 && r.Create != d.Create) {
			delete(detailCache, pid)
		}
	}
}

func pruneExpiredDetails() {
	detailMu.Lock()
	defer detailMu.Unlock()
	for pid, d := range detailCache {
		if time.Since(d.at) >= detailTTL {
			delete(detailCache, pid)
		}
	}
}

// ------------------------------------------------------------------ CPU sampling

// CPU % needs two samples. The previous sample is kept only between calls of the Processes tab
// (it polls while visible); after 30 s without a call the baseline is considered stale.
type cpuMark struct {
	create int64
	sec    float64
}

var (
	cpuMu   sync.Mutex
	cpuAt   time.Time
	cpuPrev map[int32]cpuMark
)

func cpuPercent(delta, dt float64) float64 {
	if dt <= 0 || delta < 0 {
		return 0
	}
	v := delta / dt / float64(runtime.NumCPU()) * 100
	return math.Round(math.Min(v, 100)*10) / 10
}

// listProcessesCPU lists every process with CPU % since the previous call.
func listProcessesCPU() ([]ProcInfo, error) {
	snap, err := procSnapshot(true)
	if err != nil {
		return nil, err
	}
	now := time.Now()
	cpuMu.Lock()
	dt := now.Sub(cpuAt).Seconds()
	valid := dt >= 0.5 && dt <= 30
	keep := dt < 0.5 // two callers in quick succession: reuse the older baseline
	var next map[int32]cpuMark
	if !keep {
		next = make(map[int32]cpuMark, len(snap))
	}
	out := make([]ProcInfo, 0, len(snap))
	for _, r := range snap {
		pi := ProcInfo{PID: r.PID, PPID: r.PPID, Name: r.Name, RSS: r.RSS, CPU: -1, Started: r.Create, Threads: r.Threads}
		if r.CPUSec >= 0 && r.PID != 0 { // the idle process is not load
			if m, ok := cpuPrev[r.PID]; ok && m.create == r.Create && (valid || keep) && dt > 0 {
				pi.CPU = cpuPercent(r.CPUSec-m.sec, dt)
			}
			if next != nil {
				next[r.PID] = cpuMark{r.Create, r.CPUSec}
			}
		}
		out = append(out, pi)
	}
	if !keep {
		cpuAt, cpuPrev = now, next
	}
	cpuMu.Unlock()
	sortProcs(out)
	return out, nil
}

// ------------------------------------------------------------------ details drawer

type procDetailResp struct {
	PID      int32      `json:"pid"`
	PPID     int32      `json:"ppid"`
	Name     string     `json:"name"`
	Parent   string     `json:"parent,omitempty"`
	Started  int64      `json:"started,omitempty"`
	RSS      uint64     `json:"rss"`
	CPU      float64    `json:"cpu"`
	Threads  int32      `json:"threads,omitempty"`
	Cmdline  string     `json:"cmdline,omitempty"`
	Cwd      string     `json:"cwd,omitempty"`
	Kind     string     `json:"kind,omitempty"`
	KindCat  string     `json:"kindCat,omitempty"`
	Children []ProcInfo `json:"children,omitempty"`
	Ports    []PortInfo `json:"ports,omitempty"`
	At       int64      `json:"at"`
}

// One drawer at a time: remember the previous CPU reading of the process it shows.
var (
	drawerMu   sync.Mutex
	drawerPrev struct {
		pid    int32
		create int64
		sec    float64
		at     time.Time
	}
)

func drawerCPU(pid int32, create int64, sec float64, now time.Time) float64 {
	drawerMu.Lock()
	defer drawerMu.Unlock()
	p := drawerPrev
	cpu := -1.0
	if sec >= 0 && p.pid == pid && p.create == create {
		if dt := now.Sub(p.at).Seconds(); dt >= 0.5 && dt <= 30 {
			cpu = cpuPercent(sec-p.sec, dt)
		} else if dt < 0.5 {
			return -1 // too soon: keep the older baseline
		}
	}
	drawerPrev.pid, drawerPrev.create, drawerPrev.sec, drawerPrev.at = pid, create, sec, now
	return cpu
}

var errNoProc = errors.New("process not found (it may have exited)")

// procSample is the cheap path used every 2 s while the drawer is open: one process, no snapshot.
func procSample(pid int32) (*procDetailResp, error) {
	p, err := process.NewProcess(pid)
	if err != nil {
		return nil, errNoProc
	}
	now := time.Now()
	r := &procDetailResp{PID: pid, At: now.UnixMilli(), CPU: -1}
	r.Started, _ = p.CreateTime()
	sec := -1.0
	if t, err := p.Times(); err == nil && t != nil {
		sec = t.User + t.System
	}
	if mi, err := p.MemoryInfo(); err == nil && mi != nil {
		r.RSS = mi.RSS
	}
	if sec < 0 || r.RSS == 0 { // access denied: fall back to the snapshot, which needs no handle
		snap, err := procSnapshot(true)
		if err == nil {
			if row, ok := snap[pid]; ok {
				r.RSS, sec, r.Started = row.RSS, row.CPUSec, row.Create
			}
		}
	}
	r.CPU = drawerCPU(pid, r.Started, sec, now)
	return r, nil
}

func procFull(pid int32) (*procDetailResp, error) {
	snap, err := procSnapshot(true)
	if err != nil {
		return nil, err
	}
	row, ok := snap[pid]
	if !ok {
		return nil, errNoProc
	}
	now := time.Now()
	r := &procDetailResp{PID: pid, PPID: row.PPID, Name: row.Name, Started: row.Create, RSS: row.RSS, Threads: row.Threads, At: now.UnixMilli()}
	if par, ok := snap[row.PPID]; ok && row.PPID != pid && (par.Create == 0 || row.Create == 0 || par.Create <= row.Create) {
		r.Parent = par.Name
	}
	if pid > 4 {
		d := procDetails(pid, row.Create)
		r.Cmdline, r.Cwd = d.Cmdline, d.Cwd
	}
	r.Kind, r.KindCat = devKind(r.Name, r.Cmdline)
	r.CPU = drawerCPU(pid, row.Create, row.CPUSec, now)
	for _, c := range snap {
		if c.PPID == pid && c.PID != pid && (row.Create == 0 || c.Create >= row.Create) {
			r.Children = append(r.Children, ProcInfo{PID: c.PID, PPID: c.PPID, Name: c.Name, RSS: c.RSS, CPU: -1, Started: c.Create})
		}
	}
	sortProcs(r.Children)
	if len(r.Children) > 200 {
		r.Children = r.Children[:200]
	}
	if conns, err := net.Connections("tcp"); err == nil {
		seen := map[string]bool{}
		for _, c := range conns {
			if c.Pid != pid || c.Type != syscall.SOCK_STREAM || c.Status != "LISTEN" {
				continue
			}
			k := fmt.Sprintf("%s:%d", c.Laddr.IP, c.Laddr.Port)
			if !seen[k] {
				seen[k] = true
				r.Ports = append(r.Ports, PortInfo{Port: c.Laddr.Port, Addr: c.Laddr.IP, PID: pid, Process: row.Name})
			}
		}
		sort.Slice(r.Ports, func(i, j int) bool { return r.Ports[i].Port < r.Ports[j].Port })
	}
	return r, nil
}

// ------------------------------------------------------------------ dev server labels

type kindRule struct {
	kind, cat string
	names     []string // process name, lower case, without .exe
	tokens    []string // whole tokens of the command line
	subs      []string // substrings of the lower-case command line
}

// First match wins: frameworks before services, runtimes last.
var kindRules = []kindRule{
	{kind: "Vite", cat: "web", tokens: []string{"vite"}},
	{kind: "Next.js", cat: "web", tokens: []string{"next", "next-server", "next-router-worker"}},
	{kind: "Nuxt", cat: "web", tokens: []string{"nuxt", "nuxi"}},
	{kind: "Astro", cat: "web", tokens: []string{"astro"}},
	{kind: "Remix", cat: "web", tokens: []string{"remix", "remix-serve"}},
	{kind: "SvelteKit", cat: "web", tokens: []string{"svelte-kit"}},
	{kind: "Angular", cat: "web", subs: []string{"ng serve", "@angular"}},
	{kind: "CRA", cat: "web", tokens: []string{"react-scripts"}},
	{kind: "webpack", cat: "web", tokens: []string{"webpack", "webpack-dev-server"}},
	{kind: "Parcel", cat: "web", tokens: []string{"parcel"}},
	{kind: "Storybook", cat: "web", tokens: []string{"storybook"}},
	{kind: "Gatsby", cat: "web", tokens: []string{"gatsby"}},
	{kind: "nodemon", cat: "web", tokens: []string{"nodemon"}},
	{kind: "Uvicorn", cat: "web", names: []string{"uvicorn"}, tokens: []string{"uvicorn"}},
	{kind: "Gunicorn", cat: "web", names: []string{"gunicorn"}, tokens: []string{"gunicorn"}},
	{kind: "Hypercorn", cat: "web", tokens: []string{"hypercorn"}},
	{kind: "Django", cat: "web", subs: []string{"manage.py runserver", "manage.py\" runserver", "daphne"}},
	{kind: "Flask", cat: "web", tokens: []string{"flask"}},
	{kind: "Streamlit", cat: "web", tokens: []string{"streamlit"}},
	{kind: "Jupyter", cat: "web", tokens: []string{"jupyter", "jupyter-lab", "jupyter-notebook", "ipykernel_launcher"}},
	{kind: "http.server", cat: "web", subs: []string{"-m http.server", "simplehttpserver"}},
	{kind: "Spring Boot", cat: "web", subs: []string{"spring-boot", "org.springframework", "springboot"}},
	{kind: "Rails", cat: "web", tokens: []string{"rails", "puma"}},
	{kind: "Laravel", cat: "web", subs: []string{"artisan serve"}},
	{kind: "Hugo", cat: "web", names: []string{"hugo"}},
	{kind: "IIS Express", cat: "web", names: []string{"iisexpress"}},
	{kind: ".NET", cat: "web", subs: []string{"dotnet watch", "dotnet-watch", `bin\debug\net`, "bin/debug/net", `bin\release\net`, "bin/release/net"}},
	{kind: "Go", cat: "web", subs: []string{"go-build"}},

	{kind: "PostgreSQL", cat: "db", names: []string{"postgres", "pg_ctl"}},
	{kind: "MySQL", cat: "db", names: []string{"mysqld"}},
	{kind: "MariaDB", cat: "db", names: []string{"mariadbd"}},
	{kind: "Redis", cat: "db", names: []string{"redis-server", "valkey-server", "memurai"}},
	{kind: "MongoDB", cat: "db", names: []string{"mongod", "mongos"}},
	{kind: "SQL Server", cat: "db", names: []string{"sqlservr"}},
	{kind: "Memcached", cat: "db", names: []string{"memcached"}},
	{kind: "Elasticsearch", cat: "db", subs: []string{"elasticsearch", "opensearch"}},
	{kind: "Kafka", cat: "db", subs: []string{"kafka.kafka", "kafka-server"}},
	{kind: "RabbitMQ", cat: "db", subs: []string{"rabbit"}},
	{kind: "ClickHouse", cat: "db", names: []string{"clickhouse", "clickhouse-server"}},
	{kind: "InfluxDB", cat: "db", names: []string{"influxd"}},
	{kind: "etcd", cat: "db", names: []string{"etcd"}},
	{kind: "MinIO", cat: "db", names: []string{"minio"}},

	{kind: "Docker", cat: "infra", names: []string{"com.docker.backend", "com.docker.proxy", "dockerd", "docker-proxy", "docker", "vpnkit", "containerd"}},
	{kind: "WSL", cat: "infra", names: []string{"wslrelay", "wslhost"}},
	{kind: "nginx", cat: "infra", names: []string{"nginx"}},
	{kind: "Apache", cat: "infra", names: []string{"httpd", "apache2"}},
	{kind: "Caddy", cat: "infra", names: []string{"caddy"}},
	{kind: "Traefik", cat: "infra", names: []string{"traefik"}},
	{kind: "HAProxy", cat: "infra", names: []string{"haproxy"}},
	{kind: "Ollama", cat: "infra", names: []string{"ollama", "ollama app"}},
	{kind: "kubectl", cat: "infra", names: []string{"kubectl"}},
	{kind: "SSH", cat: "infra", names: []string{"ssh", "sshd"}},
	{kind: "devdeck", cat: "infra", names: []string{"devdeck"}},

	{kind: ".NET", cat: "runtime", names: []string{"dotnet"}},
	{kind: "Deno", cat: "runtime", names: []string{"deno"}},
	{kind: "Bun", cat: "runtime", names: []string{"bun"}},
	{kind: "Node.js", cat: "runtime", names: []string{"node", "nodejs"}},
	{kind: "Python", cat: "runtime", names: []string{"python", "python3", "pythonw", "py"}},
	{kind: "Java", cat: "runtime", names: []string{"java", "javaw"}},
	{kind: "Go", cat: "runtime", names: []string{"go"}},
	{kind: "Ruby", cat: "runtime", names: []string{"ruby"}},
	{kind: "PHP", cat: "runtime", names: []string{"php", "php-cgi", "php-fpm"}},
}

var tokenSplit = func(r rune) bool {
	switch r {
	case ' ', '\t', '/', '\\', '"', '\'', '=', ',', ':', '@':
		return true
	}
	return false
}

func trimExt(s string) string {
	for _, e := range []string{".exe", ".js", ".mjs", ".cjs", ".cmd", ".py", ".ps1"} {
		s = strings.TrimSuffix(s, e)
	}
	return s
}

// devKind labels well-known dev servers and services from the process name and command line.
func devKind(name, cmdline string) (kind, cat string) {
	n := trimExt(strings.ToLower(strings.TrimSpace(name)))
	lc := strings.ToLower(cmdline)
	toks := map[string]bool{}
	for _, t := range strings.FieldsFunc(lc, tokenSplit) {
		toks[trimExt(t)] = true
	}
	// Python version suffixes: python3.12 -> python
	if strings.HasPrefix(n, "python") {
		n = strings.TrimRight(n, "0123456789.")
	}
	for _, r := range kindRules {
		for _, x := range r.names {
			if n == x {
				return r.kind, r.cat
			}
		}
		for _, x := range r.tokens {
			if toks[x] {
				return r.kind, r.cat
			}
		}
		for _, x := range r.subs {
			if strings.Contains(lc, x) {
				return r.kind, r.cat
			}
		}
	}
	return "", ""
}

// ------------------------------------------------------------------ routes + MCP

func init() {
	addRoutes(func(mux *http.ServeMux) {
		// Processes tab: every process with CPU % since the previous call (only polled while visible).
		mux.HandleFunc("GET /api/procs", func(w http.ResponseWriter, r *http.Request) {
			ps, err := listProcessesCPU()
			if err != nil {
				fail(w, 500, err)
				return
			}
			writeJSON(w, ps)
		})
		// Details drawer: ?pid=N (full) or ?pid=N&lite=1 (RAM/CPU sample only, polled every 2 s while open).
		mux.HandleFunc("GET /api/procs/detail", func(w http.ResponseWriter, r *http.Request) {
			pid, err := strconv.Atoi(r.URL.Query().Get("pid"))
			if err != nil || pid < 0 || pid > math.MaxInt32 {
				fail(w, 400, errors.New("bad pid"))
				return
			}
			var d *procDetailResp
			if r.URL.Query().Get("lite") == "1" {
				d, err = procSample(int32(pid))
			} else {
				d, err = procFull(int32(pid))
			}
			if err != nil {
				code := 500
				if errors.Is(err, errNoProc) {
					code = 404
				}
				fail(w, code, err)
				return
			}
			writeJSON(w, d)
		})
	})

	registerMCPTool(tool{Name: "port_owner", Description: "Who is listening on a TCP port: process, PID, command line, working directory, start time and dev-server type.",
		InputSchema: obj(map[string]any{"port": typ("integer", "TCP port, e.g. 3000")}, "port"),
		handler: func(a map[string]any) (string, error) {
			port := argInt(a, "port", 0)
			if port <= 0 || port > 65535 {
				return "", errors.New("port must be 1-65535")
			}
			return portOwnerText(port)
		}})
}

func portOwnerText(port int) (string, error) {
	ps, err := listPorts()
	if err != nil {
		return "", err
	}
	var sb strings.Builder
	for _, p := range ps {
		if int(p.Port) != port {
			continue
		}
		fmt.Fprintf(&sb, "%s:%d\tpid=%d\tppid=%d\t%s", p.Addr, p.Port, p.PID, p.PPID, p.Process)
		if p.Kind != "" {
			fmt.Fprintf(&sb, " (%s)", p.Kind)
		}
		sb.WriteByte('\n')
		if p.Started > 0 {
			fmt.Fprintf(&sb, "  started: %s\n", time.UnixMilli(p.Started).Format(time.RFC3339))
		}
		if p.Cmdline != "" {
			fmt.Fprintf(&sb, "  cmdline: %s\n", p.Cmdline)
		}
		if p.Cwd != "" {
			fmt.Fprintf(&sb, "  cwd: %s\n", p.Cwd)
		}
	}
	if sb.Len() == 0 {
		return fmt.Sprintf("nothing is listening on TCP port %d", port), nil
	}
	return sb.String(), nil
}
