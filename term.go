package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/coder/websocket"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

type termInit struct {
	Kind       string `json:"kind"` // "local" | "ssh"
	Cols       int    `json:"cols"`
	Rows       int    `json:"rows"`
	Cwd        string `json:"cwd"`
	Cmd        string `json:"cmd"`     // optional command to run (e.g. "claude")
	Profile    string `json:"profile"` // saved SSH host name
	Host       string `json:"host"`
	Port       int    `json:"port"`
	User       string `json:"user"`
	KeyPath    string `json:"keyPath"`
	Password   string `json:"password"`
	Passphrase string `json:"passphrase"`
}

type ptyLike interface {
	io.ReadWriteCloser
	Resize(cols, rows int) error
}

func handleTermWS(w http.ResponseWriter, r *http.Request) {
	ws, err := websocket.Accept(w, r, nil) // enforces same-origin
	if err != nil {
		return
	}
	defer ws.CloseNow()
	ws.SetReadLimit(1 << 20)
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	_, data, err := ws.Read(ctx)
	if err != nil {
		return
	}
	var in termInit
	if err := json.Unmarshal(data, &in); err != nil {
		ws.Close(websocket.StatusPolicyViolation, "bad init message")
		return
	}
	if in.Cols <= 0 {
		in.Cols = 120
	}
	if in.Rows <= 0 {
		in.Rows = 30
	}
	say := func(s string) { _ = ws.Write(ctx, websocket.MessageBinary, []byte("\r\n"+s+"\r\n")) }

	var p ptyLike
	switch in.Kind {
	case "local":
		p, err = startLocal(in)
	case "ssh":
		p, err = startSSH(in, say)
	default:
		err = errors.New("unknown terminal kind")
	}
	if err != nil {
		say("\x1b[31m" + err.Error() + "\x1b[0m")
		ws.Close(websocket.StatusNormalClosure, "failed")
		return
	}
	defer p.Close()

	var once sync.Once
	stop := func() { once.Do(cancel) }
	go func() {
		defer stop()
		buf := make([]byte, 16*1024)
		for {
			n, err := p.Read(buf)
			if n > 0 {
				if werr := ws.Write(ctx, websocket.MessageBinary, buf[:n]); werr != nil {
					return
				}
			}
			if err != nil {
				return
			}
		}
	}()
	go func() {
		defer stop()
		for {
			typ, msg, err := ws.Read(ctx)
			if err != nil {
				return
			}
			if typ == websocket.MessageBinary {
				if _, err := p.Write(msg); err != nil {
					return
				}
				continue
			}
			var c struct {
				Resize *struct{ Cols, Rows int } `json:"resize"`
			}
			if json.Unmarshal(msg, &c) == nil && c.Resize != nil && c.Resize.Cols > 0 && c.Resize.Rows > 0 {
				_ = p.Resize(c.Resize.Cols, c.Resize.Rows)
			}
		}
	}()
	<-ctx.Done()
	ws.Close(websocket.StatusNormalClosure, "bye")
}

// ---------------------------------------------------------------- SSH

type sshTarget struct {
	Host, User, KeyPath, Password, Passphrase string
	Port                                      int
}

func targetFromInit(in termInit) (sshTarget, error) {
	t := sshTarget{Host: in.Host, Port: in.Port, User: in.User, KeyPath: in.KeyPath, Password: in.Password, Passphrase: in.Passphrase}
	if in.Profile != "" {
		found := false
		for _, h := range loadConfig().Hosts {
			if h.Name == in.Profile {
				t.Host, t.Port, t.User, t.KeyPath = h.Host, h.Port, h.User, h.KeyPath
				found = true
			}
		}
		if !found {
			return t, errors.New("unknown saved host: " + in.Profile)
		}
	}
	if t.Host == "" {
		return t, errors.New("host is required")
	}
	if t.Port == 0 {
		t.Port = 22
	}
	if t.User == "" {
		t.User = os.Getenv("USERNAME")
		if t.User == "" {
			t.User = os.Getenv("USER")
		}
	}
	return t, nil
}

func hostKeyCallback(warn func(string)) (ssh.HostKeyCallback, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	dir := filepath.Join(home, ".ssh")
	_ = os.MkdirAll(dir, 0o700)
	kh := filepath.Join(dir, "known_hosts")
	if _, err := os.Stat(kh); err != nil {
		if f, err := os.OpenFile(kh, os.O_CREATE|os.O_WRONLY, 0o600); err == nil {
			f.Close()
		}
	}
	base, err := knownhosts.New(kh)
	if err != nil {
		return nil, err
	}
	return func(hostname string, remote net.Addr, key ssh.PublicKey) error {
		err := base(hostname, remote, key)
		var ke *knownhosts.KeyError
		if errors.As(err, &ke) && len(ke.Want) == 0 {
			// Trust on first use (like StrictHostKeyChecking=accept-new); mismatches are rejected.
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
		return err
	}, nil
}

func authMethods(t sshTarget, warn func(string)) []ssh.AuthMethod {
	var methods []ssh.AuthMethod
	var keyFiles []string
	if t.KeyPath != "" {
		keyFiles = []string{t.KeyPath}
	} else if home, err := os.UserHomeDir(); err == nil {
		for _, n := range []string{"id_ed25519", "id_ecdsa", "id_rsa"} {
			keyFiles = append(keyFiles, filepath.Join(home, ".ssh", n))
		}
	}
	var signers []ssh.Signer
	for _, kf := range keyFiles {
		b, err := os.ReadFile(kf)
		if err != nil {
			continue
		}
		var s ssh.Signer
		if t.Passphrase != "" {
			s, err = ssh.ParsePrivateKeyWithPassphrase(b, []byte(t.Passphrase))
		} else {
			s, err = ssh.ParsePrivateKey(b)
		}
		if err != nil {
			var pm *ssh.PassphraseMissingError
			if errors.As(err, &pm) {
				warn("Key " + kf + " is passphrase-protected; supply the passphrase to use it.")
			}
			continue
		}
		signers = append(signers, s)
	}
	if len(signers) > 0 {
		methods = append(methods, ssh.PublicKeys(signers...))
	}
	if t.Password != "" {
		methods = append(methods, ssh.Password(t.Password))
	}
	return methods
}

func dialSSH(t sshTarget, warn func(string)) (*ssh.Client, error) {
	cb, err := hostKeyCallback(warn)
	if err != nil {
		return nil, err
	}
	methods := authMethods(t, warn)
	if len(methods) == 0 {
		return nil, errors.New("no usable credentials: no readable private key and no password given")
	}
	return ssh.Dial("tcp", net.JoinHostPort(t.Host, strconv.Itoa(t.Port)), &ssh.ClientConfig{
		User: t.User, Auth: methods, HostKeyCallback: cb, Timeout: 10 * time.Second,
	})
}

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

func startSSH(in termInit, say func(string)) (ptyLike, error) {
	t, err := targetFromInit(in)
	if err != nil {
		return nil, err
	}
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
	if err := sess.Shell(); err != nil {
		client.Close()
		return nil, err
	}
	if in.Cmd != "" {
		_, _ = stdin.Write([]byte(in.Cmd + "\n"))
	}
	return &sshPTY{client: client, session: sess, stdin: stdin, stdout: stdout}, nil
}

type limitedBuf struct {
	bytes.Buffer
	max int
}

func (l *limitedBuf) Write(p []byte) (int, error) {
	if room := l.max - l.Len(); room > 0 {
		if len(p) > room {
			l.Buffer.Write(p[:room])
		} else {
			l.Buffer.Write(p)
		}
	}
	return len(p), nil
}

// sshExec runs one command on a saved host using key auth (used by the MCP server).
func sshExec(profile, command string, timeout time.Duration) (string, error) {
	var t sshTarget
	found := false
	for _, h := range loadConfig().Hosts {
		if h.Name == profile {
			t = sshTarget{Host: h.Host, Port: h.Port, User: h.User, KeyPath: h.KeyPath}
			found = true
		}
	}
	if !found {
		return "", errors.New("host not in saved hosts (~/.devdeck/config.json): " + profile)
	}
	if t.Port == 0 {
		t.Port = 22
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
	select {
	case err := <-done:
		res := strings.Join(warns, "\n") + out.String()
		if err != nil {
			return res + "\n[exit: " + err.Error() + "]", nil
		}
		return res, nil
	case <-time.After(timeout):
		return out.String() + "\n[timed out]", nil
	}
}
