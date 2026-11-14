package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

// Projects are folders the user registered on the Projects tab, stored in ~/.devdeck/projects.json.
type Project struct {
	Path string `json:"path"`
}

type projectView struct {
	Path    string `json:"path"`
	Name    string `json:"name"`
	Missing bool   `json:"missing,omitempty"`
	Tasks   []Task `json:"tasks"`
}

const maxProjects = 200

var projMu sync.Mutex // serialises read-modify-write of projects.json

func loadProjects() []Project {
	var ps []Project
	_ = loadJSON("projects", &ps)
	return ps
}

// cleanProjectPath validates user input: an absolute path (or ~/...) to an existing directory.
func cleanProjectPath(p string) (string, error) {
	p = strings.TrimSpace(strings.Trim(strings.TrimSpace(p), `"'`))
	if p == "" {
		return "", errors.New("enter a folder path")
	}
	if len(p) > 1024 || strings.ContainsRune(p, 0) {
		return "", errors.New("path is too long")
	}
	if p == "~" || strings.HasPrefix(p, "~/") || strings.HasPrefix(p, `~\`) {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		p = filepath.Join(home, p[1:])
	}
	if !filepath.IsAbs(p) {
		return "", errors.New("use an absolute path, e.g. " + exampleProjectPath())
	}
	p = filepath.Clean(p)
	st, err := os.Stat(p)
	if err != nil {
		return "", fmt.Errorf("folder not found: %s", p)
	}
	if !st.IsDir() {
		return "", fmt.Errorf("not a folder: %s", p)
	}
	return p, nil
}

func exampleProjectPath() string {
	if runtime.GOOS == "windows" {
		return `D:\code\my-app`
	}
	return "/home/you/code/my-app"
}

// samePath (case-insensitive on Windows/macOS) is defined in logs.go.

// pathInside reports whether child is dir or somewhere below it.
func pathInside(child, dir string) bool {
	if child == "" || dir == "" {
		return false
	}
	child, dir = filepath.Clean(child), filepath.Clean(dir)
	if samePath(child, dir) {
		return true
	}
	if !strings.HasSuffix(dir, string(filepath.Separator)) {
		dir += string(filepath.Separator)
	}
	return len(child) > len(dir) && samePath(child[:len(dir)], dir)
}

// cmdlineMentions reports whether a command line refers to a path inside dir
// (e.g. `node D:\app\node_modules\vite\bin\vite.js`).
func cmdlineMentions(cmdline, dir string) bool {
	if cmdline == "" || dir == "" {
		return false
	}
	lc, ld := cmdline, filepath.Clean(dir)
	if runtime.GOOS == "windows" || runtime.GOOS == "darwin" {
		lc, ld = strings.ToLower(lc), strings.ToLower(ld)
	}
	for i := 0; ; {
		j := strings.Index(lc[i:], ld)
		if j < 0 {
			return false
		}
		end := i + j + len(ld)
		if end == len(lc) || strings.ContainsRune(`/\"' `, rune(lc[end])) {
			return true
		}
		i = i + j + 1
	}
}

func projectViews() []projectView {
	ps := loadProjects()
	out := make([]projectView, 0, len(ps))
	for _, p := range ps {
		v := projectView{Path: p.Path, Name: filepath.Base(p.Path), Tasks: []Task{}}
		if isDir(p.Path) {
			v.Tasks = detectTasks(p.Path)
		} else {
			v.Missing = true
		}
		out = append(out, v)
	}
	return out
}

func isProject(p string) bool {
	for _, x := range loadProjects() {
		if samePath(x.Path, p) {
			return true
		}
	}
	return false
}

// ------------------------------------------------------------------ git status (small, read-only)

type projectGit struct {
	Repo   bool   `json:"repo"`
	Branch string `json:"branch,omitempty"`
	Dirty  int    `json:"dirty"` // changed + untracked files
}

func projectGitInfo(dir string) projectGit {
	run := func(maxOut int, args ...string) (string, error) {
		ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
		defer cancel()
		full := append([]string{"-C", dir, "--no-optional-locks", "-c", "core.fsmonitor=false", "-c", "core.quotepath=false"}, args...)
		cmd := exec.CommandContext(ctx, "git", full...)
		cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0", "GIT_OPTIONAL_LOCKS=0", "LC_ALL=C")
		procHideWindow(cmd)
		var out bytes.Buffer
		cmd.Stdout = &capWriter{w: &out, n: maxOut}
		err := cmd.Run()
		return out.String(), err
	}
	var g projectGit
	br, err := run(4096, "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return g
	}
	g.Repo, g.Branch = true, strings.TrimSpace(br)
	if g.Branch == "HEAD" {
		if h, err := run(4096, "rev-parse", "--short", "HEAD"); err == nil {
			g.Branch = "detached @ " + strings.TrimSpace(h)
		}
	}
	if st, err := run(256<<10, "status", "--porcelain"); err == nil {
		g.Dirty = strings.Count(st, "\n")
	}
	return g
}

type capWriter struct {
	w *bytes.Buffer
	n int
}

func (c *capWriter) Write(p []byte) (int, error) {
	if room := c.n - c.w.Len(); room > 0 {
		if len(p) > room {
			c.w.Write(p[:room])
		} else {
			c.w.Write(p)
		}
	}
	return len(p), nil
}

// ------------------------------------------------------------------ ports per project

type projectPorts struct {
	Path  string     `json:"path"`
	Ports []PortInfo `json:"ports"`
}

func matchProjectPorts(projects []Project, ports []PortInfo) []projectPorts {
	out := make([]projectPorts, 0, len(projects))
	for _, p := range projects {
		pp := projectPorts{Path: p.Path, Ports: []PortInfo{}}
		for _, x := range ports {
			if pathInside(x.Cwd, p.Path) || cmdlineMentions(x.Cmdline, p.Path) {
				pp.Ports = append(pp.Ports, x)
			}
		}
		out = append(out, pp)
	}
	return out
}

// ------------------------------------------------------------------ routes + MCP

func init() {
	addRoutes(func(mux *http.ServeMux) {
		mux.HandleFunc("GET /api/projects", func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, projectViews())
		})
		mux.HandleFunc("POST /api/projects", func(w http.ResponseWriter, r *http.Request) {
			var b struct {
				Path string `json:"path"`
			}
			if err := decode(r, &b); err != nil {
				fail(w, 400, err)
				return
			}
			p, err := cleanProjectPath(b.Path)
			if err != nil {
				fail(w, 400, err)
				return
			}
			projMu.Lock()
			defer projMu.Unlock()
			ps := loadProjects()
			for _, x := range ps {
				if samePath(x.Path, p) {
					fail(w, 409, fmt.Errorf("already added: %s", x.Path))
					return
				}
			}
			if len(ps) >= maxProjects {
				fail(w, 400, fmt.Errorf("at most %d projects", maxProjects))
				return
			}
			if err := saveJSON("projects", append(ps, Project{Path: p})); err != nil {
				fail(w, 500, err)
				return
			}
			writeJSON(w, map[string]string{"path": p})
		})
		mux.HandleFunc("POST /api/projects/remove", func(w http.ResponseWriter, r *http.Request) {
			var b struct {
				Path string `json:"path"`
			}
			if err := decode(r, &b); err != nil {
				fail(w, 400, err)
				return
			}
			projMu.Lock()
			defer projMu.Unlock()
			ps := loadProjects()
			kept := ps[:0]
			for _, x := range ps {
				if !samePath(x.Path, b.Path) {
					kept = append(kept, x)
				}
			}
			if err := saveJSON("projects", kept); err != nil {
				fail(w, 500, err)
				return
			}
			writeJSON(w, map[string]bool{"ok": true})
		})
		// Branch + dirty count; only for registered projects.
		mux.HandleFunc("GET /api/projects/git", func(w http.ResponseWriter, r *http.Request) {
			p := r.URL.Query().Get("path")
			if !isProject(p) || !isDir(p) {
				fail(w, 404, errors.New("unknown project"))
				return
			}
			writeJSON(w, projectGitInfo(p))
		})
		// Listening ports whose process runs inside a project (cwd or command line).
		mux.HandleFunc("GET /api/projects/ports", func(w http.ResponseWriter, r *http.Request) {
			ports, err := listPorts()
			if err != nil {
				fail(w, 500, err)
				return
			}
			writeJSON(w, matchProjectPorts(loadProjects(), ports))
		})
	})

	registerMCPTool(tool{Name: "list_projects", Description: "Project folders registered in the devdeck Projects tab, with detected tasks (npm/pnpm/yarn/bun scripts, make, just, dotnet, go, cargo, python, docker compose).",
		InputSchema: obj(map[string]any{}),
		handler: func(map[string]any) (string, error) {
			vs := projectViews()
			if len(vs) == 0 {
				return "no projects registered (add folders on the devdeck Projects tab)", nil
			}
			var sb strings.Builder
			for _, v := range vs {
				fmt.Fprintf(&sb, "%s\t%s", v.Name, v.Path)
				if v.Missing {
					sb.WriteString("\t(missing)")
				}
				sb.WriteByte('\n')
				for _, t := range v.Tasks {
					fmt.Fprintf(&sb, "  [%s] %s: %s\n", t.Source, t.Name, t.Cmd)
				}
			}
			return sb.String(), nil
		}})
}
