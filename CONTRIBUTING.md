# Contributing to devdeck

Thanks for helping! devdeck has two goals that every change should respect:

1. **Stay light.** One static binary, a few MB of RAM when idle. Nothing polls unless the user is looking at it.
2. **Stay safe.** It can kill processes, run git, open shells. Every input is validated and every destructive action is confirmed.

## Dev loop

```bash
go run . --open          # dashboard on http://127.0.0.1:7077
go vet ./... && go test ./...
./scripts/build.ps1      # Windows build with version info (or: go build -trimpath -ldflags "-s -w" .)
```

The web UI is plain HTML/CSS/JS in `web/`, embedded with `go:embed`. There is no npm and no build step: edit, rebuild, reload.
`CGO_ENABLED=0` must always build; CI checks Windows, Linux and macOS.

## How the code is organised

Each feature is a module that only touches its own files.

| Layer | Where | How a module plugs in |
|---|---|---|
| HTTP routes | `<feature>*.go` | `func init(){ addRoutes(func(mux *http.ServeMux){ ... }) }` (see `registry.go`); routes are mounted behind the auth guard automatically |
| MCP tools for Claude Code | `<feature>*.go` | `registerMCPTool(tool{...})` |
| Persistence | `~/.devdeck/<name>.json` | `loadJSON(name, &v)` / `saveJSON(name, v)`; never store passwords or tokens |
| UI | `web/<feature>.js`, `web/<feature>.css`, `<section id="feature">` in `index.html` | Shared globals from `web/app.js` (see its header comment): `api`, `post`, `toast`, `dialog`, `onTab`, `every`, `registerCommand`, `t`… |

### Frontend rules
- Load data in `onTab('<tab>', fn)`; poll only with `every(ms, '<tab>', fn)` (it pauses when the tab or window is hidden).
- Confirm destructive actions with `dialog({..., danger: true})`; report results with `toast()`.
- Wrap user-visible text in `t('module.key', 'English')` and add Vietnamese with `registerStrings('vi', {...})`.
- Prefix CSS classes with your module name; use the CSS variables from `style.css` so light and dark themes both work.
- Add useful entries to the Ctrl+K palette with `registerCommand` / `registerCommandProvider`.

### Backend rules
- Run external tools with `exec.Command(name, args...)`, never through a shell string. Validate refs, ids, paths; reject leading `-`.
- Always use timeouts and cap output sizes. Stream with `r.Context()` so work stops when the browser goes away.
- Long-lived things (terminals, tunnels, connections) must have an idle limit.

## Tests
Write Go tests for backend logic. Use temp dirs, fake binaries (`DEVDECK_DOCKER_BIN`, `DEVDECK_CLAUDE_BIN`) and in-process servers (miniredis, an in-process SSH server) instead of real services. Never call the real `claude` binary in tests.

## Releases
Push a tag `vX.Y.Z`; `.github/workflows/release.yml` builds Windows/macOS/Linux archives and publishes the GitHub release with `checksums.txt`. Then update `scoop/devdeck.json` (version and hashes).
