package main

import (
	"io"
	"net"
	"strconv"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"
)

func freePort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	return ln.Addr().(*net.TCPAddr).Port
}

func echoServer(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go func() { _, _ = io.Copy(c, c); c.Close() }()
		}
	}()
	return ln.Addr().(*net.TCPAddr).Port
}

func roundTrip(t *testing.T, port int, msg string) {
	t.Helper()
	c, err := net.DialTimeout("tcp", "127.0.0.1:"+strconv.Itoa(port), 2*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	_ = c.SetDeadline(time.Now().Add(5 * time.Second))
	if _, err := c.Write([]byte(msg)); err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, len(msg))
	if _, err := io.ReadFull(c, buf); err != nil || string(buf) != msg {
		t.Fatalf("echo = %q, %v", buf, err)
	}
}

func TestTunnelLifecycle(t *testing.T) {
	home := testHome(t)
	tunnels = &tunnelManager{runs: map[string]*tunnelRun{}, lastErr: map[string]string{}}
	user, priv := newSigner(t)
	keyFile := writeKey(t, home, priv)
	d := startTestSSHD(t, testSSHDOpts{keys: []ssh.PublicKey{user.PublicKey()}})
	if err := saveConfig(Config{Hosts: []SSHHost{{Name: "box", Host: "127.0.0.1", Port: d.port, User: "me", KeyPath: keyFile}}}); err != nil {
		t.Fatal(err)
	}
	echo := echoServer(t)
	local := freePort(t)

	if _, err := tunnels.save(tunnelDef{Host: "box", LocalPort: 0, RemotePort: echo}); err == nil {
		t.Fatal("port 0 accepted")
	}
	if _, err := tunnels.save(tunnelDef{Host: "box", LocalPort: local, RemoteHost: "bad host!", RemotePort: echo}); err == nil {
		t.Fatal("bad remote host accepted")
	}
	def, err := tunnels.save(tunnelDef{Name: "echo", Host: "box", LocalPort: local, RemotePort: echo, AutoReconnect: true})
	if err != nil {
		t.Fatal(err)
	}
	if def.RemoteHost != "127.0.0.1" || def.ID == "" {
		t.Fatalf("def = %+v", def)
	}
	if _, err := tunnels.save(tunnelDef{Host: "box", LocalPort: local, RemotePort: 1}); err == nil {
		t.Fatal("duplicate local port accepted")
	}

	// Definitions persist.
	var saved []tunnelDef
	if err := loadJSON("tunnels", &saved); err != nil || len(saved) != 1 || saved[0].ID != def.ID {
		t.Fatalf("saved = %+v, %v", saved, err)
	}

	if err := tunnels.start(def.ID, ""); err != nil {
		t.Fatal(err)
	}
	if err := tunnels.start(def.ID, ""); err == nil {
		t.Fatal("double start accepted")
	}
	roundTrip(t, local, "ping")
	roundTrip(t, local, "pong!")
	waitFor(t, "stats", func() bool {
		st := tunnels.list()[0]
		return st.State == "running" && st.Total == 2 && st.BytesOut == 9 && st.BytesIn == 9 && st.Active == 0
	})

	// Auto-reconnect: drop the SSH connection; the next local connection re-dials.
	d.mu.Lock()
	for _, c := range d.conns {
		c.Close()
	}
	d.mu.Unlock()
	waitFor(t, "idle", func() bool { return tunnels.list()[0].State == "idle" })
	roundTrip(t, local, "again")
	if st := tunnels.list()[0]; st.State != "running" {
		t.Fatalf("after reconnect: %+v", st)
	}

	tunnels.stop(def.ID)
	if st := tunnels.list()[0]; st.State != "stopped" {
		t.Fatalf("after stop: %+v", st)
	}
	if c, err := net.DialTimeout("tcp", "127.0.0.1:"+strconv.Itoa(local), time.Second); err == nil {
		c.Close()
		t.Fatal("listener still open after stop")
	}
	if err := tunnels.remove(def.ID); err != nil || len(tunnels.list()) != 0 {
		t.Fatalf("remove: %v", err)
	}
}

func TestTunnelNoAutoReconnectStops(t *testing.T) {
	home := testHome(t)
	tunnels = &tunnelManager{runs: map[string]*tunnelRun{}, lastErr: map[string]string{}}
	user, priv := newSigner(t)
	keyFile := writeKey(t, home, priv)
	d := startTestSSHD(t, testSSHDOpts{keys: []ssh.PublicKey{user.PublicKey()}})
	if err := saveConfig(Config{Hosts: []SSHHost{{Name: "box", Host: "127.0.0.1", Port: d.port, User: "me", KeyPath: keyFile}}}); err != nil {
		t.Fatal(err)
	}
	def, err := tunnels.save(tunnelDef{Host: "box", LocalPort: freePort(t), RemotePort: echoServer(t)})
	if err != nil {
		t.Fatal(err)
	}
	if err := tunnels.start(def.ID, ""); err != nil {
		t.Fatal(err)
	}
	d.close()
	waitFor(t, "error state", func() bool { st := tunnels.list()[0]; return st.State == "error" && st.Error != "" })
}
