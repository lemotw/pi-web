# pi-web (Remote Control Your Pi)

Drive your [pi](https://pi.dev) coding agent from any browser on your network — laptop, phone, or tablet.

pi-web is a local Go server that renders pi sessions in the browser using pi's own export UI, streams live updates as pi works, and lets you steer or start sessions from the same page. Bind it to your Tailscale IP and pi follows you off the desk.

## Features

### Remote control

- Continue any session from the browser with text or image attachments
- Start a brand-new session against any project path, right from the web UI
- In-browser model switching and thinking-level selector, per session
- Per-session worker status (idle / running / error) with auto-recovery on crash
- Multiple sessions run in parallel — kick off work in one, watch another stream
- Optional `PI_WEB_TOKEN` so you can safely expose it on Tailscale or a LAN

### Reading sessions

- Browse sessions across projects with filters, search, and full branch navigation
- Live incremental updates while pi is still running (via fsnotify; ~ms latency)
- Follow mode for tailing active sessions
- Deep links to individual messages
- Download a session as JSONL
- Share static snapshots as secret GitHub Gists
- `/view` pi extension to open the current session in the browser from inside pi

## Requirements

- [Go](https://go.dev) 1.25+
- `pi` on your `PATH` for browser chat/model switching
- Optional: `gh` for sharing

## Install

```bash
git clone https://github.com/setkyar/pi-web.git
cd pi-web
cd web && npm install && npm run build && cd ..
go build -o pi-web .

# optional: put it on PATH
cp pi-web ~/.pi/agent/bin/
# or
sudo cp pi-web /usr/local/bin/
```

## Usage

```bash
# Start on the default port (31483)
pi-web

# Start and open a browser
pi-web -o

# Custom port
pi-web -p 8080

# Override bind host
pi-web --host 127.0.0.1
pi-web --host 100.x.y.z
```

By default, pi-web binds to your Tailscale IP when available, otherwise `127.0.0.1`.

## Remote access

The default Tailscale bind is the point of the project: leave pi running on your desktop, then drive it from your phone on the couch or your laptop on the train.

```bash
# 1. Start pi-web — it auto-detects your Tailscale IP
PI_WEB_TOKEN=$(openssl rand -hex 16) pi-web

# 2. From any other Tailscale-connected device, open the printed URL
#    and paste the token once. The cookie persists.
```

> Warning: by default there is no authentication. Anyone who can reach the bound address can view sessions and send instructions to pi. **If you bind to anything beyond `127.0.0.1`, set `PI_WEB_TOKEN`.**
>
> Clients can pass the token via the `Authorization: Bearer <token>` header, the `X-Pi-Token` header, or once via `?token=<token>` (which sets a `pi_token` cookie for subsequent requests). Tokens passed via `?token=` end up in browser history, server access logs, and `Referer` headers from any links on the page — prefer the header form for anything beyond the initial bookmark.

## Browser chat

Open a session page and use the composer at the bottom to continue that exact session.

- `Enter` sends, `Shift+Enter` inserts a newline
- Drag-and-drop or paste images directly into the composer
- The model picker and thinking-level selector live in the header — changes apply to the underlying pi worker immediately
- Each active session gets its own dedicated `pi --mode rpc` worker, so different sessions don't block each other

## Pi integration

### `/view` command

```bash
mkdir -p ~/.pi/agent/extensions
cp view-sessions.ts ~/.pi/agent/extensions/
```

Restart pi (or run `/reload`), then use `/view` inside a session.

### Skill

```bash
mkdir -p ~/.pi/agent/skills
cp -r skill ~/.pi/agent/skills/pi-web
```

## Sharing sessions

Click **Share** on a session page to create a secret GitHub Gist.

Requirements:
- `gh` installed
- `gh auth login` completed

Sharing returns:
- the secret gist URL
- a preview URL at `https://pi.dev/session/#<gistId>`

Shared gists are snapshots and do not live-update.

## Auto-start on login (macOS)

```bash
cp com.pi-web.plist ~/Library/LaunchAgents/
launchctl load ~/Library/LaunchAgents/com.pi-web.plist
```

## How it works

pi-web reads session JSONL files from `~/.pi/agent/sessions/` and renders them with pi's own export templates, embedded into the binary at build time. There are three moving parts:

- **Live reload.** A `fsnotify` watcher tails the sessions directory and pushes a one-line SSE event to any connected browser the moment pi appends to the file. A 1.5s polling fallback kicks in if fsnotify can't initialize (e.g. NFS).
- **Per-session workers.** When you send a message from the browser, pi-web spawns a headless `pi --mode rpc` subprocess scoped to that session, switches it to the session file, and forwards your prompt. Subsequent messages reuse the same worker. If the worker crashes it's evicted and replaced on the next request; idle workers are reaped after 30 minutes so long-lived servers don't accumulate processes.
- **Sharing.** Renders a self-contained HTML snapshot and shells out to `gh gist create --public=false`. Snapshots don't live-update.

A single binary, no database, no daemon — just a Go HTTP server reading the same JSONL pi already writes.

## Development

The sessions index uses a Vite-built browser bundle. Rebuild it after frontend changes before running the Go server from source:

```bash
cd web
npm install
npm run test
npm run build
cd ..
go test ./...
go build -o pi-web .
```

## License

MIT
