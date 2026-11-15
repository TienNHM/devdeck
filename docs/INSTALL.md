# Install and run devdeck

[Tiếng Việt](INSTALL.vi.md)

devdeck is one executable. There is no installer and nothing runs in the background unless you start it.

## 1. Get it

### Windows

**Option A: download**
1. Open the [latest release](https://github.com/TienNHM/devdeck/releases/latest) and download `devdeck_<version>_windows_amd64.zip` (or `windows_arm64` on ARM PCs).
2. Unzip it to a folder you keep, for example `C:\Tools\devdeck\`.
3. Double-click `devdeck.exe`, or run it from a terminal:
   ```powershell
   C:\Tools\devdeck\devdeck.exe --open
   ```
   The exe is not code-signed yet, so Windows SmartScreen may say "Windows protected your PC". Click **More info → Run anyway**.

**Option B: Scoop**
```powershell
scoop install https://raw.githubusercontent.com/TienNHM/devdeck/main/scoop/devdeck.json
devdeck --open
```

### macOS
```bash
# Apple Silicon: darwin_arm64, Intel: darwin_amd64
curl -LO https://github.com/TienNHM/devdeck/releases/download/v0.2.0/devdeck_v0.2.0_darwin_arm64.tar.gz
tar -xzf devdeck_v0.2.0_darwin_arm64.tar.gz
cd devdeck_v0.2.0_darwin_arm64
xattr -d com.apple.quarantine ./devdeck 2>/dev/null   # the binary is not notarized
./devdeck --open
```

### Linux
```bash
# x86-64: linux_amd64, ARM: linux_arm64
curl -LO https://github.com/TienNHM/devdeck/releases/download/v0.2.0/devdeck_v0.2.0_linux_amd64.tar.gz
tar -xzf devdeck_v0.2.0_linux_amd64.tar.gz
sudo install devdeck_v0.2.0_linux_amd64/devdeck /usr/local/bin/devdeck   # optional
devdeck --open
```

### From source
Requires Go 1.27+ and git.
```bash
git clone https://github.com/TienNHM/devdeck
cd devdeck
go build -trimpath -ldflags "-s -w" -o devdeck.exe .   # name it "devdeck" on macOS/Linux
./devdeck.exe --open
```

## 2. First run

When it starts, devdeck prints something like:
```
devdeck v0.2.0 listening on 127.0.0.1 only
open: http://127.0.0.1:7077/?t=3f9c…
```
- With `--open` the browser opens that link for you; otherwise copy it into your browser.
- The `?t=…` part is your access token. You need it only the first time; after that the browser remembers you, so `http://127.0.0.1:7077/` is enough.
- Keep the terminal window open: closing it stops devdeck.

Things to try first:
- **Ctrl+K** opens the command palette (try `kill port 3000`, `ssh`, `go git`).
- **Projects → Add project**, paste a folder path, then run one of the detected tasks.
- **Git**, add a repository and right-click a commit or branch.
- The globe button switches English / Tiếng Việt; the moon button switches dark / light.

## 3. Options

| Command | What it does |
|---|---|
| `devdeck` | Start the dashboard on port 7077 |
| `devdeck --port 7080` | Use another port (if 7077 is busy) |
| `devdeck --open` | Also open the browser |
| `devdeck version` | Print the version |
| `devdeck hooks-config` | Print the Claude Code setup (see below) |
| `devdeck mcp` | MCP server for Claude Code (Claude starts this itself) |

## 4. Connect Claude Code (optional)

1. Run `devdeck hooks-config`. It prints two things.
2. Run the `claude mcp add devdeck -- "<path>" mcp` line it printed. Claude Code can now use devdeck's tools (ports, git, Redis, databases, Docker, logs, SSH…).
3. Merge the printed `"hooks"` block into `~/.claude/settings.json` (on Windows: `C:\Users\<you>\.claude\settings.json`). Start a new Claude Code session; it appears in the **Claude** tab.
4. In devdeck, click the bell icon to get a desktop notification when Claude needs you or finishes.

The **Claude → Setup** button shows the same text and tells you whether the hooks are installed. devdeck never edits your Claude settings.

If you move `devdeck.exe` later, run `claude mcp remove devdeck` and repeat steps 2–3 with the new path.

## 5. Stop, start with your computer, update, uninstall

**Stop:** click **Quit** (top right) or close the terminal window / press Ctrl+C.

**Start when you log in**
- Windows: press Win+R, type `shell:startup`, and create a shortcut to `devdeck.exe` there. In the shortcut's Properties set **Run: Minimized**.
- Linux (systemd user service):
  ```bash
  mkdir -p ~/.config/systemd/user
  cat > ~/.config/systemd/user/devdeck.service <<'EOF'
  [Unit]
  Description=devdeck
  [Service]
  ExecStart=/usr/local/bin/devdeck
  Restart=on-failure
  [Install]
  WantedBy=default.target
  EOF
  systemctl --user enable --now devdeck
  ```
- macOS: add a launchd agent or run `devdeck` from your shell profile.

**Update:** stop devdeck, replace the executable with the one from the new release, start it again. Your settings are kept.

**Uninstall:** delete the executable and the `~/.devdeck` folder (`C:\Users\<you>\.devdeck` on Windows). If you connected Claude Code, run `claude mcp remove devdeck` and remove the devdeck hooks from `~/.claude/settings.json`.

## 6. Where your data lives

Everything is in `~/.devdeck/`: the access token, saved hosts, projects, repositories, Redis/database profiles, tunnels and log file lists. Passwords are never saved there.

## 7. Troubleshooting

| Problem | Fix |
|---|---|
| `cannot listen: … bind: …` (Windows: "Only one usage of each socket address", macOS/Linux: "address already in use") | devdeck (or something else) already uses 7077. Open `http://127.0.0.1:7077/`, or start with `--port 7080` |
| Browser says `unauthorized` | Open the full link with `?t=…` once. Lost it? It is in `~/.devdeck/token`, or restart devdeck to print it again |
| Page shows `forbidden host` | Use `127.0.0.1` or `localhost`. devdeck refuses other host names on purpose |
| Docker tab says Docker is not running | Start Docker Desktop (or the docker daemon) and click Retry |
| A process shows `(access denied)` | It belongs to the system or another user. Run devdeck as administrator only if you really need to see it |
| Claude tab stays empty | Check **Claude → Setup**; the hooks must be in settings.json and devdeck must be running when Claude starts |
| Terminal looks narrow or empty | Resize the window once; if it persists, reload the page (terminals survive a reload) |
