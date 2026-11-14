package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/crypto/ssh"
)

// SSH local port forwards: 127.0.0.1:LocalPort → RemoteHost:RemotePort through a saved or
// ~/.ssh/config host. Definitions persist in ~/.devdeck/tunnels.json; running state and any
// password live in memory only. Nothing runs until the user starts a tunnel, and auto-reconnect
// is lazy: a dropped SSH connection is re-dialled when the next local connection arrives.

type tunnelDef struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	Host          string `json:"host"`
	LocalPort     int    `json:"localPort"`
	RemoteHost    string `json:"remoteHost"`
	RemotePort    int    `json:"remotePort"`
	AutoReconnect bool   `json:"autoReconnect"`
}

type tunnelStatus struct {
	tunnelDef
	State    string `json:"state"` // stopped | running | idle | error
	Error    string `json:"error,omitempty"`
	Since    int64  `json:"since,omitempty"`
	Active   int64  `json:"active"`
	Total    int64  `json:"total"`
	BytesIn  int64  `json:"bytesIn"`  // remote → local
	BytesOut int64  `json:"bytesOut"` // local → remote
}

const (
	tunnelMaxDefs  = 32
	tunnelMaxConns = 128
)

type tunnelRun struct {
	def      tunnelDef
	password string
	ln       net.Listener
	started  time.Time

	dialMu  sync.Mutex
	mu      sync.Mutex
	client  *ssh.Client
	dialing bool
	stopped bool
	conns   map[net.Conn]struct{}
	lastErr string

	active, total, in, out atomic.Int64
}

type tunnelManager struct {
	mu      sync.Mutex
	loaded  bool
	defs    []tunnelDef
	runs    map[string]*tunnelRun
	lastErr map[string]string // why a tunnel stopped by itself
}

var tunnels = &tunnelManager{runs: map[string]*tunnelRun{}, lastErr: map[string]string{}}

var remoteHostRe = regexp.MustCompile(`^[A-Za-z0-9._:-]{1,253}$`)

func (m *tunnelManager) loadLocked() {
	if m.loaded {
		return
	}
	m.loaded = true
	_ = loadJSON("tunnels", &m.defs)
}

func (m *tunnelManager) find(id string) (tunnelDef, bool) {
	for _, d := range m.defs {
		if d.ID == id {
			return d, true
		}
	}
	return tunnelDef{}, false
}

func validateTunnel(d *tunnelDef) error {
	d.Name = strings.TrimSpace(d.Name)
	d.Host = strings.TrimSpace(d.Host)
	d.RemoteHost = strings.Trim(strings.TrimSpace(d.RemoteHost), "[]")
	if d.Host == "" || len(d.Host) > 100 {
		return errors.New("pick the SSH host to tunnel through")
	}
	if d.LocalPort < 1 || d.LocalPort > 65535 || d.RemotePort < 1 || d.RemotePort > 65535 {
		return errors.New("ports must be 1-65535")
	}
	if d.RemoteHost == "" {
		d.RemoteHost = "127.0.0.1"
	}
	if !remoteHostRe.MatchString(d.RemoteHost) {
		return errors.New("remote host must be a host name or IP address")
	}
	if d.Name == "" {
		d.Name = fmt.Sprintf("%d → %s:%d", d.LocalPort, d.RemoteHost, d.RemotePort)
	}
	if len(d.Name) > 80 {
		d.Name = clip(d.Name, 80)
	}
	return nil
}

// save creates or updates a definition. Editing a running tunnel stops it first.
func (m *tunnelManager) save(d tunnelDef) (tunnelDef, error) {
	if err := validateTunnel(&d); err != nil {
		return d, err
	}
	m.mu.Lock()
	m.loadLocked()
	for _, o := range m.defs {
		if o.ID != d.ID && o.LocalPort == d.LocalPort {
			m.mu.Unlock()
			return d, fmt.Errorf("local port %d is already used by tunnel %q", d.LocalPort, o.Name)
		}
	}
	idx := -1
	for i, o := range m.defs {
		if d.ID != "" && o.ID == d.ID {
			idx = i
		}
	}
	if idx < 0 {
		if len(m.defs) >= tunnelMaxDefs {
			m.mu.Unlock()
			return d, errors.New("too many tunnels")
		}
		d.ID = randID(6)
		m.defs = append(m.defs, d)
	} else {
		m.defs[idx] = d
	}
	defs := append([]tunnelDef(nil), m.defs...)
	run := m.runs[d.ID]
	m.mu.Unlock()
	if run != nil {
		m.stop(d.ID)
	}
	return d, saveJSON("tunnels", defs)
}

func (m *tunnelManager) remove(id string) error {
	m.stop(id)
	m.mu.Lock()
	m.loadLocked()
	out := m.defs[:0]
	for _, d := range m.defs {
		if d.ID != id {
			out = append(out, d)
		}
	}
	m.defs = out
	delete(m.lastErr, id)
	defs := append([]tunnelDef(nil), m.defs...)
	m.mu.Unlock()
	return saveJSON("tunnels", defs)
}

func (m *tunnelManager) list() []tunnelStatus {
	m.mu.Lock()
	m.loadLocked()
	out := make([]tunnelStatus, 0, len(m.defs))
	for _, d := range m.defs {
		st := tunnelStatus{tunnelDef: d, State: "stopped", Error: m.lastErr[d.ID]}
		if st.Error != "" {
			st.State = "error"
		}
		if r := m.runs[d.ID]; r != nil {
			r.mu.Lock()
			st.State, st.Error = "running", r.lastErr
			if r.dialing {
				st.State = "connecting"
			} else if r.client == nil {
				st.State = "idle"
			}
			r.mu.Unlock()
			st.Since = r.started.Unix()
			st.Active, st.Total, st.BytesIn, st.BytesOut = r.active.Load(), r.total.Load(), r.in.Load(), r.out.Load()
		}
		out = append(out, st)
	}
	m.mu.Unlock()
	sort.SliceStable(out, func(i, j int) bool { return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name) })
	return out
}

// start listens on 127.0.0.1:LocalPort and connects to the SSH host. The password, if any,
// stays in memory for reconnects while the tunnel runs.
func (m *tunnelManager) start(id, password string) error {
	m.mu.Lock()
	m.loadLocked()
	d, ok := m.find(id)
	if !ok {
		m.mu.Unlock()
		return errors.New("unknown tunnel")
	}
	if m.runs[id] != nil {
		m.mu.Unlock()
		return errors.New("tunnel is already running")
	}
	r := &tunnelRun{def: d, password: password, conns: map[net.Conn]struct{}{}, started: time.Now()}
	m.runs[id] = r // reserve while connecting
	delete(m.lastErr, id)
	m.mu.Unlock()

	failStart := func(err error) error {
		m.mu.Lock()
		delete(m.runs, id)
		m.mu.Unlock()
		return err
	}
	ln, err := net.Listen("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(d.LocalPort)))
	if err != nil {
		return failStart(fmt.Errorf("cannot listen on 127.0.0.1:%d: %w", d.LocalPort, err))
	}
	r.ln = ln
	if _, err := r.connect(); err != nil {
		ln.Close()
		return failStart(err)
	}
	go r.acceptLoop(m)
	return nil
}

func (m *tunnelManager) stop(id string) {
	m.mu.Lock()
	r := m.runs[id]
	delete(m.runs, id)
	m.mu.Unlock()
	if r != nil {
		r.shutdown()
	}
}

// connect returns the live SSH client, dialling it if needed (one dial at a time).
func (r *tunnelRun) connect() (*ssh.Client, error) {
	r.dialMu.Lock()
	defer r.dialMu.Unlock()
	r.mu.Lock()
	if r.stopped {
		r.mu.Unlock()
		return nil, errors.New("tunnel stopped")
	}
	if c := r.client; c != nil {
		r.mu.Unlock()
		return c, nil
	}
	r.dialing = true
	r.mu.Unlock()

	t, err := resolveSSHProfile(r.def.Host)
	var c *ssh.Client
	if err == nil {
		t.Password = r.password
		c, err = dialSSH(t, func(string) {})
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.dialing = false
	if err != nil {
		r.lastErr = err.Error()
		return nil, err
	}
	if r.stopped {
		c.Close()
		return nil, errors.New("tunnel stopped")
	}
	r.client, r.lastErr = c, ""
	go r.watch(c)
	return c, nil
}

// watch notices a dropped SSH connection: with auto-reconnect the tunnel stays up and re-dials
// on demand; otherwise it stops and remembers why.
func (r *tunnelRun) watch(c *ssh.Client) {
	_ = c.Wait()
	r.mu.Lock()
	if r.client == c {
		r.client = nil
	}
	stopped, auto := r.stopped, r.def.AutoReconnect
	if !stopped {
		r.lastErr = "SSH connection lost"
	}
	r.mu.Unlock()
	if !stopped && !auto {
		tunnels.mu.Lock()
		if tunnels.runs[r.def.ID] == r {
			delete(tunnels.runs, r.def.ID)
			tunnels.lastErr[r.def.ID] = "SSH connection lost"
		}
		tunnels.mu.Unlock()
		r.shutdown()
	}
}

func (r *tunnelRun) shutdown() {
	r.mu.Lock()
	r.stopped = true
	c := r.client
	r.client = nil
	conns := r.conns
	r.conns = map[net.Conn]struct{}{}
	r.mu.Unlock()
	if r.ln != nil {
		r.ln.Close()
	}
	for cn := range conns {
		cn.Close()
	}
	if c != nil {
		c.Close()
	}
}

func (r *tunnelRun) acceptLoop(m *tunnelManager) {
	for {
		lc, err := r.ln.Accept()
		if err != nil {
			r.mu.Lock()
			stopped := r.stopped
			r.mu.Unlock()
			if !stopped {
				m.mu.Lock()
				if m.runs[r.def.ID] == r {
					delete(m.runs, r.def.ID)
					m.lastErr[r.def.ID] = "listener failed: " + err.Error()
				}
				m.mu.Unlock()
				r.shutdown()
			}
			return
		}
		if r.active.Load() >= tunnelMaxConns {
			lc.Close()
			continue
		}
		go r.handle(lc)
	}
}

func (r *tunnelRun) track(c net.Conn, add bool) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if add {
		if r.stopped {
			return false
		}
		r.conns[c] = struct{}{}
	} else {
		delete(r.conns, c)
	}
	return true
}

func (r *tunnelRun) handle(lc net.Conn) {
	r.active.Add(1)
	r.total.Add(1)
	defer r.active.Add(-1)
	defer lc.Close()
	if !r.track(lc, true) {
		return
	}
	defer r.track(lc, false)

	dialRemote := func(c *ssh.Client) (net.Conn, error) {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		return c.DialContext(ctx, "tcp", net.JoinHostPort(r.def.RemoteHost, strconv.Itoa(r.def.RemotePort)))
	}
	client, err := r.connect()
	if err != nil {
		return
	}
	rc, err := dialRemote(client)
	if err != nil {
		r.mu.Lock()
		r.lastErr = "remote " + net.JoinHostPort(r.def.RemoteHost, strconv.Itoa(r.def.RemotePort)) + ": " + err.Error()
		r.mu.Unlock()
		return
	}
	defer rc.Close()
	if !r.track(rc, true) {
		return
	}
	defer r.track(rc, false)
	pipeConns(lc, rc, &r.out, &r.in)
}

type countWriter struct {
	w io.Writer
	n *atomic.Int64
}

func (c countWriter) Write(p []byte) (int, error) {
	n, err := c.w.Write(p)
	c.n.Add(int64(n))
	return n, err
}

// pipeConns copies both ways, half-closing each side when its source ends.
func pipeConns(a, b net.Conn, aToB, bToA *atomic.Int64) {
	type closeWriter interface{ CloseWrite() error }
	var wg sync.WaitGroup
	cp := func(dst, src net.Conn, n *atomic.Int64) {
		defer wg.Done()
		_, err := io.Copy(countWriter{dst, n}, src)
		if cw, ok := dst.(closeWriter); ok && err == nil {
			_ = cw.CloseWrite()
		} else {
			dst.Close()
			src.Close()
		}
	}
	wg.Add(2)
	go cp(b, a, aToB)
	go cp(a, b, bToA)
	wg.Wait()
}

func init() {
	addRoutes(func(mux *http.ServeMux) {
		mux.HandleFunc("GET /api/ssh/tunnels", func(w http.ResponseWriter, r *http.Request) { writeJSON(w, tunnels.list()) })
		mux.HandleFunc("POST /api/ssh/tunnels", func(w http.ResponseWriter, r *http.Request) {
			var d tunnelDef
			if err := decode(r, &d); err != nil {
				fail(w, 400, err)
				return
			}
			d, err := tunnels.save(d)
			if err != nil {
				fail(w, 400, err)
				return
			}
			writeJSON(w, d)
		})
		mux.HandleFunc("DELETE /api/ssh/tunnels/{id}", func(w http.ResponseWriter, r *http.Request) {
			if err := tunnels.remove(r.PathValue("id")); err != nil {
				fail(w, 500, err)
				return
			}
			writeJSON(w, map[string]bool{"ok": true})
		})
		mux.HandleFunc("POST /api/ssh/tunnels/{id}/start", func(w http.ResponseWriter, r *http.Request) {
			var b struct {
				Password string `json:"password"`
			}
			_ = decode(r, &b)
			if err := tunnels.start(r.PathValue("id"), b.Password); err != nil {
				failSSH(w, err)
				return
			}
			writeJSON(w, map[string]bool{"ok": true})
		})
		mux.HandleFunc("POST /api/ssh/tunnels/{id}/stop", func(w http.ResponseWriter, r *http.Request) {
			tunnels.stop(r.PathValue("id"))
			writeJSON(w, map[string]bool{"ok": true})
		})
	})
	registerMCPTool(tool{
		Name:        "list_tunnels",
		Description: "List devdeck SSH tunnels (local port forwards) with their state, connection count and bytes.",
		InputSchema: obj(map[string]any{}),
		handler:     mcpListTunnels,
	})
}

// failSSH reports an error; auth problems get "auth": true so the UI can ask for a password.
func failSSH(w http.ResponseWriter, err error) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(400)
	_ = json.NewEncoder(w).Encode(map[string]any{"error": err.Error(), "auth": errors.Is(err, errSSHAuth)})
}

// mcpListTunnels asks the running dashboard for live state; without it, lists saved definitions.
func mcpListTunnels(map[string]any) (string, error) {
	var list []tunnelStatus
	live := false
	if port := readPortFile(); port != 0 {
		req, _ := http.NewRequest("GET", fmt.Sprintf("http://127.0.0.1:%d/api/ssh/tunnels", port), nil)
		req.Header.Set("Authorization", "Bearer "+loadToken())
		if resp, err := (&http.Client{Timeout: 3 * time.Second}).Do(req); err == nil {
			if resp.StatusCode == 200 && json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&list) == nil {
				live = true
			}
			resp.Body.Close()
		}
	}
	if !live {
		var defs []tunnelDef
		if err := loadJSON("tunnels", &defs); err != nil {
			return "", err
		}
		for _, d := range defs {
			list = append(list, tunnelStatus{tunnelDef: d, State: "stopped"})
		}
	}
	if len(list) == 0 {
		return "no tunnels defined (add them in the devdeck SSH tab)", nil
	}
	var sb strings.Builder
	if !live {
		sb.WriteString("(devdeck dashboard is not running; all tunnels are stopped)\n")
	}
	for _, t := range list {
		fmt.Fprintf(&sb, "%s\t127.0.0.1:%d -> %s:%d via %s\tstate=%s", t.Name, t.LocalPort, t.RemoteHost, t.RemotePort, t.Host, t.State)
		if t.State == "running" || t.State == "idle" {
			fmt.Fprintf(&sb, "\tactive=%d total=%d in=%dB out=%dB", t.Active, t.Total, t.BytesIn, t.BytesOut)
		}
		if t.Error != "" {
			fmt.Fprintf(&sb, "\terror=%s", t.Error)
		}
		sb.WriteString("\n")
	}
	return sb.String(), nil
}
