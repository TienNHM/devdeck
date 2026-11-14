package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// Task is a command detected in a project folder. Cmd is typed into a devdeck terminal opened in
// the project folder by an explicit user click; names and paths that are not plain identifiers are
// quoted or skipped so a file in the project cannot smuggle shell syntax into it.
type Task struct {
	Name   string `json:"name"`
	Cmd    string `json:"cmd"`
	Source string `json:"source"`         // npm, pnpm, yarn, bun, deno, make, just, dotnet, go, cargo, python, compose
	Body   string `json:"body,omitempty"` // script text (package.json), used to link tasks to listening ports
}

const (
	maxTasksPerSource = 40
	maxManifestBytes  = 1 << 20
)

var (
	safeNameRe  = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9:._@/+-]{0,79}$`)
	plainArgRe  = regexp.MustCompile(`^[A-Za-z0-9._/\\:+-]+$`)
	quotableRe  = regexp.MustCompile(`^[^'"$` + "`" + `\r\n;&|<>%!^]+$`)
	exeOutputRe = regexp.MustCompile(`(?i)<OutputType>\s*(Win)?Exe\s*</OutputType>`)
	mainPkgRe   = regexp.MustCompile(`(?m)^package main\b`)
	asgiAppRe   = regexp.MustCompile(`(?m)^([A-Za-z_][A-Za-z0-9_]*)\s*=\s*(FastAPI|Starlette)\(`)
)

// shellArg returns s ready to append to a command line (bash and PowerShell agree on '...'),
// or "" when it cannot be represented safely.
func shellArg(s string) string {
	if plainArgRe.MatchString(s) {
		return s
	}
	if quotableRe.MatchString(s) {
		return "'" + s + "'"
	}
	return ""
}

func readCapped(path string, n int64) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return io.ReadAll(io.LimitReader(f, n))
}

func exists(dir string, names ...string) string {
	for _, n := range names {
		if st, err := os.Stat(filepath.Join(dir, n)); err == nil && !st.IsDir() {
			return n
		}
	}
	return ""
}

func isDir(p string) bool {
	st, err := os.Stat(p)
	return err == nil && st.IsDir()
}

// detectTasks inspects well-known manifest files directly in dir (and a few levels down for .NET).
func detectTasks(dir string) []Task {
	var out []Task
	for _, f := range []func(string) []Task{taskNode, taskDeno, taskMake, taskJust, taskDotnet, taskGo, taskCargo, taskPython, taskCompose} {
		ts := f(dir)
		if len(ts) > maxTasksPerSource {
			ts = ts[:maxTasksPerSource]
		}
		out = append(out, ts...)
	}
	return out
}

// ------------------------------------------------------------------ package.json

var scriptPriority = map[string]int{"dev": 0, "start": 1, "serve": 2, "watch": 3, "build": 4, "test": 5, "lint": 6, "format": 7, "preview": 8}

func nodePM(dir string, pkgPM string) string {
	if pkgPM != "" { // "packageManager": "pnpm@9.1.0"
		name, _, _ := strings.Cut(pkgPM, "@")
		switch name {
		case "npm", "pnpm", "yarn", "bun":
			return name
		}
	}
	switch {
	case exists(dir, "pnpm-lock.yaml") != "":
		return "pnpm"
	case exists(dir, "yarn.lock") != "":
		return "yarn"
	case exists(dir, "bun.lockb", "bun.lock") != "":
		return "bun"
	}
	return "npm"
}

func taskNode(dir string) []Task {
	b, err := readCapped(filepath.Join(dir, "package.json"), maxManifestBytes)
	if err != nil {
		return nil
	}
	var pkg struct {
		Scripts        map[string]any `json:"scripts"`
		PackageManager string         `json:"packageManager"`
	}
	if json.Unmarshal(b, &pkg) != nil {
		return nil
	}
	pm := nodePM(dir, pkg.PackageManager)
	names := make([]string, 0, len(pkg.Scripts))
	for k := range pkg.Scripts {
		if safeNameRe.MatchString(k) && !strings.HasPrefix(k, "pre") && !strings.HasPrefix(k, "post") {
			names = append(names, k)
		}
	}
	sort.Slice(names, func(i, j int) bool {
		pi, iok := scriptPriority[names[i]]
		pj, jok := scriptPriority[names[j]]
		if iok != jok {
			return iok
		}
		if iok && pi != pj {
			return pi < pj
		}
		return names[i] < names[j]
	})
	var out []Task
	for _, k := range names {
		body, _ := pkg.Scripts[k].(string)
		out = append(out, Task{Name: k, Cmd: pm + " run " + k, Source: pm, Body: procClip(body, 300)})
	}
	if len(out) > maxTasksPerSource-1 {
		out = out[:maxTasksPerSource-1]
	}
	return append(out, Task{Name: "install", Cmd: pm + " install", Source: pm})
}

// ------------------------------------------------------------------ deno.json

func taskDeno(dir string) []Task {
	b, err := readCapped(filepath.Join(dir, "deno.json"), maxManifestBytes)
	if err != nil {
		return nil
	}
	var cfg struct {
		Tasks map[string]any `json:"tasks"`
	}
	if json.Unmarshal(b, &cfg) != nil {
		return nil
	}
	var out []Task
	for k := range cfg.Tasks {
		if safeNameRe.MatchString(k) {
			out = append(out, Task{Name: k, Cmd: "deno task " + k, Source: "deno"})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// ------------------------------------------------------------------ Makefile

var makeTargetRe = regexp.MustCompile(`^([A-Za-z0-9][A-Za-z0-9._/ -]*?)\s*::?(?:[^=]|$)`)

func taskMake(dir string) []Task {
	name := exists(dir, "GNUmakefile", "makefile", "Makefile")
	if name == "" {
		return nil
	}
	b, err := readCapped(filepath.Join(dir, name), 512<<10)
	if err != nil {
		return nil
	}
	seen := map[string]bool{}
	var out []Task
	sc := bufio.NewScanner(bytes.NewReader(b))
	for sc.Scan() {
		line := sc.Text()
		if line == "" || line[0] == '\t' || line[0] == ' ' || line[0] == '#' {
			continue
		}
		m := makeTargetRe.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		for _, t := range strings.Fields(m[1]) {
			if seen[t] || strings.ContainsAny(t, "%$") || !safeNameRe.MatchString(t) {
				continue
			}
			seen[t] = true
			out = append(out, Task{Name: t, Cmd: "make " + t, Source: "make"})
		}
	}
	return out
}

// ------------------------------------------------------------------ justfile

var justRecipeRe = regexp.MustCompile(`^@?([A-Za-z_][A-Za-z0-9_-]*)((?:\s+[^:]*)?)\s*:(?:[^=]|$)`)

func taskJust(dir string) []Task {
	name := exists(dir, "justfile", "Justfile", ".justfile")
	if name == "" {
		return nil
	}
	b, err := readCapped(filepath.Join(dir, name), 512<<10)
	if err != nil {
		return nil
	}
	var out []Task
	private := false
	sc := bufio.NewScanner(bytes.NewReader(b))
	for sc.Scan() {
		line := sc.Text()
		if line == "" || line[0] == ' ' || line[0] == '\t' || line[0] == '#' {
			continue
		}
		if line[0] == '[' { // attributes apply to the next recipe
			if strings.Contains(line, "private") {
				private = true
			}
			continue
		}
		m := justRecipeRe.FindStringSubmatch(line)
		wasPrivate := private
		private = false
		if m == nil || wasPrivate || strings.HasPrefix(m[1], "_") {
			continue
		}
		switch m[1] {
		case "set", "alias", "export", "import", "mod":
			continue
		}
		required := false // parameters without a default would need arguments
		for _, p := range strings.Fields(m[2]) {
			if !strings.Contains(p, "=") && !strings.HasPrefix(p, "*") {
				required = true
			}
		}
		if !required {
			out = append(out, Task{Name: m[1], Cmd: "just " + m[1], Source: "just"})
		}
	}
	return out
}

// ------------------------------------------------------------------ .NET

var skipDirs = map[string]bool{"node_modules": true, "bin": true, "obj": true, ".git": true, ".vs": true, "packages": true, "dist": true, "target": true, ".idea": true, "vendor": true}

// findFiles walks dir up to depth levels below it and returns paths (relative) matching ext.
func findFiles(dir string, depth, limit int, exts ...string) []string {
	var out []string
	var walk func(rel string, d int)
	walk = func(rel string, d int) {
		ents, err := os.ReadDir(filepath.Join(dir, rel))
		if err != nil {
			return
		}
		for _, e := range ents {
			if len(out) >= limit {
				return
			}
			n := e.Name()
			if e.IsDir() {
				if d < depth && !skipDirs[strings.ToLower(n)] && !strings.HasPrefix(n, ".") {
					walk(filepath.Join(rel, n), d+1)
				}
				continue
			}
			for _, x := range exts {
				if strings.EqualFold(filepath.Ext(n), x) {
					out = append(out, filepath.Join(rel, n))
				}
			}
		}
	}
	walk("", 0)
	sort.Strings(out)
	return out
}

func taskDotnet(dir string) []Task {
	slns := findFiles(dir, 0, 4, ".sln", ".slnx")
	projs := findFiles(dir, 3, 40, ".csproj", ".fsproj", ".vbproj")
	if len(slns) == 0 && len(projs) == 0 {
		return nil
	}
	var out []Task
	for _, s := range slns {
		if a := shellArg(s); a != "" {
			out = append(out,
				Task{Name: "build " + s, Cmd: "dotnet build " + a, Source: "dotnet"},
				Task{Name: "test " + s, Cmd: "dotnet test " + a, Source: "dotnet"})
		}
	}
	runnable := 0
	for _, p := range projs {
		a := shellArg(p)
		if a == "" {
			continue
		}
		b, _ := readCapped(filepath.Join(dir, p), 256<<10)
		s := string(b)
		base := strings.TrimSuffix(filepath.Base(p), filepath.Ext(p))
		isTest := strings.Contains(s, "Microsoft.NET.Test.Sdk") || strings.HasSuffix(base, ".Tests") || strings.HasSuffix(base, ".Test")
		isRun := !isTest && (strings.Contains(s, "Microsoft.NET.Sdk.Web") || strings.Contains(s, "Microsoft.NET.Sdk.Worker") ||
			strings.Contains(s, "Microsoft.NET.Sdk.BlazorWebAssembly") || strings.Contains(s, "Microsoft.NET.Sdk.Razor") ||
			strings.Contains(s, "Aspire.AppHost") || exeOutputRe.MatchString(s))
		if isRun && runnable < 8 {
			runnable++
			out = append(out,
				Task{Name: "run " + base, Cmd: "dotnet run --project " + a, Source: "dotnet"},
				Task{Name: "watch " + base, Cmd: "dotnet watch --project " + a + " run", Source: "dotnet"})
		}
		if isTest && len(slns) == 0 {
			out = append(out, Task{Name: "test " + base, Cmd: "dotnet test " + a, Source: "dotnet"})
		}
	}
	if len(slns) == 0 {
		out = append(out, Task{Name: "build", Cmd: "dotnet build", Source: "dotnet"})
		if len(projs) > 1 { // dotnet build needs a single project in the folder; build each instead
			out = out[:len(out)-1]
			for i, p := range projs {
				if i >= 6 {
					break
				}
				if a := shellArg(p); a != "" {
					base := strings.TrimSuffix(filepath.Base(p), filepath.Ext(p))
					out = append(out, Task{Name: "build " + base, Cmd: "dotnet build " + a, Source: "dotnet"})
				}
			}
		}
	}
	return out
}

// ------------------------------------------------------------------ Go

func hasMainPackage(dir string) bool {
	ents, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	for _, e := range ents {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") || strings.HasSuffix(e.Name(), "_test.go") {
			continue
		}
		b, _ := readCapped(filepath.Join(dir, e.Name()), 64<<10)
		if mainPkgRe.Match(b) {
			return true
		}
	}
	return false
}

func taskGo(dir string) []Task {
	if exists(dir, "go.mod") == "" {
		return nil
	}
	var out []Task
	if hasMainPackage(dir) {
		out = append(out, Task{Name: "run", Cmd: "go run .", Source: "go"})
	}
	if ents, err := os.ReadDir(filepath.Join(dir, "cmd")); err == nil {
		for _, e := range ents {
			if e.IsDir() && safeNameRe.MatchString(e.Name()) && hasMainPackage(filepath.Join(dir, "cmd", e.Name())) && len(out) < 12 {
				out = append(out, Task{Name: "run " + e.Name(), Cmd: "go run ./cmd/" + e.Name(), Source: "go"})
			}
		}
	}
	return append(out,
		Task{Name: "build", Cmd: "go build ./...", Source: "go"},
		Task{Name: "test", Cmd: "go test ./...", Source: "go"},
		Task{Name: "vet", Cmd: "go vet ./...", Source: "go"})
}

// ------------------------------------------------------------------ Rust

func taskCargo(dir string) []Task {
	if exists(dir, "Cargo.toml") == "" {
		return nil
	}
	return []Task{
		{Name: "run", Cmd: "cargo run", Source: "cargo"},
		{Name: "build", Cmd: "cargo build", Source: "cargo"},
		{Name: "test", Cmd: "cargo test", Source: "cargo"},
		{Name: "check", Cmd: "cargo check", Source: "cargo"},
	}
}

// ------------------------------------------------------------------ Python

func taskPython(dir string) []Task {
	manifests := []string{"pyproject.toml", "requirements.txt", "requirements-dev.txt", "setup.py", "Pipfile"}
	var deps strings.Builder
	found := false
	for _, m := range manifests {
		if b, err := readCapped(filepath.Join(dir, m), 256<<10); err == nil {
			found = true
			deps.Write(bytes.ToLower(b))
			deps.WriteByte('\n')
		}
	}
	hasManage := exists(dir, "manage.py") != ""
	if !found && !hasManage {
		return nil
	}
	d := deps.String()
	// Runner: uv / poetry / pipenv when their lock file is present, else plain python -m.
	prefix, install := "python -m ", "python -m pip install -r requirements.txt"
	switch {
	case exists(dir, "uv.lock") != "":
		prefix, install = "uv run ", "uv sync"
	case exists(dir, "poetry.lock") != "":
		prefix, install = "poetry run ", "poetry install"
	case exists(dir, "Pipfile.lock", "Pipfile") != "":
		prefix, install = "pipenv run ", "pipenv install"
	case exists(dir, "requirements.txt") == "":
		install = ""
	}
	mod := func(m string) string { return prefix + m } // "uv run uvicorn …" or "python -m uvicorn …"
	script := func(s string) string {                  // run a script file
		if prefix == "python -m " {
			return "python " + s
		}
		return prefix + "python " + s
	}
	var out []Task
	if hasManage {
		out = append(out,
			Task{Name: "runserver", Cmd: script("manage.py runserver"), Source: "python"},
			Task{Name: "migrate", Cmd: script("manage.py migrate"), Source: "python"},
			Task{Name: "test", Cmd: script("manage.py test"), Source: "python"})
	}
	if strings.Contains(d, "uvicorn") || strings.Contains(d, "fastapi") {
		for _, c := range []struct{ file, module string }{{"main.py", "main"}, {"app.py", "app"}, {"app/main.py", "app.main"}, {"src/main.py", "src.main"}, {"api/main.py", "api.main"}} {
			b, err := readCapped(filepath.Join(dir, filepath.FromSlash(c.file)), 256<<10)
			if err != nil {
				continue
			}
			if m := asgiAppRe.FindSubmatch(b); m != nil {
				out = append(out, Task{Name: "uvicorn", Cmd: mod("uvicorn " + c.module + ":" + string(m[1]) + " --reload"), Source: "python"})
				break
			}
		}
	}
	if strings.Contains(d, "flask") && !hasManage {
		out = append(out, Task{Name: "flask run", Cmd: mod("flask run --debug"), Source: "python"})
	}
	if strings.Contains(d, "pytest") || exists(dir, "pytest.ini", "conftest.py") != "" || (!hasManage && isDir(filepath.Join(dir, "tests"))) {
		out = append(out, Task{Name: "pytest", Cmd: mod("pytest"), Source: "python"})
	}
	if install != "" {
		out = append(out, Task{Name: "install", Cmd: install, Source: "python"})
	}
	return out
}

// ------------------------------------------------------------------ docker compose

func taskCompose(dir string) []Task {
	if exists(dir, "compose.yaml", "compose.yml", "docker-compose.yaml", "docker-compose.yml") == "" {
		return nil
	}
	return []Task{
		{Name: "up -d", Cmd: "docker compose up -d", Source: "compose"},
		{Name: "down", Cmd: "docker compose down", Source: "compose"},
		{Name: "logs", Cmd: "docker compose logs -f --tail 200", Source: "compose"},
		{Name: "ps", Cmd: "docker compose ps", Source: "compose"},
	}
}
