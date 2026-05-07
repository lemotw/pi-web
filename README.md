# pi-web

A local web viewer for [pi](https://pi.dev) coding agent sessions.

It renders sessions in the browser using pi's export UI, adds live updates, and lets you continue sessions from the web.

## Features

- Browse sessions across projects
- Full session tree with filters, search, and branch navigation
- Live incremental updates while pi is still running
- Follow mode for tailing active sessions
- Continue a session from the browser with text or image attachments
- Per-session worker status and in-browser model switching
- Deep links to individual messages
- Download a session as JSONL
- Share static snapshots as secret GitHub Gists
- `/view` pi extension to open the current session in the browser

## Requirements

- [Go](https://go.dev) 1.25+
- `pi` on your `PATH` for browser chat/model switching
- Optional: `gh` for sharing

## Install

```bash
git clone https://github.com/setkyar/pi-web.git
cd pi-web
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

> Warning: by default there is no authentication. Anyone who can reach the bound address can view sessions and send instructions to pi.
>
> To require a token, set `PI_WEB_TOKEN` before starting `pi-web`. Clients can pass it via the `Authorization: Bearer <token>` header, the `X-Pi-Token` header, or once via `?token=<token>` (which sets a `pi_token` cookie for subsequent requests). Tokens passed via `?token=` end up in browser history, server access logs, and `Referer` headers from any links on the page — prefer the header form for anything beyond the initial bookmark.

## Browser chat

Open a session page and use the composer at the bottom to continue that exact session.

- `Enter` sends
- `Shift+Enter` inserts a newline
- Image uploads are supported
- Each active session gets its own headless `pi --mode rpc` worker
- Multiple sessions can run in parallel, so concurrent edits are possible

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

pi-web reads session JSONL files from `~/.pi/agent/sessions/`, renders them with embedded pi export templates, watches for file changes, and pushes updates to the browser over SSE.

## Development

```bash
go test ./...
```

## License

MIT
