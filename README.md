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
- `PI_WEB_TOKEN` for safe Tailscale/LAN exposure — required by default for any non-loopback bind

### Reading sessions

- Browse sessions across projects with filters, search, and full branch navigation
- Live incremental updates while pi is still running (via fsnotify; ~ms latency)
- Follow mode for tailing active sessions
- Deep links to individual messages
- Download a session as JSONL
- Share static snapshots as secret GitHub Gists
- `/web`, `/mobile`, `/refresh` pi extensions for browser, mobile QR, and session sync

## Requirements

- [Go](https://go.dev) 1.25+
- `pi` on your `PATH` for browser chat/model switching
- Optional: `gh` for sharing

## Install

### Download binary (recommended)

Pre-built binaries are attached to each [GitHub Release](https://github.com/ygncode/pi-web/releases).

```bash
# macOS (Apple Silicon)
curl -L -o pi-web https://github.com/ygncode/pi-web/releases/latest/download/pi-web-darwin-arm64
chmod +x pi-web

# macOS (Intel)
curl -L -o pi-web https://github.com/ygncode/pi-web/releases/latest/download/pi-web-darwin-amd64
chmod +x pi-web

# Linux (amd64)
curl -L -o pi-web https://github.com/ygncode/pi-web/releases/latest/download/pi-web-linux-amd64
chmod +x pi-web

# Linux (arm64)
curl -L -o pi-web https://github.com/ygncode/pi-web/releases/latest/download/pi-web-linux-arm64
chmod +x pi-web
```

Then move it to your PATH:

```bash
cp pi-web ~/.pi/agent/bin/
# or system-wide:
sudo cp pi-web /usr/local/bin/
```

### Build from source

```bash
git clone https://github.com/ygncode/pi-web.git
cd pi-web
make build   # builds the Vite bundle, then embeds it into the Go binary

# optional: put it on PATH
cp pi-web ~/.pi/agent/bin/
```

The frontend bundle is embedded via `//go:embed all:web/dist`, so `go build` needs
`web/dist` to exist first. `make build` does both steps in order; if you build
by hand, run `npm --prefix web install && npm --prefix web run build` before
`go build`.

## Usage

```bash
# Start on the default port (31415)
pi-web

# Start and open a browser
pi-web -o

# Custom port
pi-web -p 8080

# Override bind host (loopback is unauthenticated by default)
pi-web --host 127.0.0.1

# Non-loopback bind requires a token — pi-web refuses to start otherwise
PI_WEB_TOKEN=$(openssl rand -hex 16) pi-web --host 100.x.y.z
```

By default, pi-web binds to your Tailscale IP when available, otherwise `127.0.0.1`. Any non-loopback bind requires `PI_WEB_TOKEN` to be set; pass `--insecure` to override (don't, on Tailscale).

## Remote access

The default Tailscale bind is the point of the project: leave pi running on your desktop, then drive it from your phone on the couch or your laptop on the train.

```bash
# 1. Start pi-web — it auto-detects your Tailscale IP
PI_WEB_TOKEN=$(openssl rand -hex 16) pi-web

# 2. From any other Tailscale-connected device, open the printed URL
#    and paste the token once. The cookie persists.
```

> By default, pi-web refuses to bind to a non-loopback address unless `PI_WEB_TOKEN` is set — anyone who can reach the bound address could otherwise view sessions and send instructions to pi. To override this guard for local-network testing, pass `--insecure`. **Don't use `--insecure` on Tailscale or any address reachable from outside your machine.**
>
> Clients can pass the token via the `Authorization: Bearer <token>` header, the `X-Pi-Token` header, or once via `?token=<token>` (which sets a `pi_token` cookie for subsequent requests). Tokens passed via `?token=` end up in browser history, server access logs, and `Referer` headers from any links on the page — prefer the header form for anything beyond the initial bookmark.

## Browser chat

Open a session page and use the composer at the bottom to continue that exact session.

- `Enter` sends, `Shift+Enter` inserts a newline
- Drag-and-drop or paste images directly into the composer
- The model picker and thinking-level selector live in the header — changes apply to the underlying pi worker immediately
- Each active session gets its own dedicated `pi --mode rpc` worker, so different sessions don't block each other

## Pi integration

### `/web`, `/mobile`, `/refresh` commands

**Project-local** (auto-discovered when you run `pi` inside this repo):

Already included at `.pi/extensions/pi-web.ts`. Install the extension deps:

```bash
cd .pi/extensions && npm install && cd ../..
```

**Global** (available in all projects):

```bash
mkdir -p ~/.pi/agent/extensions/pi-web
cp .pi/extensions/pi-web.ts ~/.pi/agent/extensions/pi-web/
cat > ~/.pi/agent/extensions/pi-web/package.json <<'EOF'
{
  "name": "pi-web-extension",
  "private": true,
  "dependencies": {
    "qrcode": "^1.5.4"
  }
}
EOF
cd ~/.pi/agent/extensions/pi-web && npm install
```

Restart pi (or run `/reload`), then:

- `/web` — open the current session in your default browser
- `/mobile` — show a QR code for mobile access over Tailscale (auto-installs `qrcode` on first use)
- `/refresh` — pull new messages written from mobile back into the terminal session

The extension automatically installs `qrcode` into `.pi/extensions/node_modules/` — no global npm packages needed.

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

- **Live reload and chat preview.** A `fsnotify` watcher tails the sessions directory and pushes SSE reload events to connected browsers when pi appends to the file. Session pages fetch `/api/session` and append/upsert canonical JSONL entries in place. Browser-started chat also streams best-effort assistant previews over the same SSE connection; the next JSONL reload remains the source-of-truth reconciliation.
- **Per-session workers.** When you send a message from the browser, pi-web spawns a headless `pi --mode rpc` subprocess scoped to that session, switches it to the session file, and forwards your prompt. Subsequent messages reuse the same worker. If the worker crashes it's evicted and replaced on the next request; idle workers are reaped after 30 minutes so long-lived servers don't accumulate processes.
- **Sharing.** Renders a self-contained HTML snapshot and shells out to `gh gist create --public=false`. Snapshots don't live-update.

A single binary, no database, no daemon — just a Go HTTP server reading the same JSONL pi already writes.

## Development

The sessions index and interactive session viewer use Vite-built browser bundles. Rebuild after frontend changes before running the Go server from source:

```bash
make setup   # install frontend deps and download Go modules
make check   # frontend test/build + Go test/vet
make build   # setup if needed, build frontend, then build ./pi-web
```

## License

MIT
