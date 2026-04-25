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
```

Then open http://localhost:27183.

## Auto-start on login (macOS)

```bash
cp com.pi-sessions-viewer.plist ~/Library/LaunchAgents/
launchctl load ~/Library/LaunchAgents/com.pi-sessions-viewer.plist
```

The viewer starts automatically on boot and runs in the background.

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
