package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// A fake docker CLI, built once per test run and selected through DEVDECK_DOCKER_BIN.
// FAKE_DOCKER_MODE=down simulates a stopped daemon; every call is appended to FAKE_DOCKER_LOG.
const fakeDockerSrc = `package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

func has(args []string, s string) bool {
	for _, a := range args {
		if a == s {
			return true
		}
	}
	return false
}

func main() {
	args := os.Args[1:]
	if p := os.Getenv("FAKE_DOCKER_LOG"); p != "" {
		f, _ := os.OpenFile(p, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
		b, _ := json.Marshal(map[string]any{"args": args, "dir": func() string { d, _ := os.Getwd(); return d }()})
		f.Write(append(b, '\n'))
		f.Close()
	}
	down := "failed to connect to the docker API at npipe:////./pipe/dockerDesktopLinuxEngine; check if the path is correct and if the daemon is running"
	if os.Getenv("FAKE_DOCKER_MODE") == "down" {
		if len(args) > 0 && args[0] == "version" {
			fmt.Println(` + "`" + `{"Client":{"Version":"29.0.0"},"Server":null}` + "`" + `)
		}
		fmt.Fprintln(os.Stderr, down)
		os.Exit(1)
	}
	if len(args) == 0 {
		os.Exit(2)
	}
	switch args[0] {
	case "version":
		fmt.Println(` + "`" + `{"Client":{"Version":"29.0.0"},"Server":{"Version":"29.0.1"}}` + "`" + `)
	case "ps":
		fmt.Println(` + "`" + `{"Command":"\"nginx -g\"","CreatedAt":"2026-01-01","ID":"0123456789ab","Image":"nginx:1","Labels":"com.docker.compose.project=shop,x=y","Names":"web","Ports":"0.0.0.0:8080->80/tcp, [::]:8080->80/tcp","RunningFor":"2 hours ago","State":"running","Status":"Up 2 hours"}` + "`" + `)
		fmt.Println(` + "`" + `{"Command":"pg","ID":"ba9876543210","Image":"postgres:16","Labels":"","Names":"db","Ports":"5432/tcp","RunningFor":"3 hours ago","State":"running","Status":"Up 3 hours"}` + "`" + `)
		if has(args, "-a") {
			fmt.Println(` + "`" + `{"Command":"sh","ID":"cccccccccccc","Image":"alpine","Labels":"","Names":"old","Ports":"","RunningFor":"2 days ago","State":"exited","Status":"Exited (0) 2 days ago"}` + "`" + `)
		}
	case "start", "stop", "restart", "rm":
		id := args[len(args)-1]
		if id == "missing" {
			fmt.Fprintln(os.Stderr, "Error response from daemon: No such container: missing")
			os.Exit(1)
		}
		fmt.Println(id)
	case "logs":
		n := 3
		for i, a := range args {
			if a == "--tail" && i+1 < len(args) {
				n, _ = strconv.Atoi(args[i+1])
			}
		}
		if args[len(args)-1] == "missing" {
			fmt.Fprintln(os.Stderr, "Error response from daemon: No such container: missing")
			os.Exit(1)
		}
		for i := 1; i <= n; i++ {
			fmt.Printf("\x1b[32mline %d\x1b[0m\n", i)
		}
		fmt.Fprintln(os.Stderr, "ERROR boom")
		if has(args, "-f") {
			if os.Getenv("FAKE_DOCKER_EXIT") == "1" {
				return
			}
			for k := 1; k <= 2000; k++ {
				time.Sleep(20 * time.Millisecond)
				fmt.Printf("tick %d\n", k)
			}
		}
	case "stats":
		fmt.Println(` + "`" + `{"BlockIO":"0B / 0B","CPUPerc":"0.50%","Container":"web","ID":"0123456789ab","MemPerc":"1.00%","MemUsage":"10MiB / 1GiB","Name":"web","NetIO":"1kB / 2kB","PIDs":"3"}` + "`" + `)
	case "inspect":
		fmt.Println(` + "`" + `[{"Id":"0123456789ab","Name":"/web","Config":{"Env":["A=1"]}}]` + "`" + `)
	case "images":
		fmt.Println(` + "`" + `{"Containers":"N/A","CreatedSince":"2 weeks ago","ID":"sha256:aaaa","Repository":"nginx","Size":"190MB","Tag":"1"}` + "`" + `)
	case "rmi":
		if args[len(args)-1] == "busy" && !has(args, "-f") {
			fmt.Fprintln(os.Stderr, "Error response from daemon: conflict: unable to remove repository reference \"busy\" - container is using its referenced image")
			os.Exit(1)
		}
		fmt.Println("Deleted: " + args[len(args)-1])
	case "compose":
		if has(args, "ls") {
			b, _ := json.Marshal([]map[string]string{{"Name": "shop", "Status": "running(2)", "ConfigFiles": os.Getenv("FAKE_COMPOSE_FILE")}})
			fmt.Println(string(b))
			return
		}
		fmt.Println("compose done: " + strings.Join(args, " "))
	default:
		fmt.Fprintln(os.Stderr, "unknown command "+args[0])
		os.Exit(1)
	}
}
`

// buildFakeDocker compiles the fake CLI into the test's temp dir (the Go build cache keeps repeats fast).
func buildFakeDocker(t *testing.T) string {
	t.Helper()
	goBin, err := exec.LookPath("go")
	if err != nil {
		t.Skip("go toolchain not found: ", err)
	}
	dir := t.TempDir()
	src := filepath.Join(dir, "fakedocker.go")
	if err := os.WriteFile(src, []byte(fakeDockerSrc), 0o644); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(dir, "docker")
	if runtime.GOOS == "windows" {
		out += ".exe"
	}
	cmd := exec.Command(goBin, "build", "-o", out, src)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GOFLAGS=", "GO111MODULE=on", "CGO_ENABLED=0")
	if b, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("cannot build fake docker: %v\n%s", err, b)
	}
	return out
}

func useFakeDocker(t *testing.T) (logPath string) {
	t.Helper()
	bin := buildFakeDocker(t)
	logPath = filepath.Join(t.TempDir(), "calls.log")
	t.Setenv("DEVDECK_DOCKER_BIN", bin)
	t.Setenv("FAKE_DOCKER_LOG", logPath)
	t.Setenv("FAKE_DOCKER_MODE", "")
	return logPath
}

type fakeCall struct {
	Args []string `json:"args"`
	Dir  string   `json:"dir"`
}

func readCalls(t *testing.T, p string) []fakeCall {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		return nil
	}
	var res []fakeCall
	for _, l := range strings.Split(strings.TrimSpace(string(b)), "\n") {
		var c fakeCall
		if json.Unmarshal([]byte(l), &c) == nil {
			res = append(res, c)
		}
	}
	return res
}

func TestParsePorts(t *testing.T) {
	ps := parsePorts("0.0.0.0:8080->80/tcp, [::]:8080->80/tcp, :::9000->9000/udp, 5432/tcp, 127.0.0.1:7000-7002->7000-7002/tcp")
	want := []dockerPort{
		{IP: "0.0.0.0", Host: 8080, Container: 80, Proto: "tcp"},
		{IP: "::", Host: 9000, Container: 9000, Proto: "udp"},
		{Container: 5432, Proto: "tcp"},
		{IP: "127.0.0.1", Host: 7000, Container: 7000, Proto: "tcp"},
		{IP: "127.0.0.1", Host: 7001, Container: 7001, Proto: "tcp"},
		{IP: "127.0.0.1", Host: 7002, Container: 7002, Proto: "tcp"},
	}
	if len(ps) != len(want) {
		t.Fatalf("got %+v", ps)
	}
	for i := range want {
		if ps[i] != want[i] {
			t.Errorf("port %d: got %+v want %+v", i, ps[i], want[i])
		}
	}
	if len(parsePorts("")) != 0 {
		t.Error("empty ports")
	}
}

func TestDockerValidation(t *testing.T) {
	logPath := useFakeDocker(t)
	ctx := context.Background()
	for _, bad := range []string{"", "-rf", "--help", "a b", "a;b", "web/../x", "$(x)", strings.Repeat("a", 200), "ä"} {
		if err := dockerContainerAction(ctx, bad, "stop", false); err == nil {
			t.Errorf("id %q accepted", bad)
		}
		if _, err := dockerLogsOnce(ctx, bad, 10); err == nil {
			t.Errorf("logs id %q accepted", bad)
		}
		if _, err := dockerInspect(ctx, bad); err == nil {
			t.Errorf("inspect id %q accepted", bad)
		}
	}
	if err := dockerContainerAction(ctx, "web", "exec", false); err == nil {
		t.Error("unknown action accepted")
	}
	for _, bad := range []string{"-f", "--force", "a b", ""} {
		if err := dockerRemoveImage(ctx, bad, false); err == nil {
			t.Errorf("image %q accepted", bad)
		}
	}
	for _, bad := range []string{"Shop", "-p", "a b", "../x", ""} {
		if _, err := composeAction(ctx, bad, "up"); err == nil {
			t.Errorf("compose name %q accepted", bad)
		}
	}
	if _, err := composeAction(ctx, "shop", "rm"); err == nil {
		t.Error("unknown compose action accepted")
	}
	if calls := readCalls(t, logPath); len(calls) != 0 {
		t.Fatalf("invalid input must not reach docker, got %v", calls)
	}
}

func TestDockerFake(t *testing.T) {
	logPath := useFakeDocker(t)
	ctx := context.Background()

	if st := dockerStatusCheck(ctx); st.State != "ok" || st.Server != "29.0.1" {
		t.Fatalf("status %+v", st)
	}
	cs, err := dockerContainers(ctx, false)
	if err != nil || len(cs) != 2 {
		t.Fatalf("containers %v %+v", err, cs)
	}
	web := cs[0]
	if web.Name != "web" || web.State != "running" || web.Compose != "shop" || len(web.Ports) != 1 || web.Ports[0].Host != 8080 || web.Command != "nginx -g" {
		t.Errorf("web %+v", web)
	}
	if all, _ := dockerContainers(ctx, true); len(all) != 3 || all[2].State != "exited" {
		t.Errorf("all %+v", all)
	}

	if err := dockerContainerAction(ctx, "web", "restart", false); err != nil {
		t.Fatal(err)
	}
	if err := dockerContainerAction(ctx, "old", "remove", true); err != nil {
		t.Fatal(err)
	}
	err = dockerContainerAction(ctx, "missing", "stop", false)
	var de *dockerError
	if !errors.As(err, &de) || de.State != "error" || !strings.Contains(de.Msg, "No such container") {
		t.Errorf("missing container error: %v", err)
	}

	lines, err := dockerLogsOnce(ctx, "web", 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(lines) != 5 || lines[0] != "line 2" || lines[4] != "ERROR boom" {
		t.Errorf("logs (ANSI stripped, stderr merged, last 5): %q", lines)
	}

	st, err := dockerStats(ctx, "")
	if err != nil || len(st) != 1 || st[0].CPUPerc != "0.50%" || st[0].Name != "web" {
		t.Errorf("stats %v %+v", err, st)
	}
	raw, err := dockerInspect(ctx, "web")
	if err != nil || !strings.Contains(string(raw), `"Name":"/web"`) {
		t.Errorf("inspect %v %s", err, raw)
	}
	im, err := dockerImages(ctx)
	if err != nil || len(im) != 1 || im[0].Repository != "nginx" || im[0].Created != "2 weeks ago" {
		t.Errorf("images %v %+v", err, im)
	}
	if err := dockerRemoveImage(ctx, "busy", false); err == nil || !strings.Contains(err.Error(), "conflict") {
		t.Errorf("rmi conflict: %v", err)
	}
	if err := dockerRemoveImage(ctx, "busy", true); err != nil {
		t.Error(err)
	}

	// compose: config files come from `compose ls`, the working dir is the file's folder.
	dir := t.TempDir()
	cf := filepath.Join(dir, "compose.yaml")
	if err := os.WriteFile(cf, []byte("services: {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("FAKE_COMPOSE_FILE", cf)
	ps, err := composeProjects(ctx)
	if err != nil || len(ps) != 1 || ps[0].Name != "shop" || len(ps[0].ConfigFiles) != 1 {
		t.Fatalf("compose ls %v %+v", err, ps)
	}
	out, err := composeAction(ctx, "shop", "up")
	if err != nil || !strings.Contains(out, "-f "+cf) || !strings.HasSuffix(out, "up -d") {
		t.Errorf("compose up: %v %q", err, out)
	}
	if _, err := composeAction(ctx, "nope", "down"); err == nil {
		t.Error("unknown project accepted")
	}
	t.Setenv("FAKE_COMPOSE_FILE", filepath.Join(dir, "gone.yaml"))
	if _, err := composeAction(ctx, "shop", "up"); err == nil {
		t.Error("up without compose file accepted")
	}
	if out, err := composeAction(ctx, "shop", "down"); err != nil || strings.Contains(out, "-f") {
		t.Errorf("down without files: %v %q", err, out)
	}

	calls := readCalls(t, logPath)
	var sawRm, sawUpDir bool
	for _, c := range calls {
		j := strings.Join(c.Args, " ")
		if j == "rm -f old" {
			sawRm = true
		}
		if strings.HasSuffix(j, "up -d") && samePath(filepath.Clean(c.Dir), filepath.Clean(dir)) {
			sawUpDir = true
		}
	}
	if !sawRm || !sawUpDir {
		t.Errorf("calls: rm=%v upDir=%v %+v", sawRm, sawUpDir, calls)
	}
}

func TestDockerDownAndMissing(t *testing.T) {
	useFakeDocker(t)
	t.Setenv("FAKE_DOCKER_MODE", "down")
	ctx := context.Background()
	if st := dockerStatusCheck(ctx); st.State != "down" || st.Client != "29.0.0" {
		t.Errorf("status %+v", st)
	}
	_, err := dockerContainers(ctx, true)
	var de *dockerError
	if !errors.As(err, &de) || de.State != "down" {
		t.Errorf("containers when down: %v", err)
	}
	rec := httptest.NewRecorder()
	dockerFail(rec, err)
	if rec.Code != 503 || !strings.Contains(rec.Body.String(), `"state":"down"`) {
		t.Errorf("http %d %s", rec.Code, rec.Body)
	}

	t.Setenv("DEVDECK_DOCKER_BIN", filepath.Join(t.TempDir(), "no-docker-here"))
	if st := dockerStatusCheck(ctx); st.State != "missing" {
		t.Errorf("missing status %+v", st)
	}
	if _, err := dockerImages(ctx); !errors.As(err, &de) || de.State != "missing" {
		t.Errorf("images when missing: %v", err)
	}
}

type sseEvent struct {
	Event string
	Data  string
}

// readSSE parses events from an SSE body and sends them on a channel until the body ends.
func readSSE(body interface{ Read([]byte) (int, error) }) <-chan sseEvent {
	ch := make(chan sseEvent, 256)
	go func() {
		defer close(ch)
		sc := bufio.NewScanner(body)
		sc.Buffer(make([]byte, 1<<20), 8<<20)
		var ev sseEvent
		for sc.Scan() {
			l := sc.Text()
			switch {
			case l == "":
				if ev.Data != "" {
					ch <- ev
				}
				ev = sseEvent{}
			case strings.HasPrefix(l, "event: "):
				ev.Event = l[7:]
			case strings.HasPrefix(l, "data: "):
				ev.Data = l[6:]
			}
		}
	}()
	return ch
}

func nextEvent(t *testing.T, ch <-chan sseEvent) sseEvent {
	t.Helper()
	select {
	case ev, ok := <-ch:
		if !ok {
			t.Fatal("stream closed")
		}
		return ev
	case <-time.After(10 * time.Second):
		t.Fatal("timeout waiting for event")
	}
	return sseEvent{}
}

func TestDockerLogsStream(t *testing.T) {
	useFakeDocker(t)
	done := make(chan struct{}, 4)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		dockerLogsStream(w, r, r.URL.Query().Get("id"), atoi(r.URL.Query().Get("tail")))
		done <- struct{}{}
	}))
	defer srv.Close()

	// bad id: a fail event, no process
	resp, err := http.Get(srv.URL + "?id=-x")
	if err != nil {
		t.Fatal(err)
	}
	if ev := nextEvent(t, readSSE(resp.Body)); ev.Event != "fail" {
		t.Errorf("bad id event %+v", ev)
	}
	resp.Body.Close()
	<-done

	// follow: tail lines first, then live lines; closing the client stops the handler (and the process).
	ctx, cancel := context.WithCancel(context.Background())
	req, _ := http.NewRequestWithContext(ctx, "GET", srv.URL+"?id=web&tail=2", nil)
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "text/event-stream" {
		t.Errorf("content type %q", ct)
	}
	ch := readSSE(resp.Body)
	var got []string
	for len(got) < 6 {
		ev := nextEvent(t, ch)
		if ev.Event != "" {
			t.Fatalf("unexpected event %+v", ev)
		}
		var ls []string
		if err := json.Unmarshal([]byte(ev.Data), &ls); err != nil {
			t.Fatal(err)
		}
		got = append(got, ls...)
	}
	if got[0] != "line 1" || got[1] != "line 2" || !strings.HasPrefix(got[len(got)-1], "tick ") {
		t.Errorf("stream lines %q", got)
	}
	cancel()
	resp.Body.Close()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("handler did not stop after the client went away")
	}

	// container exits: an end event closes the stream
	t.Setenv("FAKE_DOCKER_EXIT", "1")
	resp, err = http.Get(srv.URL + "?id=web&tail=1")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	ch = readSSE(resp.Body)
	var last sseEvent
	for ev := range ch {
		last = ev
	}
	if last.Event != "end" {
		t.Errorf("last event %+v", last)
	}
}

func TestDockerMCPTools(t *testing.T) {
	useFakeDocker(t)
	byName := map[string]tool{}
	for _, tl := range mcpTools() {
		byName[tl.Name] = tl
	}
	lc, ok := byName["list_containers"]
	if !ok {
		t.Fatal("list_containers not registered")
	}
	out, err := lc.handler(map[string]any{"all": true})
	if err != nil || !strings.Contains(out, "web") || !strings.Contains(out, "old") {
		t.Errorf("list_containers %v %q", err, out)
	}
	cl := byName["container_logs"]
	out, err = cl.handler(map[string]any{"name": "db", "tail": float64(2)})
	if err != nil || out != "line 2\nERROR boom\n" {
		t.Errorf("container_logs %v %q", err, out)
	}
	if _, err := cl.handler(map[string]any{"name": "--rm"}); err == nil {
		t.Error("container_logs accepted a flag as name")
	}
}
