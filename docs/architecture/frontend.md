# Frontend Architecture

pi-web uses **two different frontend strategies** for its two main pages.

## 1. Index Page (`/`)

Built with **Vite** + **Alpine.js**, embedded into the Go binary.

### Build Pipeline

```
web/src/index/index.js    ──┐
web/src/shared/api.js       ├──▶  vite build  ──▶  web/dist/  ──▶  //go:embed
web/src/shared/storage.js   │                      │              (dist_embed.go)
web/src/live/live.js        │                      │
                            │                      ▼
                        .vite/manifest.json    assets/index-*.js
```

At startup, `dist_embed.go` reads `.vite/manifest.json` to find the hashed JS filename and registers it as a route.

### Runtime

The index page (`templates/index.html`) is rendered by `indexTmpl` with these helpers:

- `fmtTime` — format RFC3339 to human-readable
- `fmtTokens` — abbreviate large numbers (1.2k, 3.4M)
- `fmtCost` — format cost as `$0.0012` or `—`
- `sessionName` — derive display name from header or first user message
- `indexScript` — inject the correct Vite bundle path

### Alpine.js App (`createSessionsPage`)

Located in `web/src/index/index.js`:

- **Search**: Filters session cards by `data-search` attribute; hides empty project groups
- **New Session Modal**: Prompts for path, calls `POST /api/new-session`, redirects to new session
- **Live Status**: Subscribes to `/events?id=__all__` and toggles `.session-card--running` class based on `status-snapshot` and `status-delta` events
- **Auto-reload**: Full page reload on `new-session` SSE event

## 2. Session Page (`/session?id=…`)

Server-rendered **embedded HTML** with no build step. Uses string replacement to inject CSS, JS, and data into `templates/template.html`.

### Rendering Pipeline

```
sessions.Session
       │
       ▼
generateExportHtml(session, showButtons=true)
       │
       ├──▶ template.html  (base HTML shell)
       ├──▶ template.css   (dark theme, with {{THEME_VARS}} replaced)
       ├──▶ templateJs     (concatenated templates/app/*.js)
       ├──▶ base64(sessionData)  (injected as global data)
       ├──▶ marked.min.js  (Markdown → HTML)
       ├──▶ highlight.min.js (syntax highlighting)
       ├──▶ chat_composer.html (chat input UI)
       └──▶ live_reload.js (SSE client for this session)
```

### Embedded JS Templates (`templates/app/*.js`)

These are concatenated in lexical order (numeric prefix controls load order):

| File | Purpose |
|------|---------|
| `00-data.js` | Parse base64 session data, expose global `sessionData` |
| `10-tree.js` | Build entry tree from flat entries (branches, compactions) |
| `20-filter.js` | Filter entries by type, search, branch |
| `30-format.js` | Format timestamps, tokens, costs, diff highlights |
| `40-render-tree.js` | Render the sidebar tree navigation |
| `50-render-entry.js` | Render individual entries (messages, tool calls, bash, etc.) |
| `60-header.js` | Render session header (model, tokens, cost, timestamp) |
| `70-navigation.js` | Keyboard shortcuts, scroll-to-entry, permalink handling |
| `80-ui.js` | UI state: dark/light mode, sidebar toggle, mobile layout |
| `90-chat.js` | Chat composer, model selector, thinking-level selector, send/receive |

### Live Reload for Session Page

The session page opens its own SSE connection to `/events?id=<sessionId>`:

- On `reload` event → fetch `/api/session?id=…`, append/upsert canonical entries, clear any temporary chat preview
- On `chat-preview` event → render/update a temporary assistant preview block until canonical JSONL reload arrives
- Debounced in the server watcher to avoid multiple reloads per save

## Shared Frontend Modules

### `web/src/shared/api.js`

```js
getJSON(url)   → fetch + parse + throw on error
postJSON(url, body) → POST with JSON body
```

Used by index page for: `/api/recent-locations`, `/api/new-session`

### `web/src/shared/storage.js`

LocalStorage helpers for persisting UI preferences (theme, sidebar state, etc.)

### `web/src/shared/escape.js`

HTML escape utility for safely rendering user content.

## Static Assets

| Asset | Source | Served From |
|-------|--------|-------------|
| Vite index bundle | `web/dist/assets/index-*.js` | `/static/assets/index-*.js` |
| Alpine.js | `templates/vendor/alpine.min.js` | `/static/alpine.js` |
| marked.js | `templates/vendor/marked.min.js` | inline in session HTML |
| highlight.js | `templates/vendor/highlight.min.js` | inline in session HTML |

## Theme System

Colors are defined in `computeThemeVars()` in `export.go` as CSS custom properties. The session page injects them into `:root`.

Key color tokens:
- `--cyan`, `--blue`, `--green`, `--red`, `--yellow` — semantic colors
- `--userMessageBg` — user chat bubble
- `--toolSuccessBg` / `--toolErrorBg` — tool result states
- `--thinkingLow` → `--thinkingXhigh` — thinking level indicator gradient
