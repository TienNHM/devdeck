package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"
)

func TestRingBuf(t *testing.T) {
	r := ringBuf{max: 8}
	r.Write([]byte("abc"))
	if string(r.Bytes()) != "abc" {
		t.Fatalf("got %q", r.Bytes())
	}
	r.Write([]byte("defgh"))
	if string(r.Bytes()) != "abcdefgh" {
		t.Fatalf("got %q", r.Bytes())
	}
	r.Write([]byte("ij"))
	if string(r.Bytes()) != "cdefghij" {
		t.Fatalf("got %q", r.Bytes())
	}
	r.Write([]byte("klmnopq"))
	if string(r.Bytes()) != "jklmnopq" {
		t.Fatalf("got %q", r.Bytes())
	}
	r.Write([]byte("0123456789"))
	if string(r.Bytes()) != "23456789" {
		t.Fatalf("got %q", r.Bytes())
	}
	r.Write([]byte("x"))
	if string(r.Bytes()) != "3456789x" || len(r.buf) != 8 {
		t.Fatalf("got %q (len %d)", r.Bytes(), len(r.buf))
	}
}

// fakePTY: the test writes "process output" into out; input written by viewers lands in in.
type fakePTY struct {
	outR *io.PipeReader
	outW *io.PipeWriter
	mu   sync.Mutex
	in   bytes.Buffer
	size [2]int
	once sync.Once
	dead chan struct{}
}

func newFakePTY() *fakePTY {
	r, w := io.Pipe()
	return &fakePTY{outR: r, outW: w, dead: make(chan struct{})}
}

func (f *fakePTY) Read(b []byte) (int, error) { return f.outR.Read(b) }
func (f *fakePTY) Write(b []byte) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.in.Write(b)
}
func (f *fakePTY) Resize(c, r int) error {
	f.mu.Lock()
	f.size = [2]int{c, r}
	f.mu.Unlock()
	return nil
}
func (f *fakePTY) Close() error {
	f.once.Do(func() { f.outW.Close(); f.outR.Close(); close(f.dead) })
	return nil
}
func (f *fakePTY) input() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.in.String()
}

func termWaitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestTermSessionReplayDetachIdle(t *testing.T) {
	old := termIdleTimeout
	termIdleTimeout = 150 * time.Millisecond
	defer func() { termIdleTimeout = old }()

	p := newFakePTY()
	s, err := termReg.start("t", "local", "/tmp", p, []byte("hello "))
	if err != nil {
		t.Fatal(err)
	}
	go p.outW.Write([]byte("world"))
	termWaitFor(t, "output in ring", func() bool { s.mu.Lock(); defer s.mu.Unlock(); return bytes.Contains(s.ring.Bytes(), []byte("world")) })

	c, replay, err := s.attach()
	if err != nil || string(replay) != "hello world" {
		t.Fatalf("attach = %q, %v", replay, err)
	}
	go p.outW.Write([]byte("!"))
	select {
	case b := <-c.ch:
		if string(b) != "!" {
			t.Fatalf("live chunk = %q", b)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no live output")
	}
	if got := termReg.list(); len(got) == 0 || got[len(got)-1].Clients != 1 {
		t.Fatalf("list = %+v", got)
	}

	// Attached sessions are never reaped, detached ones are after the idle timeout.
	time.Sleep(250 * time.Millisecond)
	if termReg.get(s.ID) == nil {
		t.Fatal("attached session was killed")
	}
	s.detach(c)
	select {
	case <-p.dead:
	case <-time.After(2 * time.Second):
		t.Fatal("detached session not killed after idle timeout")
	}
	termWaitFor(t, "removal", func() bool { return termReg.get(s.ID) == nil })
}

func TestTermSessionExit(t *testing.T) {
	p := newFakePTY()
	s, err := termReg.start("t", "local", "", p, nil)
	if err != nil {
		t.Fatal(err)
	}
	c, _, err := s.attach()
	if err != nil {
		t.Fatal(err)
	}
	p.outW.Close() // the process exits
	for range c.ch {
	}
	termWaitFor(t, "removal", func() bool { return termReg.get(s.ID) == nil })
	if _, _, err := s.attach(); err == nil {
		t.Fatal("attach to an exited session should fail")
	}
}

func TestTermWebSocketAttach(t *testing.T) {
	p := newFakePTY()
	s, err := termReg.start("ws", "local", "", p, []byte("before\r\n"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.kill()
	srv := httptest.NewServer(http.HandlerFunc(handleTermWS))
	defer srv.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http")

	attach := func() *websocket.Conn {
		ws, _, err := websocket.Dial(ctx, wsURL, nil)
		if err != nil {
			t.Fatal(err)
		}
		b, _ := json.Marshal(map[string]any{"attach": s.ID, "cols": 90, "rows": 33})
		if err := ws.Write(ctx, websocket.MessageText, b); err != nil {
			t.Fatal(err)
		}
		typ, msg, err := ws.Read(ctx)
		if err != nil || typ != websocket.MessageText || !strings.Contains(string(msg), s.ID) {
			t.Fatalf("control = %v %q %v", typ, msg, err)
		}
		typ, msg, err = ws.Read(ctx)
		if err != nil || typ != websocket.MessageBinary || !strings.HasPrefix(string(msg), "before") {
			t.Fatalf("replay = %v %q %v", typ, msg, err)
		}
		return ws
	}

	ws := attach()
	if err := ws.Write(ctx, websocket.MessageBinary, []byte("ls\r")); err != nil {
		t.Fatal(err)
	}
	termWaitFor(t, "input", func() bool { return p.input() == "ls\r" })
	p.mu.Lock()
	size := p.size
	p.mu.Unlock()
	if size != [2]int{90, 33} {
		t.Fatalf("resize on attach = %v", size)
	}
	go p.outW.Write([]byte("file.txt"))
	if _, msg, err := ws.Read(ctx); err != nil || string(msg) != "file.txt" {
		t.Fatalf("live = %q %v", msg, err)
	}

	// Closing the socket only detaches: a second socket gets the full replay.
	ws.Close(websocket.StatusNormalClosure, "")
	termWaitFor(t, "detach", func() bool { s.mu.Lock(); defer s.mu.Unlock(); return len(s.clients) == 0 })
	ws = attach()
	// An explicit close kills the session.
	_ = ws.Write(ctx, websocket.MessageText, []byte(`{"close":true}`))
	select {
	case <-p.dead:
	case <-time.After(2 * time.Second):
		t.Fatal("close did not kill the session")
	}
	ws.CloseNow()

	// Attaching to an unknown session reports it is gone.
	ws2, _, err := websocket.Dial(ctx, wsURL, nil)
	if err != nil {
		t.Fatal(err)
	}
	_ = ws2.Write(ctx, websocket.MessageText, []byte(`{"attach":"nope"}`))
	if _, msg, err := ws2.Read(ctx); err != nil || !strings.Contains(string(msg), `"gone":true`) {
		t.Fatalf("unknown attach = %q %v", msg, err)
	}
	ws2.CloseNow()
}
