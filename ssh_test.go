package main

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"errors"
	"io"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
)

// ---------------------------------------------------------------- in-process SSH server

type testSSHD struct {
	addr   string
	port   int
	ln     net.Listener
	cfg    *ssh.ServerConfig
	root   string // SFTP working directory
	direct atomic.Int64
	mu     sync.Mutex
	conns  []net.Conn
}

type testSSHDOpts struct {
	addr     string
	password string
	keys     []ssh.PublicKey
	hostKey  ssh.Signer
	root     string
}

func newSigner(t *testing.T) (ssh.Signer, ed25519.PrivateKey) {
	t.Helper()
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	s, err := ssh.NewSignerFromKey(priv)
	if err != nil {
		t.Fatal(err)
	}
	return s, priv
}

func startTestSSHD(t *testing.T, o testSSHDOpts) *testSSHD {
	t.Helper()
	cfg := &ssh.ServerConfig{
		PublicKeyCallback: func(_ ssh.ConnMetadata, k ssh.PublicKey) (*ssh.Permissions, error) {
			for _, a := range o.keys {
				if bytes.Equal(a.Marshal(), k.Marshal()) {
					return nil, nil
				}
			}
			return nil, errors.New("unknown key")
		},
	}
	if o.password != "" {
		cfg.PasswordCallback = func(_ ssh.ConnMetadata, p []byte) (*ssh.Permissions, error) {
			if string(p) == o.password {
				return nil, nil
			}
			return nil, errors.New("bad password")
		}
	}
	if o.hostKey == nil {
		o.hostKey, _ = newSigner(t)
	}
	cfg.AddHostKey(o.hostKey)
	if o.addr == "" {
		o.addr = "127.0.0.1:0"
	}
	ln, err := net.Listen("tcp", o.addr)
	if err != nil {
		t.Fatal(err)
	}
	d := &testSSHD{addr: ln.Addr().String(), port: ln.Addr().(*net.TCPAddr).Port, ln: ln, cfg: cfg, root: o.root}
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			d.mu.Lock()
			d.conns = append(d.conns, c)
			d.mu.Unlock()
			go d.serve(c)
		}
	}()
	t.Cleanup(d.close)
	return d
}

func (d *testSSHD) close() {
	d.ln.Close()
	d.mu.Lock()
	for _, c := range d.conns {
		c.Close()
	}
	d.mu.Unlock()
}

func (d *testSSHD) serve(c net.Conn) {
	sc, chans, reqs, err := ssh.NewServerConn(c, d.cfg)
	if err != nil {
		c.Close()
		return
	}
	defer sc.Close()
	go ssh.DiscardRequests(reqs)
	for nc := range chans {
		switch nc.ChannelType() {
		case "session":
			ch, rq, err := nc.Accept()
			if err == nil {
				go d.session(ch, rq)
			}
		case "direct-tcpip":
			var p struct {
				Host  string
				Port  uint32
				OHost string
				OPort uint32
			}
			if ssh.Unmarshal(nc.ExtraData(), &p) != nil {
				nc.Reject(ssh.ConnectionFailed, "bad payload")
				continue
			}
			d.direct.Add(1)
			tc, err := net.Dial("tcp", net.JoinHostPort(p.Host, strconv.Itoa(int(p.Port))))
			if err != nil {
				nc.Reject(ssh.ConnectionFailed, err.Error())
				continue
			}
			ch, rq, err := nc.Accept()
			if err != nil {
				tc.Close()
				continue
			}
			go ssh.DiscardRequests(rq)
			go func() {
				go func() { _, _ = io.Copy(ch, tc); _ = ch.CloseWrite() }()
				_, _ = io.Copy(tc, ch)
				_ = tc.(*net.TCPConn).CloseWrite()
			}()
		default:
			nc.Reject(ssh.UnknownChannelType, "unsupported")
		}
	}
}

func (d *testSSHD) session(ch ssh.Channel, rq <-chan *ssh.Request) {
	for req := range rq {
		switch req.Type {
		case "exec":
			var p struct{ Cmd string }
			_ = ssh.Unmarshal(req.Payload, &p)
			_ = req.Reply(true, nil)
			out := "ran: " + p.Cmd + "\n"
			if strings.HasPrefix(p.Cmd, "echo ") {
				out = p.Cmd[5:] + "\n"
			}
			_, _ = ch.Write([]byte(out))
			_, _ = ch.SendRequest("exit-status", false, ssh.Marshal(struct{ S uint32 }{0}))
			ch.Close()
			return
		case "subsystem":
			var p struct{ Name string }
			_ = ssh.Unmarshal(req.Payload, &p)
			if p.Name != "sftp" || d.root == "" {
				_ = req.Reply(false, nil)
				continue
			}
			_ = req.Reply(true, nil)
			srv, err := sftp.NewServer(ch, sftp.WithServerWorkingDirectory(d.root))
			if err != nil {
				ch.Close()
				return
			}
			go func() { _ = srv.Serve(); srv.Close(); ch.Close() }()
		case "pty-req", "env":
			_ = req.Reply(true, nil)
		case "window-change":
		case "shell":
			_ = req.Reply(true, nil)
			go func() {
				_, _ = ch.Write([]byte("welcome\r\n"))
				_, _ = io.Copy(ch, ch) // echo
				ch.Close()
			}()
		default:
			_ = req.Reply(false, nil)
		}
	}
}

// ---------------------------------------------------------------- environment helpers

// testHome points HOME/USERPROFILE (so ~/.devdeck and ~/.ssh) at a temp dir and disables the
// real ssh-agent.
func testHome(t *testing.T) string {
	t.Helper()
	d := t.TempDir()
	t.Setenv("USERPROFILE", d)
	t.Setenv("HOME", d)
	t.Setenv("SSH_AUTH_SOCK", "")
	oldAgent, oldDial := sshUseAgent, dialAgent
	sshUseAgent = false
	t.Cleanup(func() { sshUseAgent, dialAgent = oldAgent, oldDial })
	if err := os.MkdirAll(filepath.Join(d, ".ssh"), 0o700); err != nil {
		t.Fatal(err)
	}
	return d
}

func writeKey(t *testing.T, home string, priv ed25519.PrivateKey) string {
	t.Helper()
	blk, err := ssh.MarshalPrivateKey(priv, "")
	if err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(home, "test_key")
	if err := os.WriteFile(p, pem.EncodeToMemory(blk), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func writeSSHConfig(t *testing.T, home, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(home, ".ssh", "config"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

// ---------------------------------------------------------------- tests

func TestSSHExecSavedHostAndTOFU(t *testing.T) {
	home := testHome(t)
	user, priv := newSigner(t)
	keyFile := writeKey(t, home, priv)
	hostKey, _ := newSigner(t)
	d := startTestSSHD(t, testSSHDOpts{keys: []ssh.PublicKey{user.PublicKey()}, hostKey: hostKey})
	if err := saveConfig(Config{Hosts: []SSHHost{{Name: "box", Host: "127.0.0.1", Port: d.port, User: "me", KeyPath: keyFile}}}); err != nil {
		t.Fatal(err)
	}
	out, err := sshExec("box", "echo hello", 5*time.Second)
	if err != nil || !strings.Contains(out, "hello") {
		t.Fatalf("sshExec = %q, %v", out, err)
	}
	if !strings.Contains(out, "permanently added") {
		t.Errorf("first connection should report TOFU: %q", out)
	}
	kh, _ := os.ReadFile(filepath.Join(home, ".ssh", "known_hosts"))
	if !strings.Contains(string(kh), "[127.0.0.1]:"+strconv.Itoa(d.port)) {
		t.Fatalf("known_hosts not updated: %q", kh)
	}
	out, err = sshExec("box", "echo again", 5*time.Second)
	if err != nil || strings.Contains(out, "permanently added") || !strings.Contains(out, "again") {
		t.Fatalf("second exec = %q, %v", out, err)
	}

	// Same address, different host key: must be refused.
	d.close()
	other, _ := newSigner(t)
	startTestSSHD(t, testSSHDOpts{addr: d.addr, keys: []ssh.PublicKey{user.PublicKey()}, hostKey: other})
	if _, err := sshExec("box", "echo x", 5*time.Second); err == nil || !strings.Contains(err.Error(), "HOST KEY CHANGED") {
		t.Fatalf("changed host key not rejected: %v", err)
	}
}

func TestSSHPasswordAndAuthError(t *testing.T) {
	testHome(t)
	d := startTestSSHD(t, testSSHDOpts{password: "s3cret"})
	tg := sshTarget{Host: "127.0.0.1", Port: d.port, User: "me"}
	if _, err := dialSSH(tg, func(string) {}); !errors.Is(err, errSSHAuth) {
		t.Fatalf("want errSSHAuth without credentials, got %v", err)
	}
	tg.Password = "wrong"
	if _, err := dialSSH(tg, func(string) {}); !errors.Is(err, errSSHAuth) {
		t.Fatalf("want errSSHAuth with wrong password, got %v", err)
	}
	tg.Password = "s3cret"
	c, err := dialSSH(tg, func(string) {})
	if err != nil {
		t.Fatal(err)
	}
	c.Close()
}

func TestSSHAgentAndConfigAlias(t *testing.T) {
	home := testHome(t)
	user, priv := newSigner(t)
	d := startTestSSHD(t, testSSHDOpts{keys: []ssh.PublicKey{user.PublicKey()}})
	kr := agent.NewKeyring()
	if err := kr.Add(agent.AddedKey{PrivateKey: priv, Comment: "test-key"}); err != nil {
		t.Fatal(err)
	}
	sshUseAgent = true
	dialAgent = func() (io.ReadWriteCloser, error) {
		a, b := net.Pipe()
		go func() { _ = agent.ServeAgent(kr, b) }()
		return a, nil
	}
	writeSSHConfig(t, home, "Host agenthost\n  HostName 127.0.0.1\n  Port "+strconv.Itoa(d.port)+"\n  User me\n")
	out, err := sshExec("agenthost", "echo via-agent", 5*time.Second)
	if err != nil || !strings.Contains(out, "via-agent") {
		t.Fatalf("agent auth: %q, %v", out, err)
	}
	st := agentStatus()
	if st["ok"] != true || st["keys"] != 1 {
		t.Fatalf("agentStatus = %v", st)
	}
	hosts := sshHostList()
	if len(hosts) != 1 || hosts[0].Source != "config" || hosts[0].Port != d.port {
		t.Fatalf("host list = %+v", hosts)
	}
}

func TestSSHProxyJump(t *testing.T) {
	home := testHome(t)
	user, priv := newSigner(t)
	keyFile := writeKey(t, home, priv)
	jump := startTestSSHD(t, testSSHDOpts{keys: []ssh.PublicKey{user.PublicKey()}})
	target := startTestSSHD(t, testSSHDOpts{keys: []ssh.PublicKey{user.PublicKey()}})
	writeSSHConfig(t, home, strings.Join([]string{
		"Host *",
		"  IdentityFile " + keyFile,
		"  User me",
		"Host bastion",
		"  HostName 127.0.0.1",
		"  Port " + strconv.Itoa(jump.port),
		"Host inner",
		"  HostName 127.0.0.1",
		"  Port " + strconv.Itoa(target.port),
		"  ProxyJump bastion",
		"",
	}, "\n"))
	out, err := sshExec("inner", "echo through-jump", 5*time.Second)
	if err != nil || !strings.Contains(out, "through-jump") {
		t.Fatalf("jump exec: %q, %v", out, err)
	}
	if jump.direct.Load() != 1 || target.direct.Load() != 0 {
		t.Fatalf("expected one forwarded connection through the bastion, got %d/%d", jump.direct.Load(), target.direct.Load())
	}

	// A saved host can use a jump too (stored next to config.json).
	if err := saveConfig(Config{Hosts: []SSHHost{{Name: "saved-inner", Host: "127.0.0.1", Port: target.port, User: "me", KeyPath: keyFile}}}); err != nil {
		t.Fatal(err)
	}
	if err := saveJSON("ssh-hosts", map[string]sshExtra{"saved-inner": {Jump: "me@127.0.0.1:" + strconv.Itoa(jump.port)}}); err != nil {
		t.Fatal(err)
	}
	out, err = sshExec("saved-inner", "echo saved-jump", 5*time.Second)
	if err != nil || !strings.Contains(out, "saved-jump") || jump.direct.Load() != 2 {
		t.Fatalf("saved jump exec: %q, %v (direct=%d)", out, err, jump.direct.Load())
	}
}

func TestSSHTerminalSession(t *testing.T) {
	home := testHome(t)
	user, priv := newSigner(t)
	keyFile := writeKey(t, home, priv)
	d := startTestSSHD(t, testSSHDOpts{keys: []ssh.PublicKey{user.PublicKey()}})
	tg := sshTarget{Host: "127.0.0.1", Port: d.port, User: "me", KeyFiles: []string{keyFile}}
	p, err := startSSH(tg, termInit{Cols: 80, Rows: 24}, func(string) {})
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	if _, err := p.Write([]byte("ping")); err != nil {
		t.Fatal(err)
	}
	var got []byte
	buf := make([]byte, 256)
	deadline := time.Now().Add(5 * time.Second)
	for !bytes.Contains(got, []byte("ping")) && time.Now().Before(deadline) {
		n, err := p.Read(buf)
		got = append(got, buf[:n]...)
		if err != nil {
			break
		}
	}
	if !bytes.Contains(got, []byte("welcome")) || !bytes.Contains(got, []byte("ping")) {
		t.Fatalf("terminal output = %q", got)
	}
	if err := p.Resize(100, 40); err != nil {
		t.Fatal(err)
	}
}

func TestParseJumpHop(t *testing.T) {
	cases := []struct {
		in, user, host string
		port           int
		bad            bool
	}{
		{"host", "", "host", 0, false},
		{"u@host:2222", "u", "host", 2222, false},
		{"[::1]:22", "", "::1", 22, false},
		{"u@[fe80::1]", "", "", 0, true},
		{"host:99999", "", "", 0, true},
		{"u@", "", "", 0, true},
	}
	for _, c := range cases {
		u, h, p, err := parseJumpHop(c.in)
		if c.bad {
			if err == nil {
				t.Errorf("%q: want error", c.in)
			}
			continue
		}
		if err != nil || u != c.user || h != c.host || p != c.port {
			t.Errorf("%q = %q %q %d %v", c.in, u, h, p, err)
		}
	}
}

func TestSSHConfigParse(t *testing.T) {
	home := testHome(t)
	inc := filepath.Join(home, ".ssh", "work.conf")
	if err := os.WriteFile(inc, []byte("Host work\n  HostName work.example.com\n  User worker\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg := parseSSHConfig(strings.NewReader(strings.Join([]string{
		"# comment",
		"Include work.conf",
		"Host web web2",
		"  HostName=10.0.0.5",
		"  Port 2200",
		"  IdentityFile ~/.ssh/web_key",
		"Host *.corp !skip.corp",
		"  User corpuser",
		"  ProxyJump bastion.corp",
		"Match host foo",
		"  User matched",
		"Host \"quoted name\"",
		"  HostName q.example.com",
		"Host *",
		"  User fallback",
		"  Port 2222",
		"  IdentityFile %d/.ssh/id_%r",
		"",
	}, "\n")))

	got := cfg.aliases()
	want := []string{"work", "web", "web2", "quoted name"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("aliases = %v, want %v", got, want)
	}
	web := cfg.lookup("web2")
	if web.HostName != "10.0.0.5" || web.Port != 2200 || web.User != "fallback" {
		t.Fatalf("web2 = %+v", web)
	}
	if len(web.IdentityFiles) != 2 || web.IdentityFiles[0] != filepath.Join(home, ".ssh", "web_key") || web.IdentityFiles[1] != home+"/.ssh/id_fallback" {
		t.Fatalf("web2 identity files = %v", web.IdentityFiles)
	}
	corp := cfg.lookup("db.corp")
	if corp.User != "corpuser" || corp.ProxyJump != "bastion.corp" || corp.HostName != "db.corp" || corp.Port != 2222 {
		t.Fatalf("db.corp = %+v", corp)
	}
	if skip := cfg.lookup("skip.corp"); skip.User != "fallback" || skip.ProxyJump != "" {
		t.Fatalf("negated pattern ignored: %+v", skip)
	}
	if w := cfg.lookup("work"); w.HostName != "work.example.com" || w.User != "worker" {
		t.Fatalf("included host = %+v", w)
	}
	if q := cfg.lookup("quoted name"); q.HostName != "q.example.com" {
		t.Fatalf("quoted = %+v", q)
	}
	if !wildMatch("a*c?e", "abbbcde") || wildMatch("a*c", "abd") || !wildMatch("*", "") {
		t.Fatal("wildMatch")
	}
}
