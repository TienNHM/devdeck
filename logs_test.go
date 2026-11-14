package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"
)

func scratchHome(t *testing.T) string {
	t.Helper()
	h := t.TempDir()
	t.Setenv("HOME", h)
	t.Setenv("USERPROFILE", h)
	return h
}

func appendFile(t *testing.T, p, s string) {
	t.Helper()
	f, err := os.OpenFile(p, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString(s); err != nil {
		t.Fatal(err)
	}
	f.Close()
}

func TestLogsStore(t *testing.T) {
	scratchHome(t)
	dir := t.TempDir()
	p := filepath.Join(dir, "app.log")
	appendFile(t, p, "a\n")

	if _, err := logsAdd(dir); err == nil {
		t.Error("directory accepted")
	}
	if _, err := logsAdd(filepath.Join(dir, "nope.log")); err == nil {
		t.Error("missing file accepted")
	}
	if _, err := logsAdd(""); err == nil {
		t.Error("empty path accepted")
	}
	files, err := logsAdd(`"` + p + `"`)
	if err != nil || len(files) != 1 || files[0] != p {
		t.Fatalf("add %v %v", err, files)
	}
	if files, _ = logsAdd(p); len(files) != 1 {
		t.Errorf("duplicate added: %v", files)
	}
	if got, err := logsAllowed(p); err != nil || got != p {
		t.Errorf("allowed %v %q", err, got)
	}
	if _, err := logsAllowed(filepath.Join(dir, "other.log")); err == nil {
		t.Error("unsaved file allowed")
	}
	info := logsInfo()
	if len(info) != 1 || info[0].Size != 2 || info[0].Name != "app.log" {
		t.Errorf("info %+v", info)
	}
	os.Remove(p)
	if info = logsInfo(); info[0].Error != "missing" {
		t.Errorf("missing info %+v", info)
	}
	if files, _ = logsRemove(p); len(files) != 0 {
		t.Errorf("remove %v", files)
	}
	if logsList() == nil || len(logsList()) != 0 {
		t.Error("list after remove")
	}
}

func tailOf(t *testing.T, content string, n int, complete bool) ([]string, int64) {
	t.Helper()
	p := filepath.Join(t.TempDir(), "x.log")
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	f, _ := os.Open(p)
	defer f.Close()
	ls, next, err := tailFile(f, int64(len(content)), n, complete)
	if err != nil {
		t.Fatal(err)
	}
	return ls, next
}

func TestTailFile(t *testing.T) {
	eq := func(a, b []string) bool { return strings.Join(a, "|") == strings.Join(b, "|") && len(a) == len(b) }
	if ls, _ := tailOf(t, "", 5, false); len(ls) != 0 {
		t.Errorf("empty: %q", ls)
	}
	if ls, _ := tailOf(t, "a\nb\nc\n", 2, false); !eq(ls, []string{"b", "c"}) {
		t.Errorf("last 2: %q", ls)
	}
	if ls, _ := tailOf(t, "a\nb\nc", 2, false); !eq(ls, []string{"b", "c"}) {
		t.Errorf("no trailing newline: %q", ls)
	}
	ls, next := tailOf(t, "a\nb\r\nc-partial", 5, true)
	if !eq(ls, []string{"a", "b"}) || next != 5 {
		t.Errorf("complete mode: %q next=%d", ls, next)
	}
	if ls, next := tailOf(t, "partial", 5, true); len(ls) != 0 || next != 0 {
		t.Errorf("only partial: %q %d", ls, next)
	}
	// many lines across several 64 KB chunks
	var sb strings.Builder
	for i := 0; i < 30000; i++ {
		sb.WriteString("line ")
		sb.WriteString(strings.Repeat("x", i%7))
		sb.WriteString("\n")
	}
	sb.WriteString("LAST\n")
	ls, _ = tailOf(t, sb.String(), 1000, false)
	if len(ls) != 1000 || ls[999] != "LAST" || !strings.HasPrefix(ls[0], "line ") {
		t.Errorf("big tail: %d %q", len(ls), ls[len(ls)-1])
	}
}

func TestTailPathGrepAndMCP(t *testing.T) {
	p := filepath.Join(t.TempDir(), "app.log")
	appendFile(t, p, "INFO start\nERROR one\nDEBUG x\nERROR two\nINFO end\n")
	ls, err := tailPath(p, 10, regexp.MustCompile(`ERROR`))
	if err != nil || strings.Join(ls, "|") != "ERROR one|ERROR two" {
		t.Errorf("grep %v %q", err, ls)
	}
	ls, _ = tailPath(p, 1, regexp.MustCompile(`ERROR`))
	if strings.Join(ls, "|") != "ERROR two" {
		t.Errorf("grep last 1: %q", ls)
	}
	var tf tool
	for _, x := range mcpTools() {
		if x.Name == "tail_file" {
			tf = x
		}
	}
	if tf.handler == nil {
		t.Fatal("tail_file not registered")
	}
	out, err := tf.handler(map[string]any{"path": p, "lines": float64(2)})
	if err != nil || out != "ERROR two\nINFO end\n" {
		t.Errorf("tail_file %v %q", err, out)
	}
	out, _ = tf.handler(map[string]any{"path": p, "grep": "ERROR ("}) // invalid regex: plain text
	if out != "(no lines)\n" {
		t.Errorf("invalid regex fallback %q", out)
	}
	if _, err := tf.handler(map[string]any{"path": filepath.Dir(p)}); err == nil {
		t.Error("directory accepted")
	}
}

type followRec struct {
	lines chan string
	infos chan string
}

func startFollow(t *testing.T, p string) (*followRec, context.CancelFunc) {
	t.Helper()
	st, err := statID(p)
	if err != nil {
		t.Fatal(err)
	}
	rec := &followRec{lines: make(chan string, 1000), infos: make(chan string, 100)}
	ctx, cancel := context.WithCancel(context.Background())
	go followFile(ctx, p, st, st.Size(), 20*time.Millisecond, followEvents{
		lines: func(ls []string) error {
			for _, l := range ls {
				rec.lines <- l
			}
			return nil
		},
		info: func(m string) error { rec.infos <- m; return nil },
	})
	return rec, cancel
}

func expect(t *testing.T, ch chan string, want string) {
	t.Helper()
	select {
	case got := <-ch:
		if !strings.Contains(got, want) {
			t.Fatalf("got %q, want %q", got, want)
		}
	case <-time.After(5 * time.Second):
		t.Fatalf("timeout waiting for %q", want)
	}
}

func TestFollowFile(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "app.log")
	appendFile(t, p, "old line\n")
	rec, cancel := startFollow(t, p)
	defer cancel()

	appendFile(t, p, "new 1\nnew 2\n")
	expect(t, rec.lines, "new 1")
	expect(t, rec.lines, "new 2")

	appendFile(t, p, "half")
	time.Sleep(5 * time.Millisecond)
	appendFile(t, p, " done\n")
	expect(t, rec.lines, "half done")

	// truncation
	if err := os.WriteFile(p, []byte("t1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	expect(t, rec.infos, "truncated")
	expect(t, rec.lines, "t1")

	// rotation: rename away, create a new file at the same path
	time.Sleep(50 * time.Millisecond)
	if err := os.Rename(p, p+".1"); err != nil {
		t.Fatal(err)
	}
	appendFile(t, p, "rotated 1\nrotated 2 with more text so the size grows\n")
	select {
	case m := <-rec.infos:
		if !strings.Contains(m, "rotated") && !strings.Contains(m, "missing") {
			t.Fatalf("info %q", m)
		}
		if strings.Contains(m, "missing") {
			expect(t, rec.infos, "back")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("no rotation notice")
	}
	expect(t, rec.lines, "rotated 1")
	expect(t, rec.lines, "rotated 2")

	// deletion and re-creation
	time.Sleep(50 * time.Millisecond)
	if err := os.Remove(p); err != nil {
		t.Fatal(err)
	}
	expect(t, rec.infos, "missing")
	appendFile(t, p, "again\n")
	expect(t, rec.infos, "back")
	expect(t, rec.lines, "again")

	// an unfinished last line is shown after a short wait
	appendFile(t, p, "no newline")
	expect(t, rec.lines, "no newline")
}

func TestLogsFollowHTTP(t *testing.T) {
	scratchHome(t)
	p := filepath.Join(t.TempDir(), "srv.log")
	var sb strings.Builder
	for i := 0; i < 50; i++ {
		sb.WriteString("INFO boot\n")
	}
	sb.WriteString("WARN last\npartial")
	appendFile(t, p, sb.String())
	srv := httptest.NewServer(http.HandlerFunc(logsFollowHandler))
	defer srv.Close()

	resp, err := http.Get(srv.URL + "?path=" + url.QueryEscape(p))
	if err != nil {
		t.Fatal(err)
	}
	if ev := nextEvent(t, readSSE(resp.Body)); ev.Event != "fail" || !strings.Contains(ev.Data, "not in the saved") {
		t.Errorf("unsaved path: %+v", ev)
	}
	resp.Body.Close()

	if _, err := logsAdd(p); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, "GET", srv.URL+"?lines=3&path="+url.QueryEscape(p), nil)
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	ch := readSSE(resp.Body)
	var first []string
	_ = json.Unmarshal([]byte(nextEvent(t, ch).Data), &first)
	if strings.Join(first, "|") != "INFO boot|INFO boot|WARN last" {
		t.Errorf("initial tail %q", first)
	}
	appendFile(t, p, " now\nERROR next\n")
	var got []string
	for len(got) < 2 {
		ev := nextEvent(t, ch)
		var ls []string
		_ = json.Unmarshal([]byte(ev.Data), &ls)
		got = append(got, ls...)
	}
	if strings.Join(got, "|") != "partial now|ERROR next" {
		t.Errorf("followed %q", got)
	}
}
