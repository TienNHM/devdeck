package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/user"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
	"golang.org/x/crypto/ssh/knownhosts"
)

// sshTarget is one fully resolved SSH endpoint. Secrets only live here in memory.
type sshTarget struct {
	Name       string // saved/config name, if any
	Host       string
	Port       int
	User       string
	KeyFiles   []string // explicit identity files; empty = agent + default ~/.ssh keys
	Password   string
	Passphrase string
	Jump       []sshTarget // ProxyJump hops, first hop first
}

func (t sshTarget) label() string {
	if t.Name != "" {
		return t.Name
	}
	return t.User + "@" + net.JoinHostPort(t.Host, strconv.Itoa(t.Port))
}

// errSSHAuth marks failures the user can fix by supplying a password or passphrase.
var errSSHAuth = errors.New("authentication failed")

// sshUseAgent can be switched off by tests so they never talk to the developer's real agent.
var sshUseAgent = true

// sshExtra holds per-saved-host settings that config.go's SSHHost has no field for.
type sshExtra struct {
	Jump string `json:"jump,omitempty"`
}

func loadSSHExtras() map[string]sshExtra {
	m := map[string]sshExtra{}
	_ = loadJSON("ssh-hosts", &m)
	return m
}

var jumpRe = regexp.MustCompile(`^[A-Za-z0-9._@:\[\]%,+-]*$`)

func localUser() string {
	for _, k := range []string{"USER", "USERNAME"} {
		if v := os.Getenv(k); v != "" {
			return v
		}
	}
	if u, err := user.Current(); err == nil {
		n := u.Username
		if i := strings.LastIndex(n, `\`); i >= 0 {
			n = n[i+1:]
		}
		return n
	}
	return "root"
}

// resolveSSHProfile turns a saved host name or ~/.ssh/config alias into a target (saved hosts win).
func resolveSSHProfile(name string) (sshTarget, error) { return resolveSSH(name, 0) }

func resolveSSH(name string, depth int) (sshTarget, error) {
	if depth > 6 {
		return sshTarget{}, errors.New("ProxyJump chain is too deep (loop?)")
	}
	var t sshTarget
	var jump string
	found := false
	for _, h := range loadConfig().Hosts {
		if h.Name == name {
			t = sshTarget{Name: h.Name, Host: h.Host, Port: h.Port, User: h.User}
			if h.KeyPath != "" {
				t.KeyFiles = []string{expandHome(h.KeyPath)}
			}
			jump = loadSSHExtras()[name].Jump
			found = true
			break
		}
	}
	if !found {
		cfg := loadSSHConfig()
		if cfg == nil || !cfg.hasAlias(name) {
			return t, errors.New("unknown host: " + name + " (not in saved hosts or ~/.ssh/config)")
		}
		h := cfg.lookup(name)
		t = sshTarget{Name: name, Host: h.HostName, Port: h.Port, User: h.User, KeyFiles: h.IdentityFiles}
		jump = h.ProxyJump
	}
	if t.Host == "" {
		return t, errors.New("host " + name + " has no address")
	}
	if t.Port == 0 {
		t.Port = 22
	}
	if t.User == "" {
		t.User = localUser()
	}
	hops, err := resolveJumps(jump, depth)
	if err != nil {
		return t, err
	}
	t.Jump = hops
	return t, nil
}

// resolveJumps expands a ProxyJump value ("a,user@b:2222") into flattened hops.
func resolveJumps(spec string, depth int) ([]sshTarget, error) {
	spec = strings.TrimSpace(spec)
	if spec == "" || strings.EqualFold(spec, "none") {
		return nil, nil
	}
	if !jumpRe.MatchString(spec) || len(spec) > 512 {
		return nil, errors.New("invalid ProxyJump value: " + spec)
	}
	var out []sshTarget
	for _, part := range strings.Split(spec, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		u, host, port, err := parseJumpHop(part)
		if err != nil {
			return nil, err
		}
		var hop sshTarget
		if r, rerr := resolveSSH(host, depth+1); rerr == nil {
			hop = r // a saved host or config alias; it may bring its own jumps
		} else {
			hop = sshTarget{Host: host, Port: 22}
			if cfg := loadSSHConfig(); cfg != nil { // patterns like "Host *.corp" still apply
				h := cfg.lookup(host)
				hop.Host, hop.User, hop.KeyFiles = h.HostName, h.User, h.IdentityFiles
				if h.Port != 0 {
					hop.Port = h.Port
				}
			}
			if hop.User == "" {
				hop.User = localUser()
			}
		}
		if u != "" {
			hop.User = u
		}
		if port != 0 {
			hop.Port = port
		}
		out = append(out, hop.Jump...)
		hop.Jump = nil
		out = append(out, hop)
	}
	if len(out) > 8 {
		return nil, errors.New("too many jump hosts")
	}
	return out, nil
}

// parseJumpHop splits "[user@]host[:port]" (IPv6 in brackets).
func parseJumpHop(s string) (user, host string, port int, err error) {
	if i := strings.LastIndex(s, "@"); i >= 0 {
		user, s = s[:i], s[i+1:]
	}
	host = s
	if strings.HasPrefix(s, "[") || strings.Count(s, ":") == 1 {
		h, p, e := net.SplitHostPort(s)
		if e != nil {
			return "", "", 0, fmt.Errorf("invalid jump host %q", s)
		}
		host = h
		if port, err = strconv.Atoi(p); err != nil || port < 1 || port > 65535 {
			return "", "", 0, fmt.Errorf("invalid port in jump host %q", s)
		}
	}
	if host == "" {
		return "", "", 0, errors.New("empty jump host")
	}
	return user, host, port, nil
}

func targetFromInit(in termInit) (sshTarget, error) {
	var t sshTarget
	if in.Profile != "" {
		var err error
		if t, err = resolveSSHProfile(in.Profile); err != nil {
			return t, err
		}
	} else {
		t = sshTarget{Host: in.Host, Port: in.Port, User: in.User}
		if in.KeyPath != "" {
			t.KeyFiles = []string{expandHome(in.KeyPath)}
		}
		if t.Host == "" {
			return t, errors.New("host is required")
		}
		if t.Port == 0 {
			t.Port = 22
		}
		if t.User == "" {
			t.User = localUser()
		}
	}
	t.Password, t.Passphrase = in.Password, in.Passphrase
	return t, nil
}

func expandHome(p string) string {
	if p == "~" || strings.HasPrefix(p, "~/") || strings.HasPrefix(p, `~\`) {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, p[1:])
		}
	}
	return p
}

// ---------------------------------------------------------------- host keys (TOFU)

var knownHostsMu sync.Mutex

func hostKeyCallback(warn func(string)) (ssh.HostKeyCallback, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	dir := filepath.Join(home, ".ssh")
	_ = os.MkdirAll(dir, 0o700)
	kh := filepath.Join(dir, "known_hosts")
	knownHostsMu.Lock()
	if _, err := os.Stat(kh); err != nil {
		if f, err := os.OpenFile(kh, os.O_CREATE|os.O_WRONLY, 0o600); err == nil {
			f.Close()
		}
	}
	base, err := knownhosts.New(kh)
	knownHostsMu.Unlock()
	if err != nil {
		return nil, err
	}
	return func(hostname string, remote net.Addr, key ssh.PublicKey) error {
		err := base(hostname, remote, key)
		var ke *knownhosts.KeyError
		if errors.As(err, &ke) && len(ke.Want) == 0 {
			// Trust on first use (like StrictHostKeyChecking=accept-new); mismatches are rejected.
			knownHostsMu.Lock()
			defer knownHostsMu.Unlock()
			f, ferr := os.OpenFile(kh, os.O_APPEND|os.O_WRONLY, 0o600)
			if ferr != nil {
				return ferr
			}
			defer f.Close()
			if _, werr := fmt.Fprintln(f, knownhosts.Line([]string{knownhosts.Normalize(hostname)}, key)); werr != nil {
				return werr
			}
			warn("Warning: permanently added " + hostname + " to known_hosts (" + ssh.FingerprintSHA256(key) + ")")
			return nil
		}
		if errors.As(err, &ke) {
			return fmt.Errorf("HOST KEY CHANGED for %s (now %s): refusing to connect; if the change is expected, remove the old entry from %s", hostname, ssh.FingerprintSHA256(key), kh)
		}
		return err
	}, nil
}

// ---------------------------------------------------------------- auth

// dialAgent is swapped by tests; the platform files provide the default.
var dialAgent = dialSystemAgent

// agentSigners returns the agent's keys; the closer must stay open until the handshake is done.
func agentSigners() ([]ssh.Signer, io.Closer) {
	if !sshUseAgent {
		return nil, nil
	}
	conn, err := dialAgent()
	if err != nil {
		return nil, nil
	}
	signers, err := agent.NewClient(conn).Signers()
	if err != nil {
		conn.Close()
		return nil, nil
	}
	return signers, conn
}

func defaultKeyFiles() []string {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}
	var out []string
	for _, n := range []string{"id_ed25519", "id_ecdsa", "id_rsa"} {
		out = append(out, filepath.Join(home, ".ssh", n))
	}
	return out
}

// authMethods builds one publickey method (explicit keys, then agent keys, then default keys)
// plus password / keyboard-interactive when a password was given. The x/crypto client tries each
// method type only once, so all keys must live in a single method.
func authMethods(t sshTarget, warn func(string)) ([]ssh.AuthMethod, io.Closer) {
	var signers []ssh.Signer
	seen := map[string]bool{}
	addSigner := func(s ssh.Signer) {
		k := string(s.PublicKey().Marshal())
		if !seen[k] {
			seen[k] = true
			signers = append(signers, s)
		}
	}
	var locked []string
	loadKey := func(kf string, explicit bool) {
		b, err := os.ReadFile(kf)
		if err != nil {
			if explicit {
				warn("Cannot read key " + kf + ": " + err.Error())
			}
			return
		}
		s, err := ssh.ParsePrivateKey(b)
		var pm *ssh.PassphraseMissingError
		if errors.As(err, &pm) && t.Passphrase != "" {
			s, err = ssh.ParsePrivateKeyWithPassphrase(b, []byte(t.Passphrase))
			if err != nil {
				if explicit {
					warn("Wrong passphrase for " + kf)
				}
				return
			}
		}
		if err != nil {
			if errors.As(err, &pm) {
				locked = append(locked, kf)
			} else if explicit {
				warn("Cannot use key " + kf + ": " + err.Error())
			}
			return
		}
		addSigner(s)
	}
	for _, kf := range t.KeyFiles {
		loadKey(kf, true)
	}
	agentKeys, closer := agentSigners()
	for _, s := range agentKeys {
		addSigner(s)
	}
	if len(t.KeyFiles) == 0 {
		for _, kf := range defaultKeyFiles() {
			loadKey(kf, false)
		}
	}
	if len(signers) == 0 && len(locked) > 0 && t.Password == "" {
		warn("Key " + strings.Join(locked, ", ") + " is passphrase-protected; supply the passphrase or add it to ssh-agent.")
	}
	var methods []ssh.AuthMethod
	if len(signers) > 0 {
		methods = append(methods, ssh.PublicKeys(signers...))
	}
	if t.Password != "" {
		pw := t.Password
		methods = append(methods, ssh.Password(pw), ssh.KeyboardInteractive(func(_, _ string, qs []string, _ []bool) ([]string, error) {
			ans := make([]string, len(qs))
			for i := range ans {
				ans[i] = pw
			}
			return ans, nil
		}))
	}
	return methods, closer
}

// ---------------------------------------------------------------- dialing (with ProxyJump)

const sshHandshakeTimeout = 20 * time.Second

// dialSSH connects to t, through its jump hosts if any. Closing the returned client also closes
// the hop connections. Jump hosts use keys/agent only; the password is only sent to t itself.
func dialSSH(t sshTarget, warn func(string)) (*ssh.Client, error) {
	cb, err := hostKeyCallback(warn)
	if err != nil {
		return nil, err
	}
	hops := append(append([]sshTarget{}, t.Jump...), t)
	var chain []*ssh.Client
	closeAll := func() {
		for i := len(chain) - 1; i >= 0; i-- {
			chain[i].Close()
		}
	}
	for i, h := range hops {
		label := h.label()
		if i < len(hops)-1 {
			label = "jump host " + label
			h.Password = ""
		}
		addr := net.JoinHostPort(h.Host, strconv.Itoa(h.Port))
		var conn net.Conn
		if len(chain) == 0 {
			conn, err = net.DialTimeout("tcp", addr, 10*time.Second)
		} else {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			conn, err = chain[len(chain)-1].DialContext(ctx, "tcp", addr)
			cancel()
		}
		if err != nil {
			closeAll()
			return nil, fmt.Errorf("%s: %w", label, err)
		}
		methods, agentConn := authMethods(h, warn)
		if len(methods) == 0 {
			conn.Close()
			closeAll()
			return nil, fmt.Errorf("%s: %w: no usable credentials (no key in ssh-agent or ~/.ssh, no password)", label, errSSHAuth)
		}
		timer := time.AfterFunc(sshHandshakeTimeout, func() { conn.Close() })
		c, chans, reqs, err := ssh.NewClientConn(conn, addr, &ssh.ClientConfig{User: h.User, Auth: methods, HostKeyCallback: cb})
		timer.Stop()
		if agentConn != nil {
			agentConn.Close()
		}
		if err != nil {
			conn.Close()
			closeAll()
			if strings.Contains(err.Error(), "unable to authenticate") {
				return nil, fmt.Errorf("%s: %w (%v)", label, errSSHAuth, err)
			}
			return nil, fmt.Errorf("%s: %w", label, err)
		}
		chain = append(chain, ssh.NewClient(c, chans, reqs))
	}
	final := chain[len(chain)-1]
	if len(chain) > 1 {
		hopsOnly := chain[:len(chain)-1]
		go func() {
			_ = final.Wait()
			for i := len(hopsOnly) - 1; i >= 0; i-- {
				hopsOnly[i].Close()
			}
		}()
	}
	return final, nil
}

// ---------------------------------------------------------------- interactive sessions

type sshPTY struct {
	client  *ssh.Client
	session *ssh.Session
	stdin   io.WriteCloser
	stdout  io.Reader
}

func (s *sshPTY) Read(b []byte) (int, error)  { return s.stdout.Read(b) }
func (s *sshPTY) Write(b []byte) (int, error) { return s.stdin.Write(b) }
func (s *sshPTY) Resize(c, r int) error       { return s.session.WindowChange(r, c) }
func (s *sshPTY) Close() error {
	_ = s.session.Close()
	return s.client.Close()
}

func startSSH(t sshTarget, in termInit, say func(string)) (ptyLike, error) {
	client, err := dialSSH(t, say)
	if err != nil {
		return nil, err
	}
	sess, err := client.NewSession()
	if err != nil {
		client.Close()
		return nil, err
	}
	modes := ssh.TerminalModes{ssh.ECHO: 1, ssh.TTY_OP_ISPEED: 14400, ssh.TTY_OP_OSPEED: 14400}
	if err := sess.RequestPty("xterm-256color", in.Rows, in.Cols, modes); err != nil {
		client.Close()
		return nil, err
	}
	stdin, _ := sess.StdinPipe()
	stdout, _ := sess.StdoutPipe()
	sess.Stderr = io.Discard // with a pty, stderr is merged into stdout anyway
	if err := sess.Shell(); err != nil {
		client.Close()
		return nil, err
	}
	if in.Cmd != "" {
		_, _ = stdin.Write([]byte(in.Cmd + "\n"))
	}
	return &sshPTY{client: client, session: sess, stdin: stdin, stdout: stdout}, nil
}

// limitedBuf keeps the first max bytes. It must not embed bytes.Buffer: the promoted ReadFrom
// would let io.Copy bypass both the limit and the lock (stdout and stderr copy concurrently).
type limitedBuf struct {
	mu  sync.Mutex
	buf bytes.Buffer
	max int
}

func (l *limitedBuf) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if room := l.max - l.buf.Len(); room > 0 {
		if len(p) > room {
			l.buf.Write(p[:room])
		} else {
			l.buf.Write(p)
		}
	}
	return len(p), nil
}

func (l *limitedBuf) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.buf.String()
}

// sshExec runs one command on a saved host or ~/.ssh/config alias using key / agent auth
// (used by the MCP server). Jump hosts from ProxyJump are honoured.
func sshExec(profile, command string, timeout time.Duration) (string, error) {
	t, err := resolveSSHProfile(profile)
	if err != nil {
		return "", err
	}
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	var warns []string
	client, err := dialSSH(t, func(s string) { warns = append(warns, s) })
	if err != nil {
		return "", err
	}
	defer client.Close()
	sess, err := client.NewSession()
	if err != nil {
		return "", err
	}
	defer sess.Close()
	out := &limitedBuf{max: 64 * 1024}
	sess.Stdout, sess.Stderr = out, out
	done := make(chan error, 1)
	go func() { done <- sess.Run(command) }()
	prefix := ""
	if len(warns) > 0 {
		prefix = strings.Join(warns, "\n") + "\n"
	}
	select {
	case err := <-done:
		res := prefix + out.String()
		if err != nil {
			return res + "\n[exit: " + err.Error() + "]", nil
		}
		return res, nil
	case <-time.After(timeout):
		return prefix + out.String() + "\n[timed out]", nil
	}
}

// ---------------------------------------------------------------- host list API

type sshHostView struct {
	Name     string   `json:"name"`
	Host     string   `json:"host"`
	Port     int      `json:"port"`
	User     string   `json:"user"`
	KeyPath  string   `json:"keyPath,omitempty"`
	KeyFiles []string `json:"keyFiles,omitempty"`
	Jump     string   `json:"jump,omitempty"`
	Source   string   `json:"source"` // saved | config
	Shadowed bool     `json:"shadowed,omitempty"`
}

func sshHostList() []sshHostView {
	var out []sshHostView
	saved := map[string]bool{}
	extras := loadSSHExtras()
	for _, h := range loadConfig().Hosts {
		saved[h.Name] = true
		out = append(out, sshHostView{Name: h.Name, Host: h.Host, Port: h.Port, User: h.User, KeyPath: h.KeyPath, Jump: extras[h.Name].Jump, Source: "saved"})
	}
	if cfg := loadSSHConfig(); cfg != nil {
		for _, a := range cfg.aliases() {
			h := cfg.lookup(a)
			out = append(out, sshHostView{Name: a, Host: h.HostName, Port: h.Port, User: h.User, KeyFiles: h.IdentityFiles, Jump: h.ProxyJump, Source: "config", Shadowed: saved[a]})
		}
	}
	return out
}

func agentStatus() map[string]any {
	if !sshUseAgent {
		return map[string]any{"ok": false, "error": "disabled"}
	}
	conn, err := dialAgent()
	if err != nil {
		return map[string]any{"ok": false, "error": err.Error()}
	}
	defer conn.Close()
	keys, err := agent.NewClient(conn).List()
	if err != nil {
		return map[string]any{"ok": false, "error": err.Error()}
	}
	names := make([]string, 0, len(keys))
	for _, k := range keys {
		names = append(names, k.Comment)
	}
	return map[string]any{"ok": true, "keys": len(keys), "comments": names}
}

func init() {
	addRoutes(func(mux *http.ServeMux) {
		mux.HandleFunc("GET /api/ssh/hosts", func(w http.ResponseWriter, r *http.Request) {
			res := map[string]any{"hosts": sshHostList()}
			if r.URL.Query().Get("agent") == "1" {
				res["agent"] = agentStatus()
			}
			writeJSON(w, res)
		})
		// Extra settings for a saved host (currently its ProxyJump); empty jump removes the entry.
		mux.HandleFunc("PUT /api/ssh/extra", func(w http.ResponseWriter, r *http.Request) {
			var b struct {
				Name string `json:"name"`
				Jump string `json:"jump"`
			}
			if err := decode(r, &b); err != nil {
				fail(w, 400, err)
				return
			}
			b.Jump = strings.TrimSpace(b.Jump)
			if b.Name == "" || len(b.Name) > 100 {
				fail(w, 400, errors.New("name is required"))
				return
			}
			if !jumpRe.MatchString(b.Jump) || len(b.Jump) > 512 {
				fail(w, 400, errors.New("jump hosts look like user@host:port, separated by commas"))
				return
			}
			m := loadSSHExtras()
			if b.Jump == "" {
				delete(m, b.Name)
			} else {
				m[b.Name] = sshExtra{Jump: b.Jump}
			}
			if err := saveJSON("ssh-hosts", m); err != nil {
				fail(w, 500, err)
				return
			}
			writeJSON(w, map[string]bool{"ok": true})
		})
	})
}
