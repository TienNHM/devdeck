# devdeck

A small, single-binary developer dashboard that runs in your browser. Go backend, no-build web UI, **about 11–20 MB of RAM**, one ~20 MB executable, no installer.

devdeck listens on `127.0.0.1` only and talks to Claude Code through MCP and hooks.

## Features

| Area | What you get |
|---|---|
| **Ports** | Every listening TCP port with its process, command line, folder and a dev-server label (Vite, Next.js, .NET, Django, Postgres…). Open in browser, kill, group by process |
| **Processes** | Flat or tree view, CPU %, memory, details drawer with live RAM/CPU charts (sampled only while open), kill / kill tree |
| **Projects** | Add project folders; devdeck finds npm/pnpm/yarn/bun scripts, Makefile/justfile targets, dotnet, go, cargo, Python and compose tasks, runs them in a terminal, and links each project to the ports it opened |
| **Git** | A browser take on the vscode-git-graph feature set: lane graph, branches/tags/stashes/uncommitted changes, right-click actions (checkout, merge, rebase, cherry-pick, revert, reset, push, pull, tags, stashes), commit details with file tree and diffs, compare commits, find, branch filter, remotes, keyboard shortcuts. **Commit…** dialog with "Generate with Claude" |
| **Redis** | Saved profiles, key tree by `:`, JSON viewer, server stats. Read-only by default; optional **write mode** (chosen per connection, every change confirmed) |
| **Database** | Read-only viewer for PostgreSQL, MySQL/MariaDB and SQLite: schema browser, data grid with paging/sort/filter, SQL editor (always in a read-only transaction), CSV export |
| **Docker** | Containers (start/stop/restart/remove, live logs, stats, inspect, shell), images, compose projects |
| **Logs** | Follow log files live with filter, regex, level colours (also JSON logs), pause and wrap |
| **Terminal** | Local shells (PowerShell, pwsh, cmd, Git Bash on Windows; your `$SHELL` on macOS/Linux) and SSH terminals in tabs. **Terminals survive a page reload** |
| **SSH** | Saved hosts plus `~/.ssh/config`, ssh-agent, ProxyJump; **tunnel manager** (local forwards); **SFTP file browser** (browse, preview, upload, download, rename, mkdir, delete) |
| **Claude** | Live Claude Code sessions via hooks, **desktop notification + sound when a session needs you or finishes**, last prompt / tokens / model per session, resume, **Ask Claude** (streams `claude -p`), PR descriptions and commit explanations from the Git tab |
| **Everywhere** | Ctrl+K command palette ("kill port 3000", "ssh prod", "run dev in my-app"…), light/dark theme, English / Tiếng Việt |

## Install

### Download
Grab the archive for your OS from the [Releases](https://github.com/TienNHM/devdeck/releases) page, unpack it and run `devdeck` (or `devdeck.exe`).

### Scoop (Windows)
```powershell
scoop install https://raw.githubusercontent.com/TienNHM/devdeck/main/scoop/devdeck.json
```

### From source
Requires Go 1.27+ (see `go.mod`).
```bash
git clone https://github.com/TienNHM/devdeck
cd devdeck
go build -trimpath -ldflags "-s -w" -o devdeck.exe .
./devdeck.exe --open
```

The server prints a URL containing an access token. Open it once; the token is then kept in an HttpOnly cookie.

## Claude Code integration

```bash
devdeck hooks-config
```

prints two things (also shown under **Claude → Setup**):

1. `claude mcp add devdeck -- <path> mcp`, which gives Claude Code these tools:
   `list_ports`, `port_owner`, `list_processes`, `kill_process`, `list_projects`,
   `git_graph`, `git_show`, `git_status`, `git_log_search`, `git_create_branch`, `git_stash`,
   `redis_scan`, `redis_get`, `db_query`, `db_tables`,
   `list_containers`, `container_logs`, `tail_file`,
   `ssh_exec`, `list_tunnels`, `claude_sessions`
2. a `hooks` block to merge into `~/.claude/settings.json`; the Claude tab then shows each session as idle / working / waiting and can notify you. devdeck only reads that file, it never writes it.

Secrets for the MCP tools come from environment variables, never from tool arguments: `DEVDECK_REDIS_PASSWORD`, `DEVDECK_DB_PASSWORD`. `ssh_exec` uses keys or ssh-agent only.

## Security model

devdeck can kill processes, run git and open shells, so it is locked down:

- Listens on `127.0.0.1` only; requests whose `Host` is not loopback are rejected (DNS rebinding).
- Every request needs a random token (`~/.devdeck/token`), sent as an HttpOnly `SameSite=Strict` cookie or a bearer header.
- Writes and WebSockets are same-origin only.
- External tools (git, docker, claude) run with argument lists, never through a shell string; inputs are validated.
- Destructive actions always ask first. Redis writes are refused on read-only connections; database queries always run in a read-only transaction (also use a read-only DB role for a hard guarantee).
- Passwords (Redis, databases, SSH) are never written to disk.
- SSH uses `~/.ssh/known_hosts`: unknown hosts are added on first use (`accept-new`), changed host keys are rejected.

## Staying light

Nothing polls unless you are looking at it: tabs refresh only while visible, charts sample only while open, log/docker streams stop when you leave, idle SSH/SFTP/DB/Redis connections close themselves, and detached terminals are reaped after 30 minutes.

## Limits

- Git tab: no author avatars (would send emails to a third party), GPG signature status, interactive rebase or config export.
- PostgreSQL and MySQL support is unit-tested but has had little real-world use yet; please report issues.
- SFTP deletes empty folders only. Tunnels are local forwards (`-L`) only.

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md). Each feature is a module (`<feature>.go` + `web/<feature>.js/.css`) that plugs into small registries, so new tabs do not touch shared files.

## License

MIT
