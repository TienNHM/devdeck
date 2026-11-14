package main

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func writeFiles(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, body := range files {
		p := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func cmds(ts []Task) map[string]string {
	m := map[string]string{}
	for _, t := range ts {
		m[t.Source+":"+t.Name] = t.Cmd
	}
	return m
}

func wantCmd(t *testing.T, ts []Task, key, cmd string) {
	t.Helper()
	got, ok := cmds(ts)[key]
	if !ok {
		t.Fatalf("missing task %q in %v", key, cmds(ts))
	}
	if got != cmd {
		t.Fatalf("task %q: got %q, want %q", key, got, cmd)
	}
}

func noTask(t *testing.T, ts []Task, key string) {
	t.Helper()
	if _, ok := cmds(ts)[key]; ok {
		t.Fatalf("unexpected task %q", key)
	}
}

func TestNodeScriptsPackageManagerAndOrder(t *testing.T) {
	dir := writeFiles(t, map[string]string{
		"package.json":   `{"scripts":{"lint":"eslint .","build":"vite build","dev":"vite --port 5173","zeta":"x","x; rm -rf ~":"evil","predev":"echo"}}`,
		"pnpm-lock.yaml": "",
	})
	ts := detectTasks(dir)
	wantCmd(t, ts, "pnpm:dev", "pnpm run dev")
	wantCmd(t, ts, "pnpm:install", "pnpm install")
	noTask(t, ts, "pnpm:x; rm -rf ~")
	noTask(t, ts, "pnpm:predev")
	var order []string
	for _, x := range ts {
		order = append(order, x.Name)
	}
	if strings.Join(order, ",") != "dev,build,lint,zeta,install" {
		t.Fatalf("order: %v", order)
	}
	if ts[0].Body != "vite --port 5173" {
		t.Fatalf("body: %q", ts[0].Body)
	}
}

func TestNodePackageManagers(t *testing.T) {
	for lock, pm := range map[string]string{"yarn.lock": "yarn", "bun.lockb": "bun", "bun.lock": "bun", "package-lock.json": "npm"} {
		dir := writeFiles(t, map[string]string{"package.json": `{"scripts":{"start":"node ."}}`, lock: ""})
		wantCmd(t, detectTasks(dir), pm+":start", pm+" run start")
	}
	dir := writeFiles(t, map[string]string{"package.json": `{"packageManager":"pnpm@9.1.0","scripts":{"start":"node ."}}`})
	wantCmd(t, detectTasks(dir), "pnpm:start", "pnpm run start")
}

func TestMakefileTargets(t *testing.T) {
	dir := writeFiles(t, map[string]string{"Makefile": `.PHONY: build test
VAR := value
OTHER ?= x
build: deps
	go build ./...
test lint:
	go test ./...
%.o: %.c
	cc -c $<
# comment: not a target
deploy::
`})
	ts := detectTasks(dir)
	for _, n := range []string{"build", "test", "lint", "deploy"} {
		wantCmd(t, ts, "make:"+n, "make "+n)
	}
	for _, n := range []string{".PHONY", "VAR", "OTHER", "%.o"} {
		noTask(t, ts, "make:"+n)
	}
}

func TestJustfileRecipes(t *testing.T) {
	dir := writeFiles(t, map[string]string{"justfile": `set shell := ["bash", "-c"]
alias b := build

# build it
build:
    cargo build

serve port="8080":
    python -m http.server {{port}}

deploy target:
    echo {{target}}

_helper:
    echo hidden

[private]
secret:
    echo no

@quiet:
    echo shh
`})
	ts := detectTasks(dir)
	wantCmd(t, ts, "just:build", "just build")
	wantCmd(t, ts, "just:serve", "just serve")
	wantCmd(t, ts, "just:quiet", "just quiet")
	for _, n := range []string{"set", "alias", "deploy", "_helper", "secret"} {
		noTask(t, ts, "just:"+n)
	}
}

func TestDotnetSolutionAndProjects(t *testing.T) {
	dir := writeFiles(t, map[string]string{
		"App.sln":                          "",
		"src/Api/Api.csproj":               `<Project Sdk="Microsoft.NET.Sdk.Web"></Project>`,
		"src/Lib/Lib.csproj":               `<Project Sdk="Microsoft.NET.Sdk"></Project>`,
		"src/Tool/Tool.csproj":             `<Project Sdk="Microsoft.NET.Sdk"><PropertyGroup><OutputType>Exe</OutputType></PropertyGroup></Project>`,
		"tests/Api.Tests/Api.Tests.csproj": `<Project Sdk="Microsoft.NET.Sdk"><ItemGroup><PackageReference Include="Microsoft.NET.Test.Sdk" /></ItemGroup></Project>`,
		"src/Api/bin/Debug/Copy.csproj":    `<Project Sdk="Microsoft.NET.Sdk.Web"></Project>`,
	})
	ts := detectTasks(dir)
	api := filepath.Join("src", "Api", "Api.csproj")
	wantCmd(t, ts, "dotnet:build App.sln", "dotnet build App.sln")
	wantCmd(t, ts, "dotnet:test App.sln", "dotnet test App.sln")
	wantCmd(t, ts, "dotnet:run Api", "dotnet run --project "+api)
	wantCmd(t, ts, "dotnet:watch Api", "dotnet watch --project "+api+" run")
	wantCmd(t, ts, "dotnet:run Tool", "dotnet run --project "+filepath.Join("src", "Tool", "Tool.csproj"))
	noTask(t, ts, "dotnet:run Lib")
	noTask(t, ts, "dotnet:run Api.Tests")
	noTask(t, ts, "dotnet:run Copy")
}

func TestDotnetSingleProjectWithSpaces(t *testing.T) {
	dir := writeFiles(t, map[string]string{"My App/My App.csproj": `<Project Sdk="Microsoft.NET.Sdk.Web"></Project>`})
	ts := detectTasks(dir)
	p := filepath.Join("My App", "My App.csproj")
	wantCmd(t, ts, "dotnet:run My App", "dotnet run --project '"+p+"'")
	wantCmd(t, ts, "dotnet:build", "dotnet build")
}

func TestShellArgRejectsMetacharacters(t *testing.T) {
	for _, s := range []string{"a'b", "a$b", "a`b", "a;b", "a|b", "a&b", "x\ny"} {
		if got := shellArg(s); got != "" {
			t.Fatalf("shellArg(%q) = %q, want rejection", s, got)
		}
	}
	if got := shellArg(`src\App.csproj`); got != `src\App.csproj` {
		t.Fatal(got)
	}
	if got := shellArg("a b"); got != "'a b'" {
		t.Fatal(got)
	}
}

func TestGoModule(t *testing.T) {
	dir := writeFiles(t, map[string]string{
		"go.mod":              "module x\n",
		"main.go":             "package main\nfunc main(){}\n",
		"cmd/tool/main.go":    "package main\nfunc main(){}\n",
		"cmd/notmain/x.go":    "package lib\n",
		"internal/lib/lib.go": "package lib\n",
	})
	ts := detectTasks(dir)
	wantCmd(t, ts, "go:run", "go run .")
	wantCmd(t, ts, "go:run tool", "go run ./cmd/tool")
	wantCmd(t, ts, "go:test", "go test ./...")
	wantCmd(t, ts, "go:build", "go build ./...")
	noTask(t, ts, "go:run notmain")

	lib := writeFiles(t, map[string]string{"go.mod": "module y\n", "lib.go": "package y\n"})
	noTask(t, detectTasks(lib), "go:run")
}

func TestCargoAndCompose(t *testing.T) {
	dir := writeFiles(t, map[string]string{"Cargo.toml": "[package]\n", "compose.yaml": "services: {}\n"})
	ts := detectTasks(dir)
	wantCmd(t, ts, "cargo:run", "cargo run")
	wantCmd(t, ts, "cargo:test", "cargo test")
	wantCmd(t, ts, "compose:up -d", "docker compose up -d")
	wantCmd(t, ts, "compose:down", "docker compose down")
	wantCmd(t, ts, "compose:logs", "docker compose logs -f --tail 200")

	old := writeFiles(t, map[string]string{"docker-compose.yml": ""})
	wantCmd(t, detectTasks(old), "compose:up -d", "docker compose up -d")
}

func TestPythonFastAPIWithUv(t *testing.T) {
	dir := writeFiles(t, map[string]string{
		"pyproject.toml": "[project]\ndependencies = [\"fastapi\", \"uvicorn\"]\n[dependency-groups]\ndev = [\"pytest\"]\n",
		"uv.lock":        "",
		"app/main.py":    "from fastapi import FastAPI\napi = FastAPI()\n",
	})
	ts := detectTasks(dir)
	wantCmd(t, ts, "python:uvicorn", "uv run uvicorn app.main:api --reload")
	wantCmd(t, ts, "python:pytest", "uv run pytest")
	wantCmd(t, ts, "python:install", "uv sync")
}

func TestPythonDjangoAndRequirements(t *testing.T) {
	dir := writeFiles(t, map[string]string{"manage.py": "", "requirements.txt": "Django==5.0\npytest-django\n"})
	ts := detectTasks(dir)
	wantCmd(t, ts, "python:runserver", "python manage.py runserver")
	wantCmd(t, ts, "python:migrate", "python manage.py migrate")
	wantCmd(t, ts, "python:pytest", "python -m pytest")
	wantCmd(t, ts, "python:install", "python -m pip install -r requirements.txt")

	flask := writeFiles(t, map[string]string{"requirements.txt": "flask\n", "poetry.lock": ""})
	wantCmd(t, detectTasks(flask), "python:flask run", "poetry run flask run --debug")
}

func TestEmptyFolderHasNoTasks(t *testing.T) {
	if ts := detectTasks(t.TempDir()); len(ts) != 0 {
		t.Fatalf("got %v", ts)
	}
}

func TestCleanProjectPath(t *testing.T) {
	dir := writeFiles(t, map[string]string{"file.txt": "x"})
	if p, err := cleanProjectPath(`"` + dir + `"`); err != nil || p != filepath.Clean(dir) {
		t.Fatalf("got %q, %v", p, err)
	}
	if _, err := cleanProjectPath(filepath.Join(dir, "file.txt")); err == nil {
		t.Fatal("file accepted as project")
	}
	if _, err := cleanProjectPath(filepath.Join(dir, "missing")); err == nil {
		t.Fatal("missing folder accepted")
	}
	if _, err := cleanProjectPath("relative/dir"); err == nil {
		t.Fatal("relative path accepted")
	}
	if _, err := cleanProjectPath("   "); err == nil {
		t.Fatal("empty path accepted")
	}
}

func TestPathInsideAndCmdline(t *testing.T) {
	root := filepath.Join(t.TempDir(), "app")
	if !pathInside(filepath.Join(root, "src"), root) || !pathInside(root, root) {
		t.Fatal("inside not detected")
	}
	if pathInside(root+"-other", root) || pathInside(filepath.Dir(root), root) {
		t.Fatal("sibling or parent treated as inside")
	}
	if runtime.GOOS == "windows" && !pathInside(strings.ToUpper(root), root) {
		t.Fatal("windows paths are case-insensitive")
	}
	cmd := `node "` + filepath.Join(root, "node_modules", "vite", "bin", "vite.js") + `" --port 5173`
	if !cmdlineMentions(cmd, root) {
		t.Fatal("cmdline path not detected")
	}
	if cmdlineMentions(`node `+root+`-other/x.js`, root) {
		t.Fatal("prefix match on sibling folder")
	}
	if !cmdlineMentions("python -m http.server --directory "+root, root) {
		t.Fatal("cmdline ending with the path not detected")
	}
}

func TestMatchProjectPorts(t *testing.T) {
	root := filepath.Join(t.TempDir(), "web")
	ports := []PortInfo{
		{Port: 5173, PID: 10, Cwd: root},
		{Port: 8080, PID: 11, Cwd: filepath.Join(root, "api")},
		{Port: 6379, PID: 12, Cwd: filepath.Dir(root)},
		{Port: 3000, PID: 13, Cmdline: "node " + filepath.Join(root, "server.js")},
	}
	got := matchProjectPorts([]Project{{Path: root}}, ports)
	var ns []uint32
	for _, p := range got[0].Ports {
		ns = append(ns, p.Port)
	}
	if len(ns) != 3 || ns[0] != 5173 || ns[1] != 8080 || ns[2] != 3000 {
		t.Fatalf("got %v", ns)
	}
}
