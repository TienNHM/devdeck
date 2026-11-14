# devdeck

A tiny, single-binary developer dashboard. Go backend, no-build web UI, **~10-15 MB RAM** working set, ~11 MB binary.

| Feature | What you get |
|---|---|
| **Ports & processes** | Listening TCP ports with owning process, process list by memory, kill / kill tree |
| **Git graph** | Lane-based commit graph of all branches (like the VS Code graph extensions), click a commit for stat and patch |
| **Redis browser** | Read-only: SCAN with pattern, type and TTL, view string / list / set / zset / hash / stream |
| **Terminal** | Local ConPTY shell (Windows) and SSH sessions in xterm.js tabs, optionally auto-running `claude` |
| **Claude Code** | MCP server (`devdeck mcp`), hook-based live session status, embedded `claude` terminals |

## Install

Requires Go 1.22+.

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

prints two things:

1. the `claude mcp add devdeck -- <path> mcp` command, which exposes these tools to Claude Code:
   `list_ports`, `list_processes`, `kill_process`, `git_graph`, `git_show`, `redis_scan`, `redis_get`, `ssh_exec`, `claude_sessions`
2. a `hooks` block to merge into `~/.claude/settings.json`; the **Claude** tab then shows each session as idle / working / waiting for input.

Redis passwords for the MCP tools are read from the `DEVDECK_REDIS_PASSWORD` environment variable, never from tool arguments.
`ssh_exec` only connects to hosts you saved in `~/.devdeck/config.json` and only uses key auth.

## Security model

devdeck can kill processes and open shells, so it is locked down:

- Listens on `127.0.0.1` only; requests whose `Host` is not loopback are rejected (DNS rebinding).
- Every request needs a random token (`~/.devdeck/token`), sent as an HttpOnly `SameSite=Strict` cookie or a bearer header.
- Writes and WebSockets are same-origin only.
- Redis is read-only. Passwords (Redis, SSH) are never written to disk; only host names, users and key paths are saved.
- SSH uses `~/.ssh/known_hosts`: unknown hosts are added on first use (`accept-new` behaviour), changed host keys are rejected.

## Status and limits

Early (v0.1). Known gaps: local terminal is Windows-only for now (SSH terminals work everywhere), no ssh-agent support, no Redis write operations by design, `~/.ssh/config` is not parsed.

## License

MIT
