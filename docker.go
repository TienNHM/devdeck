package main

// Docker tab backend. Everything shells out to the docker CLI (env DEVDECK_DOCKER_BIN, default "docker")
// with an argument slice (never a shell), strictly validated ids, timeouts and capped output.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var (
	// Container ids / names: docker allows [a-zA-Z0-9][a-zA-Z0-9_.-]+. Never a leading '-'.
	dockerIDRe = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.-]{0,127}$`)
	// Image references (id, sha256:id, repo[:tag][@digest]).
	dockerImageRe = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._/:@-]{0,254}$`)
	// Compose project names: lowercase letters, digits, dashes and underscores.
	composeNameRe = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,62}$`)
	daemonDownRe  = regexp.MustCompile(`(?i)cannot connect to the docker daemon|failed to connect to the docker api|error during connect|is the docker daemon running|docker desktop is (not running|unable to start)|pipe/docker[a-z_]*engine`)
	ansiRe        = regexp.MustCompile(`\x1b\[[0-9;?]*[ -/]*[@-~]|\x1b\][^\x07\x1b]*(\x07|\x1b\\)`)
)

func dockerBin() string {
	if b := strings.TrimSpace(os.Getenv("DEVDECK_DOCKER_BIN")); b != "" {
		return b
	}
	return "docker"
}

func dockerEnv() []string {
	return append(os.Environ(), "DOCKER_CLI_HINTS=false", "NO_COLOR=1", "COMPOSE_ANSI=never", "COMPOSE_STATUS_STDOUT=1")
}

// dockerError carries a state the UI can show as an empty state: "missing" (no CLI), "down" (no daemon) or "error".
type dockerError struct {
	State string `json:"state"`
	Msg   string `json:"error"`
}

func (e *dockerError) Error() string { return e.Msg }

func classifyDocker(ctx context.Context, err error, stderr string, args []string) error {
	if err == nil {
		return nil
	}
	var pe *fs.PathError
	if errors.Is(err, exec.ErrNotFound) || (errors.As(err, &pe) && errors.Is(pe.Err, fs.ErrNotExist)) {
		return &dockerError{"missing", "docker CLI not found (" + dockerBin() + "). Install Docker or set DEVDECK_DOCKER_BIN."}
	}
	stderr = strings.TrimSpace(ansiRe.ReplaceAllString(stderr, ""))
	if daemonDownRe.MatchString(stderr) {
		return &dockerError{"down", "Docker daemon is not running. " + firstLine(stderr)}
	}
	if ctx != nil && errors.Is(ctx.Err(), context.DeadlineExceeded) {
		name := "command"
		if len(args) > 0 {
			name = args[0]
		}
		return &dockerError{"error", "docker " + name + " timed out"}
	}
	if stderr == "" {
		stderr = err.Error()
	}
	return &dockerError{"error", stderr}
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return strings.TrimSpace(s[:i])
	}
	return s
}

type dockerCap struct {
	b   []byte
	max int
	cut bool
}

func (c *dockerCap) Write(p []byte) (int, error) {
	if room := c.max - len(c.b); room > 0 {
		if len(p) > room {
			c.b = append(c.b, p[:room]...)
			c.cut = true
		} else {
			c.b = append(c.b, p...)
		}
	} else if len(p) > 0 {
		c.cut = true
	}
	return len(p), nil
}

// runDocker runs the docker CLI once. Stdout is capped at maxOut bytes.
func runDocker(parent context.Context, timeout time.Duration, maxOut int, dir string, args ...string) ([]byte, bool, error) {
	if parent == nil {
		parent = context.Background()
	}
	ctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, dockerBin(), args...)
	cmd.Env = dockerEnv()
	cmd.Dir = dir
	cmd.WaitDelay = 2 * time.Second
	hideWindow(cmd)
	out := &dockerCap{max: maxOut}
	errb := &dockerCap{max: 64 << 10}
	cmd.Stdout, cmd.Stderr = out, errb
	err := cmd.Run()
	if err != nil {
		return out.b, out.cut, classifyDocker(ctx, err, string(errb.b), args)
	}
	return out.b, out.cut, nil
}

// jsonLines parses `--format '{{json .}}'` output (one object per line).
func jsonLines[T any](b []byte) ([]T, error) {
	var res []T
	for _, ln := range strings.Split(string(b), "\n") {
		ln = strings.TrimSpace(ln)
		if ln == "" {
			continue
		}
		var v T
		if err := json.Unmarshal([]byte(ln), &v); err != nil {
			return nil, fmt.Errorf("unexpected docker output: %s", truncStr(ln, 200))
		}
		res = append(res, v)
	}
	return res, nil
}

func truncStr(s string, n int) string {
	if len(s) > n {
		return s[:n] + "…"
	}
	return s
}

// ------------------------------------------------------------------ status

type dockerStatus struct {
	State   string `json:"state"` // ok | missing | down | error
	Client  string `json:"client,omitempty"`
	Server  string `json:"server,omitempty"`
	Message string `json:"error,omitempty"`
}

func dockerStatusCheck(ctx context.Context) dockerStatus {
	out, _, err := runDocker(ctx, 10*time.Second, 1<<20, "", "version", "--format", "{{json .}}")
	var v struct {
		Client *struct{ Version string }
		Server *struct{ Version string }
	}
	_ = json.Unmarshal([]byte(strings.TrimSpace(string(out))), &v)
	st := dockerStatus{State: "ok"}
	if v.Client != nil {
		st.Client = v.Client.Version
	}
	if v.Server != nil {
		st.Server = v.Server.Version
	}
	if err != nil {
		var de *dockerError
		if errors.As(err, &de) {
			st.State, st.Message = de.State, de.Msg
		} else {
			st.State, st.Message = "error", err.Error()
		}
		if st.State == "error" && v.Client != nil && v.Server == nil {
			st.State = "down"
		}
		return st
	}
	if v.Server == nil {
		st.State, st.Message = "down", "Docker daemon is not running."
	}
	return st
}

// ------------------------------------------------------------------ containers

type dockerPort struct {
	IP        string `json:"ip,omitempty"`
	Host      int    `json:"host,omitempty"`
	Container int    `json:"container"`
	Proto     string `json:"proto"`
}

type dockerContainer struct {
	ID         string       `json:"id"`
	Name       string       `json:"name"`
	Image      string       `json:"image"`
	State      string       `json:"state"`
	Status     string       `json:"status"`
	RunningFor string       `json:"runningFor"`
	Command    string       `json:"command"`
	CreatedAt  string       `json:"createdAt"`
	Ports      []dockerPort `json:"ports"`
	PortsRaw   string       `json:"portsRaw"`
	Compose    string       `json:"compose,omitempty"`
}

type rawContainer struct {
	ID, Names, Image, State, Status, RunningFor, Command, CreatedAt, Ports, Labels string
}

// parsePorts turns "0.0.0.0:8080->80/tcp, [::]:8080->80/tcp, 9000/udp" into unique entries.
func parsePorts(s string) []dockerPort {
	res := []dockerPort{}
	seen := map[string]bool{}
	for _, item := range strings.Split(s, ",") {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		host, ctr, published := strings.Cut(item, "->")
		if !published {
			ctr, host = item, ""
		}
		cport, proto, _ := strings.Cut(ctr, "/")
		if proto == "" {
			proto = "tcp"
		}
		cfrom, cto := portRange(cport)
		if !published {
			for p := cfrom; p <= cto && p-cfrom < 32; p++ {
				k := fmt.Sprintf("-:%d/%s", p, proto)
				if !seen[k] {
					seen[k] = true
					res = append(res, dockerPort{Container: p, Proto: proto})
				}
			}
			continue
		}
		i := strings.LastIndex(host, ":")
		if i < 0 {
			continue
		}
		ip, hport := strings.Trim(host[:i], "[]"), host[i+1:]
		hfrom, hto := portRange(hport)
		if hfrom == 0 {
			continue
		}
		for n := 0; hfrom+n <= hto && n < 32; n++ {
			k := fmt.Sprintf("%d/%s", hfrom+n, proto)
			if seen[k] {
				continue
			}
			seen[k] = true
			c := cfrom + n
			if c > cto {
				c = cto
			}
			res = append(res, dockerPort{IP: ip, Host: hfrom + n, Container: c, Proto: proto})
		}
	}
	return res
}

func portRange(s string) (int, int) {
	a, b, isRange := strings.Cut(s, "-")
	from, err := strconv.Atoi(a)
	if err != nil || from < 0 || from > 65535 {
		return 0, -1
	}
	if !isRange {
		return from, from
	}
	to, err := strconv.Atoi(b)
	if err != nil || to < from || to > 65535 {
		return from, from
	}
	return from, to
}

func labelValue(labels, key string) string {
	for _, kv := range strings.Split(labels, ",") {
		if k, v, ok := strings.Cut(kv, "="); ok && k == key {
			return v
		}
	}
	return ""
}

func dockerContainers(ctx context.Context, all bool) ([]dockerContainer, error) {
	args := []string{"ps", "--format", "{{json .}}"}
	if all {
		args = append(args, "-a")
	}
	out, _, err := runDocker(ctx, 20*time.Second, 8<<20, "", args...)
	if err != nil {
		return nil, err
	}
	raws, err := jsonLines[rawContainer](out)
	if err != nil {
		return nil, err
	}
	res := make([]dockerContainer, 0, len(raws))
	for _, r := range raws {
		name := r.Names
		if i := strings.IndexByte(name, ','); i >= 0 {
			name = name[:i]
		}
		res = append(res, dockerContainer{
			ID: r.ID, Name: name, Image: r.Image, State: strings.ToLower(r.State), Status: r.Status, RunningFor: r.RunningFor,
			Command: strings.Trim(r.Command, `"`), CreatedAt: r.CreatedAt, Ports: parsePorts(r.Ports), PortsRaw: r.Ports,
			Compose: labelValue(r.Labels, "com.docker.compose.project"),
		})
	}
	return res, nil
}

func validContainer(id string) error {
	if !dockerIDRe.MatchString(id) {
		return errors.New("invalid container id or name")
	}
	return nil
}

func dockerContainerAction(ctx context.Context, id, action string, force bool) error {
	if err := validContainer(id); err != nil {
		return err
	}
	var args []string
	switch action {
	case "start", "stop", "restart", "pause", "unpause":
		args = []string{action, id}
	case "remove":
		args = []string{"rm"}
		if force {
			args = append(args, "-f")
		}
		args = append(args, id)
	default:
		return errors.New("unknown action")
	}
	_, _, err := runDocker(ctx, 90*time.Second, 64<<10, "", args...)
	return err
}

func dockerLogsOnce(ctx context.Context, id string, tail int) ([]string, error) {
	if err := validContainer(id); err != nil {
		return nil, err
	}
	tail = clampInt(tail, 1, 5000, 200)
	// docker logs writes the container's stderr to its own stderr, so merge both through one capped buffer.
	cctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	cmd := exec.CommandContext(cctx, dockerBin(), "logs", "--tail", strconv.Itoa(tail), id)
	cmd.Env = dockerEnv()
	cmd.WaitDelay = 2 * time.Second
	hideWindow(cmd)
	buf := &dockerCap{max: 4 << 20}
	cmd.Stdout, cmd.Stderr = buf, buf
	if err := cmd.Run(); err != nil {
		return nil, classifyDocker(cctx, err, string(buf.b), []string{"logs"})
	}
	lines := splitLogLines(string(buf.b))
	if len(lines) > tail {
		lines = lines[len(lines)-tail:]
	}
	return lines, nil
}

func splitLogLines(s string) []string {
	s = strings.TrimRight(ansiRe.ReplaceAllString(s, ""), "\r\n")
	if s == "" {
		return []string{}
	}
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		lines[i] = strings.TrimRight(l, "\r")
	}
	return lines
}

func clampInt(v, lo, hi, def int) int {
	if v <= 0 {
		return def
	}
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

type dockerStat struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	CPUPerc  string `json:"cpu"`
	MemUsage string `json:"mem"`
	MemPerc  string `json:"memPerc"`
	NetIO    string `json:"net"`
	BlockIO  string `json:"block"`
	PIDs     string `json:"pids"`
}

func dockerStats(ctx context.Context, id string) ([]dockerStat, error) {
	args := []string{"stats", "--no-stream", "--format", "{{json .}}"}
	if id != "" {
		if err := validContainer(id); err != nil {
			return nil, err
		}
		args = append(args, id)
	}
	out, _, err := runDocker(ctx, 40*time.Second, 4<<20, "", args...)
	if err != nil {
		return nil, err
	}
	raws, err := jsonLines[map[string]any](out)
	if err != nil {
		return nil, err
	}
	s := func(m map[string]any, k string) string { v, _ := m[k].(string); return v }
	res := make([]dockerStat, 0, len(raws))
	for _, m := range raws {
		res = append(res, dockerStat{ID: s(m, "ID"), Name: s(m, "Name"), CPUPerc: s(m, "CPUPerc"), MemUsage: s(m, "MemUsage"),
			MemPerc: s(m, "MemPerc"), NetIO: s(m, "NetIO"), BlockIO: s(m, "BlockIO"), PIDs: s(m, "PIDs")})
	}
	return res, nil
}

func dockerInspect(ctx context.Context, id string) (json.RawMessage, error) {
	if err := validContainer(id); err != nil {
		return nil, err
	}
	out, cut, err := runDocker(ctx, 20*time.Second, 4<<20, "", "inspect", "--type", "container", id)
	if err != nil {
		return nil, err
	}
	if cut {
		return nil, errors.New("inspect output too large")
	}
	var arr []json.RawMessage
	if err := json.Unmarshal(out, &arr); err != nil || len(arr) == 0 {
		return nil, errors.New("unexpected docker inspect output")
	}
	return arr[0], nil
}

// ------------------------------------------------------------------ images

type dockerImage struct {
	ID         string `json:"id"`
	Repository string `json:"repository"`
	Tag        string `json:"tag"`
	Size       string `json:"size"`
	Created    string `json:"created"`
	Containers string `json:"containers"`
}

func dockerImages(ctx context.Context) ([]dockerImage, error) {
	out, _, err := runDocker(ctx, 30*time.Second, 8<<20, "", "images", "--format", "{{json .}}")
	if err != nil {
		return nil, err
	}
	raws, err := jsonLines[map[string]any](out)
	if err != nil {
		return nil, err
	}
	s := func(m map[string]any, k string) string {
		switch v := m[k].(type) {
		case string:
			return v
		case float64:
			return strconv.FormatFloat(v, 'f', -1, 64)
		}
		return ""
	}
	res := make([]dockerImage, 0, len(raws))
	for _, m := range raws {
		res = append(res, dockerImage{ID: s(m, "ID"), Repository: s(m, "Repository"), Tag: s(m, "Tag"), Size: s(m, "Size"),
			Created: s(m, "CreatedSince"), Containers: s(m, "Containers")})
	}
	return res, nil
}

func dockerRemoveImage(ctx context.Context, ref string, force bool) error {
	if !dockerImageRe.MatchString(ref) {
		return errors.New("invalid image reference")
	}
	args := []string{"rmi"}
	if force {
		args = append(args, "-f")
	}
	_, _, err := runDocker(ctx, 120*time.Second, 64<<10, "", append(args, ref)...)
	return err
}

// ------------------------------------------------------------------ compose

type composeProject struct {
	Name        string   `json:"name"`
	Status      string   `json:"status"`
	ConfigFiles []string `json:"configFiles"`
}

func composeProjects(ctx context.Context) ([]composeProject, error) {
	out, _, err := runDocker(ctx, 30*time.Second, 4<<20, "", "compose", "ls", "-a", "--format", "json")
	if err != nil {
		return nil, err
	}
	var raws []struct{ Name, Status, ConfigFiles string }
	if s := strings.TrimSpace(string(out)); s != "" && s != "null" {
		if err := json.Unmarshal([]byte(s), &raws); err != nil {
			return nil, errors.New("unexpected docker compose ls output")
		}
	}
	res := make([]composeProject, 0, len(raws))
	for _, r := range raws {
		p := composeProject{Name: r.Name, Status: r.Status, ConfigFiles: []string{}}
		for _, f := range strings.Split(r.ConfigFiles, ",") {
			if f = strings.TrimSpace(f); f != "" {
				p.ConfigFiles = append(p.ConfigFiles, f)
			}
		}
		res = append(res, p)
	}
	return res, nil
}

// composeAction runs up -d / down / restart for a project known to `docker compose ls`.
// Config files come from docker itself, never from the client.
func composeAction(ctx context.Context, name, action string) (string, error) {
	if !composeNameRe.MatchString(name) {
		return "", errors.New("invalid compose project name")
	}
	var tail []string
	timeout := 3 * time.Minute
	switch action {
	case "up":
		tail, timeout = []string{"up", "-d"}, 10*time.Minute
	case "down":
		tail = []string{"down"}
	case "restart":
		tail = []string{"restart"}
	default:
		return "", errors.New("unknown compose action")
	}
	projects, err := composeProjects(ctx)
	if err != nil {
		return "", err
	}
	var proj *composeProject
	for i := range projects {
		if projects[i].Name == name {
			proj = &projects[i]
		}
	}
	if proj == nil {
		return "", errors.New("unknown compose project: " + name)
	}
	args := []string{"compose", "--ansi", "never", "-p", name}
	dir := ""
	filesOK := len(proj.ConfigFiles) > 0
	for _, f := range proj.ConfigFiles {
		if st, err := os.Stat(f); err != nil || !st.Mode().IsRegular() {
			filesOK = false
		}
	}
	if filesOK {
		for _, f := range proj.ConfigFiles {
			args = append(args, "-f", f)
		}
		dir = filepath.Dir(proj.ConfigFiles[0])
	} else if action == "up" {
		return "", errors.New("compose file not found for project " + name)
	}
	cctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.CommandContext(cctx, dockerBin(), append(args, tail...)...)
	cmd.Env = dockerEnv()
	cmd.Dir = dir
	cmd.WaitDelay = 2 * time.Second
	hideWindow(cmd)
	buf := &dockerCap{max: 256 << 10}
	cmd.Stdout, cmd.Stderr = buf, buf
	err = cmd.Run()
	text := strings.TrimSpace(ansiRe.ReplaceAllString(string(buf.b), ""))
	if err != nil {
		return text, classifyDocker(cctx, err, text, []string{"compose " + action})
	}
	return text, nil
}

// ------------------------------------------------------------------ HTTP

func dockerFail(w http.ResponseWriter, err error) {
	var de *dockerError
	if errors.As(err, &de) {
		code := 500
		if de.State == "missing" || de.State == "down" {
			code = 503
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(code)
		_ = json.NewEncoder(w).Encode(de)
		return
	}
	fail(w, 400, err)
}

func init() {
	addRoutes(func(mux *http.ServeMux) {
		mux.HandleFunc("GET /api/docker/status", func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, dockerStatusCheck(r.Context()))
		})
		mux.HandleFunc("GET /api/docker/containers", func(w http.ResponseWriter, r *http.Request) {
			cs, err := dockerContainers(r.Context(), r.URL.Query().Get("all") == "1")
			if err != nil {
				dockerFail(w, err)
				return
			}
			writeJSON(w, cs)
		})
		mux.HandleFunc("POST /api/docker/container", func(w http.ResponseWriter, r *http.Request) {
			var b struct {
				ID     string `json:"id"`
				Action string `json:"action"`
				Force  bool   `json:"force"`
			}
			if err := decode(r, &b); err != nil {
				fail(w, 400, err)
				return
			}
			if err := dockerContainerAction(r.Context(), b.ID, b.Action, b.Force); err != nil {
				dockerFail(w, err)
				return
			}
			writeJSON(w, map[string]bool{"ok": true})
		})
		mux.HandleFunc("GET /api/docker/logs", func(w http.ResponseWriter, r *http.Request) {
			q := r.URL.Query()
			tail, _ := strconv.Atoi(q.Get("tail"))
			if q.Get("follow") == "1" {
				dockerLogsStream(w, r, q.Get("id"), tail)
				return
			}
			lines, err := dockerLogsOnce(r.Context(), q.Get("id"), tail)
			if err != nil {
				dockerFail(w, err)
				return
			}
			writeJSON(w, map[string]any{"lines": lines})
		})
		mux.HandleFunc("GET /api/docker/stats", func(w http.ResponseWriter, r *http.Request) {
			st, err := dockerStats(r.Context(), r.URL.Query().Get("id"))
			if err != nil {
				dockerFail(w, err)
				return
			}
			writeJSON(w, st)
		})
		mux.HandleFunc("GET /api/docker/inspect", func(w http.ResponseWriter, r *http.Request) {
			v, err := dockerInspect(r.Context(), r.URL.Query().Get("id"))
			if err != nil {
				dockerFail(w, err)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("Cache-Control", "no-store")
			_, _ = w.Write(v)
		})
		mux.HandleFunc("GET /api/docker/images", func(w http.ResponseWriter, r *http.Request) {
			im, err := dockerImages(r.Context())
			if err != nil {
				dockerFail(w, err)
				return
			}
			writeJSON(w, im)
		})
		mux.HandleFunc("POST /api/docker/image/remove", func(w http.ResponseWriter, r *http.Request) {
			var b struct {
				ID    string `json:"id"`
				Force bool   `json:"force"`
			}
			if err := decode(r, &b); err != nil {
				fail(w, 400, err)
				return
			}
			if err := dockerRemoveImage(r.Context(), b.ID, b.Force); err != nil {
				dockerFail(w, err)
				return
			}
			writeJSON(w, map[string]bool{"ok": true})
		})
		mux.HandleFunc("GET /api/docker/compose", func(w http.ResponseWriter, r *http.Request) {
			ps, err := composeProjects(r.Context())
			if err != nil {
				dockerFail(w, err)
				return
			}
			writeJSON(w, ps)
		})
		mux.HandleFunc("POST /api/docker/compose", func(w http.ResponseWriter, r *http.Request) {
			var b struct {
				Name   string `json:"name"`
				Action string `json:"action"`
			}
			if err := decode(r, &b); err != nil {
				fail(w, 400, err)
				return
			}
			out, err := composeAction(r.Context(), b.Name, b.Action)
			if err != nil {
				var de *dockerError
				if errors.As(err, &de) {
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(500)
					_ = json.NewEncoder(w).Encode(map[string]string{"error": de.Msg, "state": de.State, "output": out})
					return
				}
				fail(w, 400, err)
				return
			}
			writeJSON(w, map[string]any{"ok": true, "output": out})
		})
	})

	registerMCPTool(tool{Name: "list_containers", Description: "List Docker containers (running only unless all=true) with state, image and published ports.",
		InputSchema: obj(map[string]any{"all": typ("boolean", "include stopped containers")}),
		handler: func(a map[string]any) (string, error) {
			all, _ := a["all"].(bool)
			cs, err := dockerContainers(context.Background(), all)
			if err != nil {
				return "", err
			}
			if len(cs) == 0 {
				return "no containers\n", nil
			}
			var sb strings.Builder
			for _, c := range cs {
				fmt.Fprintf(&sb, "%s\t%s\t%s\t%s\t%s\t%s\n", c.ID, c.Name, c.State, c.Status, c.Image, c.PortsRaw)
			}
			return sb.String(), nil
		}})
	registerMCPTool(tool{Name: "container_logs", Description: "Last lines of a Docker container's logs (stdout and stderr merged, max 2000 lines).",
		InputSchema: obj(map[string]any{"name": typ("string", "container name or id"), "tail": typ("integer", "number of lines, default 100, max 2000")}, "name"),
		handler: func(a map[string]any) (string, error) {
			lines, err := dockerLogsOnce(context.Background(), argStr(a, "name", ""), clampInt(argInt(a, "tail", 100), 1, 2000, 100))
			if err != nil {
				return "", err
			}
			return strings.Join(lines, "\n") + "\n", nil
		}})
}
