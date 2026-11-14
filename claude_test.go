package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// The real `claude` binary is never run in tests: DEVDECK_CLAUDE_BIN points at this test binary,
// which acts as a fake claude when DEVDECK_FAKE_CLAUDE=1 (see TestMain).

func TestMain(m *testing.M) {
	if os.Getenv("DEVDECK_FAKE_CLAUDE") == "1" {
		fakeClaude()
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func fakeClaude() {
	in, _ := io.ReadAll(os.Stdin)
	fmt.Printf("ARGS:%s\n", strings.Join(os.Args[1:], " "))
	wd, _ := os.Getwd()
	fmt.Printf("CWD:%s\n", wd)
	if os.Getenv("FAKE_MODE") == "slow" {
		fmt.Println("started")
		time.Sleep(60 * time.Second)
		return
	}
	if os.Getenv("FAKE_MODE") == "fail" {
		fmt.Fprintln(os.Stderr, "fake failure")
		os.Exit(3)
	}
	fmt.Print("FAKE:" + string(in) + "héllo ✓")
}

func useFakeClaude(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("DEVDECK_CLAUDE_BIN", exe)
	t.Setenv("DEVDECK_FAKE_CLAUDE", "1")
}

func testServer(t *testing.T) *httptest.Server {
	mux := http.NewServeMux()
	for _, f := range routeRegistrars {
		f(mux)
	}
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func resetSessions() {
	claudeSessions.Lock()
	claudeSessions.m = map[string]*ClaudeSession{}
	claudeSessions.Unlock()
}

// ------------------------------------------------------------ transcript

const fixtureTranscript = `{"type":"summary","summary":"x"}
{"type":"user","timestamp":"2026-11-14T10:00:00.000Z","gitBranch":"main","version":"2.1.0","message":{"role":"user","content":"Fix the login bug"}}
{"type":"assistant","timestamp":"2026-11-14T10:00:05.000Z","message":{"id":"msg_1","role":"assistant","model":"claude-opus-4","content":[{"type":"thinking","thinking":"hmm"}],"usage":{"input_tokens":10,"output_tokens":5,"cache_read_input_tokens":100,"cache_creation_input_tokens":20}}}
{"type":"assistant","timestamp":"2026-11-14T10:00:06.000Z","message":{"id":"msg_1","role":"assistant","model":"claude-opus-4","content":[{"type":"tool_use","name":"Read"}],"usage":{"input_tokens":10,"output_tokens":7,"cache_read_input_tokens":100,"cache_creation_input_tokens":20}}}
{"type":"user","timestamp":"2026-11-14T10:00:07.000Z","message":{"role":"user","content":[{"type":"tool_result","content":"file text"}]}}
{"type":"user","timestamp":"2026-11-14T10:00:08.000Z","isMeta":true,"message":{"role":"user","content":"Caveat: meta"}}
{"type":"user","timestamp":"2026-11-14T10:01:00.000Z","message":{"role":"user","content":[{"type":"text","text":"Now add a test"}]}}
{"type":"user","timestamp":"2026-11-14T10:01:01.000Z","message":{"role":"user","content":"<command-name>/clear</command-name>"}}
not json at all
{"type":"assistant","timestamp":"2026-11-14T10:02:30.000Z","message":{"id":"msg_2","role":"assistant","model":"claude-sonnet-4-5","content":[{"type":"text","text":"Added **TestLogin**."}],"usage":{"input_tokens":3,"output_tokens":40,"cache_read_input_tokens":200,"cache_creation_input_tokens":0}}}
{"type":"assistant","timestamp":"2026-11-14T10:02:31.000Z","message":{"id":"msg_3","role":"assistant","model":"<synthetic>","content":[{"type":"text","text":"   "}]}}
`

func TestTranscriptParse(t *testing.T) {
	p := filepath.Join(t.TempDir(), "s.jsonl")
	if err := os.WriteFile(p, []byte(fixtureTranscript), 0o600); err != nil {
		t.Fatal(err)
	}
	ti, err := transcriptInfo(p)
	if err != nil {
		t.Fatal(err)
	}
	if ti.LastPrompt != "Now add a test" {
		t.Errorf("lastPrompt = %q", ti.LastPrompt)
	}
	if ti.LastReply != "Added **TestLogin**." {
		t.Errorf("lastReply = %q", ti.LastReply)
	}
	if ti.Model != "claude-sonnet-4-5" {
		t.Errorf("model = %q", ti.Model)
	}
	// msg_1 counted once (last usage wins), plus msg_2
	if ti.InputTokens != 13 || ti.OutputToks != 47 || ti.CacheRead != 300 || ti.CacheWrite != 20 {
		t.Errorf("tokens = %+v", ti)
	}
	if ti.Messages != 2 || ti.Branch != "main" || ti.Version != "2.1.0" || ti.Partial {
		t.Errorf("meta = %+v", ti)
	}
	if ti.Last-ti.Started != 151 {
		t.Errorf("duration = %d", ti.Last-ti.Started)
	}
	if _, err := transcriptInfo(filepath.Join(t.TempDir(), "x.txt")); err == nil {
		t.Error("non-jsonl path should be refused")
	}
	if _, err := transcriptInfo("relative.jsonl"); err == nil {
		t.Error("relative path should be refused")
	}
}

func TestTranscriptTail(t *testing.T) {
	p := filepath.Join(t.TempDir(), "big.jsonl")
	f, err := os.Create(p)
	if err != nil {
		t.Fatal(err)
	}
	first := `{"type":"user","timestamp":"2026-11-14T08:00:00Z","message":{"role":"user","content":"very first"}}` + "\n"
	f.WriteString(first)
	pad := `{"type":"progress","data":"` + strings.Repeat("x", 1000) + `"}` + "\n"
	for i := 0; i < 2600; i++ { // ~2.6 MB, more than the tail window
		f.WriteString(pad)
	}
	f.WriteString(`{"type":"user","timestamp":"2026-11-14T09:00:00Z","message":{"role":"user","content":"latest"}}` + "\n")
	f.Close()
	ti, err := parseTranscript(p)
	if err != nil {
		t.Fatal(err)
	}
	if !ti.Partial || ti.LastPrompt != "latest" || ti.Last-ti.Started != 3600 {
		t.Errorf("tail parse = %+v", ti)
	}
}

// ------------------------------------------------------------ SSE

type sseReader struct {
	ch   chan claudeMsg
	stop context.CancelFunc
}

func openSSE(t *testing.T, url string) *sseReader {
	ctx, cancel := context.WithCancel(context.Background())
	req, _ := http.NewRequestWithContext(ctx, "GET", url+"/api/claude/events", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "text/event-stream" {
		t.Fatalf("content type %q", ct)
	}
	r := &sseReader{ch: make(chan claudeMsg, 32), stop: cancel}
	go func() {
		defer resp.Body.Close()
		sc := bufio.NewScanner(resp.Body)
		sc.Buffer(make([]byte, 1<<20), 1<<20)
		for sc.Scan() {
			if d, ok := strings.CutPrefix(sc.Text(), "data: "); ok {
				var m claudeMsg
				if json.Unmarshal([]byte(d), &m) == nil {
					r.ch <- m
				}
			}
		}
		close(r.ch)
	}()
	return r
}

func (r *sseReader) next(t *testing.T) claudeMsg {
	t.Helper()
	select {
	case m, ok := <-r.ch:
		if !ok {
			t.Fatal("stream closed")
		}
		return m
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for an event")
	}
	return claudeMsg{}
}

func hookEv(id, ev string, extra map[string]any) {
	p := map[string]any{"session_id": id, "hook_event_name": ev, "cwd": "/work/app", "transcript_path": "/tmp/t.jsonl"}
	for k, v := range extra {
		p[k] = v
	}
	recordClaudeEvent(p)
}

func TestClaudeEventsSSE(t *testing.T) {
	resetSessions()
	srv := testServer(t)
	r := openSSE(t, srv.URL)
	if m := r.next(t); len(m.Sessions) != 0 || m.Alert != nil {
		t.Fatalf("initial snapshot = %+v", m)
	}
	id := "0b1c2d3e-0000-4000-8000-000000000001"
	hookEv(id, "SessionStart", nil)
	if m := r.next(t); len(m.Sessions) != 1 || m.Sessions[0].State != "idle" || m.Alert != nil || m.Sessions[0].Transcript != "/tmp/t.jsonl" {
		t.Fatalf("after start = %+v", m)
	}
	hookEv(id, "UserPromptSubmit", nil)
	r.next(t)
	hookEv(id, "PreToolUse", map[string]any{"tool_name": "Bash"})
	if m := r.next(t); m.Sessions[0].State != "working" || m.Sessions[0].Tool != "Bash" {
		t.Fatalf("working = %+v", m)
	}
	hookEv(id, "Notification", map[string]any{"message": "Claude needs your permission to use Bash"})
	if m := r.next(t); m.Alert == nil || m.Alert.Kind != "waiting" || m.Alert.ID != id || m.Sessions[0].State != "waiting" {
		t.Fatalf("waiting alert = %+v", m)
	}
	hookEv(id, "PreToolUse", nil)
	r.next(t)
	hookEv(id, "Stop", nil)
	if m := r.next(t); m.Alert == nil || m.Alert.Kind != "done" || m.Sessions[0].State != "idle" {
		t.Fatalf("done alert = %+v", m)
	}
	// idle reminder after a finished turn: state changes, no second alert
	hookEv(id, "Notification", map[string]any{"message": "Claude is waiting for your input"})
	if m := r.next(t); m.Alert != nil {
		t.Fatalf("idle reminder should not alert: %+v", m.Alert)
	}
	if !dismissClaudeSession(id) {
		t.Fatal("dismiss failed")
	}
	if m := r.next(t); len(m.Sessions) != 0 {
		t.Fatalf("after dismiss = %+v", m)
	}
	if n := claudeHub.count(); n != 1 {
		t.Fatalf("subscribers = %d", n)
	}
	r.stop()
	deadline := time.Now().Add(3 * time.Second)
	for claudeHub.count() != 0 && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if n := claudeHub.count(); n != 0 {
		t.Fatalf("subscriber not cleaned up: %d", n)
	}
}

// ------------------------------------------------------------ ask

type askLine struct {
	T    string `json:"t"`
	Err  string `json:"err"`
	Done bool   `json:"done"`
}

func postAsk(ctx context.Context, url string, body map[string]string) (*http.Response, error) {
	b, _ := json.Marshal(body)
	req, _ := http.NewRequestWithContext(ctx, "POST", url+"/api/claude/ask", strings.NewReader(string(b)))
	req.Header.Set("Content-Type", "application/json")
	return http.DefaultClient.Do(req)
}

func readAsk(t *testing.T, resp *http.Response) (string, askLine) {
	t.Helper()
	defer resp.Body.Close()
	var sb strings.Builder
	var last askLine
	sc := bufio.NewScanner(resp.Body)
	for sc.Scan() {
		var l askLine
		if err := json.Unmarshal(sc.Bytes(), &l); err != nil {
			t.Fatalf("bad line %q", sc.Text())
		}
		sb.WriteString(l.T)
		last = l
	}
	return sb.String(), last
}

func TestClaudeAskStreaming(t *testing.T) {
	useFakeClaude(t)
	srv := testServer(t)
	dir := t.TempDir()
	resp, err := postAsk(context.Background(), srv.URL, map[string]string{"prompt": "Explain --dangerously-skip-permissions $(rm -rf /)", "context": "diff here", "cwd": dir})
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 200 || !strings.HasPrefix(resp.Header.Get("Content-Type"), "application/x-ndjson") {
		t.Fatalf("status %d %s", resp.StatusCode, resp.Header.Get("Content-Type"))
	}
	out, last := readAsk(t, resp)
	if !last.Done {
		t.Fatalf("no done marker, last = %+v, out = %q", last, out)
	}
	if !strings.Contains(out, "ARGS:-p\n") {
		t.Errorf("claude must get exactly -p, got %q", out)
	}
	wantCwd, _ := filepath.EvalSymlinks(dir)
	if !strings.Contains(out, "CWD:"+dir) && !strings.Contains(out, "CWD:"+wantCwd) {
		t.Errorf("cwd not applied: %q", out)
	}
	if !strings.Contains(out, "FAKE:Explain --dangerously-skip-permissions $(rm -rf /)\n\n<context>\ndiff here\n</context>") || !strings.HasSuffix(out, "héllo ✓") {
		t.Errorf("stdin not passed through: %q", out)
	}

	// invalid cwd and empty prompt are rejected before anything runs
	for _, b := range []map[string]string{{"prompt": "x", "cwd": filepath.Join(dir, "missing")}, {"prompt": " "}, {"prompt": "x", "cwd": "-rf"}} {
		resp, err := postAsk(context.Background(), srv.URL, b)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != 400 {
			t.Errorf("%v: status %d", b, resp.StatusCode)
		}
	}

	// failing binary: error marker with stderr
	t.Setenv("FAKE_MODE", "fail")
	resp, err = postAsk(context.Background(), srv.URL, map[string]string{"prompt": "x", "cwd": dir})
	if err != nil {
		t.Fatal(err)
	}
	if _, last := readAsk(t, resp); !strings.Contains(last.Err, "fake failure") {
		t.Errorf("failure not reported: %+v", last)
	}
}

func TestClaudeAskCancel(t *testing.T) {
	useFakeClaude(t)
	t.Setenv("FAKE_MODE", "slow")
	srv := testServer(t)
	ctx, cancel := context.WithCancel(context.Background())
	resp, err := postAsk(ctx, srv.URL, map[string]string{"prompt": "x", "cwd": t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	sc := bufio.NewScanner(resp.Body)
	got := ""
	for !strings.Contains(got, "started") && sc.Scan() {
		var l askLine
		_ = json.Unmarshal(sc.Bytes(), &l)
		got += l.T
	}
	if !strings.Contains(got, "started") {
		t.Fatalf("no streamed output before cancel: %q", got)
	}
	// both slots: a second slow request runs, a third is refused while two are active
	ctx2, cancel2 := context.WithCancel(context.Background())
	resp2, err := postAsk(ctx2, srv.URL, map[string]string{"prompt": "x"})
	if err != nil {
		t.Fatal(err)
	}
	waitFor(t, func() bool { return len(askSlots) == 2 })
	resp3, err := postAsk(context.Background(), srv.URL, map[string]string{"prompt": "x"})
	if err != nil {
		t.Fatal(err)
	}
	resp3.Body.Close()
	if resp3.StatusCode != 429 {
		t.Errorf("third request status %d, want 429", resp3.StatusCode)
	}
	start := time.Now()
	cancel()
	cancel2()
	resp.Body.Close()
	resp2.Body.Close()
	waitFor(t, func() bool { return len(askSlots) == 0 })
	if d := time.Since(start); d > 10*time.Second {
		t.Errorf("cancel took %s", d)
	}
}

func waitFor(t *testing.T, f func() bool) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for !f() {
		if time.Now().After(deadline) {
			t.Fatal("condition not reached")
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func TestUTF8Cut(t *testing.T) {
	b := []byte("aé✓")
	for i := 0; i <= len(b); i++ {
		c := utf8Cut(b[:i])
		if !strings.HasPrefix("aé✓", string(b[:c])) || c > i {
			t.Fatalf("cut(%d) = %d", i, c)
		}
	}
	if utf8Cut([]byte("a\xc3")) != 1 || utf8Cut([]byte("a\xe2\x9c")) != 1 || utf8Cut([]byte("a\xe2\x9c\x93")) != 4 {
		t.Fatal("partial rune handling")
	}
}

// ------------------------------------------------------------ hooks status

func TestHookStatus(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	if hs := claudeHookStatus(); hs.Found || hs.Installed || len(hs.Missing) != len(recommendedHookEvents) || !strings.Contains(hs.Snippet, "hook") {
		t.Fatalf("no settings: %+v", hs)
	}
	os.MkdirAll(filepath.Join(home, ".claude"), 0o700)
	settings := `{"hooks":{"Stop":[{"hooks":[{"type":"command","command":"\"C:\\tools\\devdeck.exe\" hook"}]}],"PreToolUse":[{"matcher":"","hooks":[{"type":"command","command":"other"}]}]}}`
	p := filepath.Join(home, ".claude", "settings.json")
	os.WriteFile(p, []byte(settings), 0o600)
	before, _ := os.ReadFile(p)
	hs := claudeHookStatus()
	if !hs.Found || !hs.Installed || len(hs.Events) != 1 || hs.Events[0] != "Stop" || len(hs.Missing) != 5 {
		t.Fatalf("status = %+v", hs)
	}
	after, _ := os.ReadFile(p)
	if string(before) != string(after) {
		t.Fatal("settings.json must never be modified")
	}
}

// ------------------------------------------------------------ git: commit action, context, MCP tools

func TestCommitAction(t *testing.T) {
	repo := newRepo(t)
	write(t, repo, "b.txt", "new\n")
	write(t, repo, "a.txt", "changed\n")
	msg := "--amend -m evil\n\nBody with `$(touch pwned)` and \"quotes\""
	act(t, repo, "commit", aargs{"message": msg, "all": true})
	if got := tg(t, repo, "log", "-1", "--format=%B"); got != msg {
		t.Fatalf("message = %q", got)
	}
	if st := tg(t, repo, "status", "--porcelain"); st != "" {
		t.Fatalf("all changes should be committed: %q", st)
	}
	if _, err := os.Stat(filepath.Join(repo, "pwned")); err == nil {
		t.Fatal("message was executed")
	}
	n := tg(t, repo, "rev-list", "--count", "HEAD")
	// amend without a message keeps the old one
	write(t, repo, "c.txt", "c\n")
	tg(t, repo, "add", "c.txt")
	act(t, repo, "commit", aargs{"amend": true})
	if tg(t, repo, "rev-list", "--count", "HEAD") != n || tg(t, repo, "log", "-1", "--format=%B") != msg {
		t.Fatal("amend should keep the commit count and message")
	}
	act(t, repo, "commit", aargs{"amend": true, "message": "reworded"})
	if tg(t, repo, "log", "-1", "--format=%s") != "reworded" {
		t.Fatal("amend with message should reword")
	}
	actFails(t, repo, "commit", aargs{"message": "   "})
	actFails(t, repo, "commit", aargs{"message": "nothing staged"})
	write(t, repo, "d.txt", "d\n")
	actFails(t, repo, "commit", aargs{"message": "untracked only, not staged"})
	if out := act(t, repo, "commit", aargs{"message": "with d", "all": true}); !strings.Contains(out, "with d") {
		t.Logf("commit output: %s", out)
	}

	// context helpers used by the Claude prompts
	head := tg(t, repo, "rev-parse", "HEAD")
	text, _, err := claudeGitContext(repo, "commit", map[string]string{"hash": head})
	if err != nil || !strings.Contains(text, "with d") || !strings.Contains(text, "+d") {
		t.Fatalf("commit ctx: %v %q", err, text)
	}
	write(t, repo, "a.txt", "again\n")
	text, _, err = claudeGitContext(repo, "uncommitted", map[string]string{})
	if err != nil || !strings.Contains(text, "+again") {
		t.Fatalf("uncommitted ctx: %v %q", err, text)
	}
	if _, _, err := claudeGitContext(repo, "uncommitted", map[string]string{"staged": "1"}); err == nil {
		t.Fatal("nothing staged should be an error")
	}
	act(t, repo, "branch.create", aargs{"name": "feat", "hash": head, "checkout": true})
	commit(t, repo, "f.txt", "feature\n", "add feature")
	text, _, err = claudeGitContext(repo, "branch", map[string]string{"branch": "feat", "base": "main"})
	if err != nil || !strings.Contains(text, "add feature") || !strings.Contains(text, "+feature") {
		t.Fatalf("branch ctx: %v %q", err, text)
	}
	if _, _, err := claudeGitContext(repo, "branch", map[string]string{"branch": "--output=x", "base": "main"}); err == nil {
		t.Fatal("bad branch name accepted")
	}
	if _, _, err := claudeGitContext(repo, "commit", map[string]string{"hash": "HEAD;rm"}); err == nil {
		t.Fatal("bad hash accepted")
	}
}

func TestMCPGitTools(t *testing.T) {
	repo := newRepo(t)
	tools := map[string]tool{}
	for _, tl := range mcpTools() {
		tools[tl.Name] = tl
	}
	for _, n := range []string{"list_ports", "git_graph", "git_show", "claude_sessions", "git_status", "git_log_search", "git_create_branch", "git_stash"} {
		if _, ok := tools[n]; !ok {
			t.Fatalf("missing tool %s", n)
		}
	}
	call := func(name string, a map[string]any) (string, error) { return tools[name].handler(a) }
	if out, err := call("git_status", map[string]any{"repo": repo}); err != nil || !strings.Contains(out, "## main") || !strings.Contains(out, "clean") {
		t.Fatalf("git_status: %v %q", err, out)
	}
	write(t, repo, "a.txt", "dirty\n")
	if out, _ := call("git_status", map[string]any{"repo": repo}); !strings.Contains(out, " M a.txt") {
		t.Fatalf("git_status dirty: %q", out)
	}
	if out, err := call("git_log_search", map[string]any{"repo": repo, "query": "SECOND", "limit": float64(5)}); err != nil || !strings.Contains(out, "second") || strings.Contains(out, "first") {
		t.Fatalf("git_log_search: %v %q", err, out)
	}
	if out, _ := call("git_log_search", map[string]any{"repo": repo, "query": "nope-nothing"}); !strings.Contains(out, "no matching") {
		t.Fatalf("git_log_search empty: %q", out)
	}
	if _, err := call("git_create_branch", map[string]any{"repo": repo, "name": "topic"}); err != nil {
		t.Fatal(err)
	}
	if _, err := call("git_create_branch", map[string]any{"repo": repo, "name": "topic"}); err == nil {
		t.Fatal("existing branch must not be overwritten")
	}
	if _, err := call("git_create_branch", map[string]any{"repo": repo, "name": "-f"}); err == nil {
		t.Fatal("invalid branch name accepted")
	}
	if _, err := call("git_stash", map[string]any{"repo": repo, "message": "wip from mcp"}); err != nil {
		t.Fatal(err)
	}
	if l := tg(t, repo, "stash", "list"); !strings.Contains(l, "wip from mcp") {
		t.Fatalf("stash list: %q", l)
	}
	if _, err := call("git_status", map[string]any{"repo": "-C"}); err == nil {
		t.Fatal("bad repo accepted")
	}
}
