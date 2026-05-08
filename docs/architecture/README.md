# Architecture Documentation

This directory contains the architecture documentation for **pi-web**, a local web viewer for pi coding-agent sessions.

## Documents

| Document | Description |
|----------|-------------|
| [system-overview.md](./system-overview.md) | High-level system architecture, component diagram, and tech stack |
| [backend.md](./backend.md) | Go backend: packages, responsibilities, and key types |
| [frontend.md](./frontend.md) | Frontend architecture: embedded templates, Vite build, and Alpine.js |
| [data-flow.md](./data-flow.md) | Session file format, data model, and storage layout |

## Architecture at a Glance

```
┌─────────────────────────────────────────────────────────────────────┐
│                           Browser                                    │
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────────────────────┐  │
│  │  / (index)  │  │ /session?id │  │      SSE /events            │  │
│  │  Alpine.js  │  │  Embedded   │  │   Live reload + status      │  │
│  │   (Vite)    │  │   HTML/CSS  │  │        updates              │  │
│  └─────────────┘  └─────────────┘  └─────────────────────────────┘  │
└─────────────────────────────────────────────────────────────────────┘
                                  │
                                  ▼ HTTP
┌─────────────────────────────────────────────────────────────────────┐
│                        pi-web HTTP Server                            │
│  ┌────────────┐ ┌────────────┐ ┌────────────┐ ┌──────────────────┐  │
│  │   Auth     │ │  Handlers  │ │   SSE      │ │  File Watcher    │  │
│  │Middleware  │ │  (server)  │ │ (events)   │ │ (fsnotify/poll)  │  │
│  └────────────┘ └────────────┘ └────────────┘ └──────────────────┘  │
│  ┌────────────┐ ┌────────────┐ ┌────────────┐ ┌──────────────────┐  │
│  │  Sessions  │ │  Workers   │ │   RPC      │ │  Share (gh)      │  │
│  │  (cache)   │ │  (manager) │ │  (pi CLI)  │ │  (gist create)   │  │
│  └────────────┘ └────────────┘ └────────────┘ └──────────────────┘  │
└─────────────────────────────────────────────────────────────────────┘
                                  │
                                  ▼ filesystem
┌─────────────────────────────────────────────────────────────────────┐
│                    ~/.pi/agent/sessions/                             │
│         Project dirs  →  JSONL session files                         │
│         (--name--)        (timestamp_uuid.jsonl)                     │
└─────────────────────────────────────────────────────────────────────┘
```

## Key Design Decisions

1. **Read-only session storage**: pi-web reads from `~/.pi/agent/sessions/` but never writes to existing sessions. New sessions can be created via the web UI.

2. **Live updates via SSE**: The browser opens an EventSource connection. The server watches session files via `fsnotify` (with polling fallback) and pushes `reload` events; session pages fetch `/api/session` to reconcile canonical JSONL entries. Browser chat can also receive best-effort `chat-preview` SSE events before JSONL reconciliation.

3. **Chat via RPC workers**: Each session gets a dedicated `pi --mode rpc` subprocess. Workers are cached and reaped after 30 minutes of idle time.

4. **Dual frontend strategy**:
   - **Index page** (`/`): Built with Vite + Alpine.js, served from embedded `web/dist`
   - **Session page** (`/session`): Server-rendered HTML with embedded JS templates (no build step)

5. **Security**: Token-based auth (`PI_WEB_TOKEN`) is required when binding to non-loopback addresses (e.g., Tailscale).
