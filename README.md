# pi-sessions-viewer

A local web viewer for [pi](https://pi.dev) coding agent sessions.

Browse, search, and share all your pi sessions from a browser. Renders sessions using the same HTML/CSS as pi's `/export` command, with live incremental updates as you chat.

## Features

- **Session browser** - Grid view of all sessions, grouped by project
- **Tree navigation** - Full session tree with filters (same UI as `/export`)
- **Live updates** - New messages append without page refresh
- **Follow mode** - Auto-scrolls to latest message; pauses when you scroll up to read history
- **Share** - Create secret GitHub Gists directly from the browser
- **`/view` command** - Type `/view` inside pi to open the current session

## Install

Requires [Go](https://go.dev) 1.21+.

```bash
git clone https://github.com/ygncode/pi-sessions-viewer.git
cd pi-sessions-viewer
go build -o pi-sessions-viewer .

# Put on PATH
sudo cp pi-sessions-viewer /usr/local/bin/
# or
cp pi-sessions-viewer ~/.pi/agent/bin/
```

## Usage

```bash
# Start server (default port 27183)
pi-sessions-viewer

# Start and open browser
pi-sessions-viewer -o

# Custom port
pi-sessions-viewer -p 8080

# Bind manually
pi-sessions-viewer --host 127.0.0.1
pi-sessions-viewer --host 100.x.y.z
```

By default, the viewer tries to bind to your Tailscale IP. If Tailscale is not available, it falls back to `127.0.0.1`.

Warning: v1 has no authentication. Anyone who can reach the bound address can view sessions and send instructions to pi.

Then open the URL printed at startup.

## Auto-start on login (macOS)

```bash
cp com.pi-sessions-viewer.plist ~/Library/LaunchAgents/
launchctl load ~/Library/LaunchAgents/com.pi-sessions-viewer.plist
```

The viewer starts automatically on boot and runs in the background.

## Browser chat

Session pages include a compact composer at the bottom. Type instructions and press Enter to continue the same pi session from the browser. Use Shift+Enter for a newline.

The image icon attaches images. v1 supports image attachments only; arbitrary files are not uploaded.

Each active session gets its own headless `pi --mode rpc` worker. Multiple sessions can run in parallel, including sessions from different projects. Be careful: parallel agents may edit files concurrently.

## Pi integration

### `/view` command

Install the extension to add a `/view` command inside pi:

```bash
cp view-sessions.ts ~/.pi/agent/extensions/
```

Restart pi (or run `/reload`). Then type `/view` while in a session to open it in your browser.

### Skill

Install the skill so pi suggests the viewer when relevant:

```bash
cp -r skill ~/.pi/agent/skills/pi-sessions-viewer
```

## Sharing sessions

On any session page, click **Share** in the top-right to create a secret GitHub Gist.

Requirements:
- `gh` CLI installed (`brew install gh`)
- Logged in (`gh auth login`)

The share produces:
- A secret gist URL
- A preview URL at `https://pi.dev/session/#<gistId>`

Shared gists are static snapshots and do not auto-update.

## How it works

Sessions are read from `~/.pi/agent/sessions/` as JSONL files. The server:

1. Parses each `.jsonl` session file
2. Generates HTML using pi's export templates (embedded in the binary)
3. Watches files for changes and pushes SSE events to the browser
4. The browser fetches new entries via `/api/session` and appends them incrementally

## License

MIT
