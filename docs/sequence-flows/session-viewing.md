# Sequence Flow: Viewing a Session

This flow covers a user clicking a session card on the index page (or visiting `/session?id=…` directly).

## Sequence Diagram

```
┌─────────┐   ┌─────────┐   ┌──────────────┐   ┌────────────────┐   ┌─────────────┐   ┌──────────┐
│ Browser │   │  Server │   │   sessions   │   │ generateExport │   │  template   │   │  marked  │
│         │   │         │   │   (lookup)   │   │     Html       │   │   (embed)   │   │ highlight│
└────┬────┘   └────┬────┘   └──────┬───────┘   └───────┬────────┘   └──────┬──────┘   └────┬─────┘
     │             │               │                   │                  │              │
     │ GET /session?id=abc
     │────────────▶│               │                   │                  │              │
     │             │               │                   │                  │              │
     │             │─── ResolveByID(sessionsDir, id) ─▶│                  │              │
     │             │               │                   │                  │              │
     │             │               │─── ReadDir all project dirs          │              │
     │             │               │   find matching filename             │              │
     │             │               │                   │                  │              │
     │             │◀────────────── ResolvedSession ────│                  │              │
     │             │   (Session + Path)                  │                  │              │
     │             │               │                   │                  │              │
     │             │─── ParseFile(path, …) ─────────────▶│                  │              │
     │             │               │                   │                  │              │
     │             │               │─── os.ReadFile    │                  │              │
     │             │               │─── strings.Split("\\n")                │              │
     │             │               │─── json.Unmarshal each line            │              │
     │             │               │─── Count messages/tokens/cost          │              │
     │             │               │─── Check cwd exists                    │              │
     │             │               │                   │                  │              │
     │             │◀────────────── Session struct ─────│                  │              │
     │             │               │                   │                  │              │
     │             │─── generateExportHtml(session, true) ────────────────▶│              │
     │             │               │                   │                  │              │
     │             │               │                   ├─── buildTemplateJsBundle()
     │             │               │                   │   (concat templates/app/*.js)
     │             │               │                   │                  │              │
     │             │               │                   ├─── base64(sessionData)
     │             │               │                   │                  │              │
     │             │               │                   ├─── template.html │              │
     │             │               │                   ├─── template.css  │              │
     │             │               │                   ├─── marked.min.js │              │
     │             │               │                   ├─── highlight.min.js              │
     │             │               │                   └─── chat_composer.html              │
     │             │               │                   │                  │              │
     │             │◀────────────── HTML string ───────│                  │              │
     │             │               │                   │                  │              │
     │◀──────────── Response (text/html) ──────────────│                  │              │
     │             │               │                   │                  │              │
     │             │               │                   │                  │              │
     │ GET /events?id=abc
     │────────────▶│               │                   │                  │              │
     │             │─── addClient(abc)                 │                  │              │
     │             │               │                   │                  │              │
     │◀──────────── SSE stream ────────────────────────│                  │              │
     │             │               │                   │                  │              │
```

## Step-by-Step

### 1. Request Arrives

```
GET /session?id=2026-01-15T10-30-00.000Z_a1b2c3d4.jsonl
```

### 2. Session Resolution

`sessions.ResolveByID` validates and locates the file:

```go
func ResolveByID(sessionsDir, id string) (ResolvedSession, error) {
    // Validate: must be a basename ending in .jsonl
    if id == "" || filepath.Base(id) != id || filepath.Ext(id) != ".jsonl" {
        return ResolvedSession{}, ErrInvalidSessionID
    }
    // Walk all project subdirs to find the file
    path, err := findPathByFilename(sessionsDir, id)
    // …
}
```

Security: `filepath.Base(id) != id` prevents path traversal.

### 3. Parse Session

`sessions.ParseFile` reads and transforms the JSONL file:

1. **Read file** → split by newlines
2. **Unmarshal each line** into `map[string]any`
3. **Categorize**:
   - `type == "session"` → `sess.Header`
   - `type == "message"` → increment `MessageCount`, sum `TokenTotal`/`CostTotal`
   - All lines → `sess.Entries`
4. **Set `LastActivity`** to latest timestamp (or file modtime as fallback)
5. **Check chat availability**: if `cwd` from header no longer exists, disable chat

### 4. Generate HTML

`generateExportHtml` assembles the final page via string replacement:

```go
func generateExportHtml(session sessions.Session, showButtons bool) string {
    // 1. Build session data payload
    sessionData := map[string]any{
        "header":  session.Header,
        "entries": session.Entries,
        "leafId":  lastEntryID,
        // …
    }
    dataBase64 := base64.StdEncoding.EncodeToString(marshal(sessionData))

    // 2. Build CSS with theme variables injected
    css := strings.Replace(templateCss, "{{THEME_VARS}}", precomputedThemeVars, 1)

    // 3. Assemble HTML
    html := templateHtml
    html = strings.Replace(html, "<title>…", "<title>"+sessionName(session)+"</title>", 1)
    html = strings.Replace(html, "{{CSS}}", css, 1)
    html = strings.Replace(html, "{{JS}}", templateJs, 1)
    html = strings.Replace(html, "{{SESSION_DATA}}", dataBase64, 1)
    html = strings.Replace(html, "{{MARKED_JS}}", markedJs, 1)
    html = strings.Replace(html, "{{HIGHLIGHT_JS}}", hljsJs, 1)

    if showButtons {
        html = strings.Replace(html, "<body>", "<body>"+actionButtons, 1)
        html = strings.Replace(html, "{{CHAT_COMPOSER}}", chatComposerHtml, 1)
        html = strings.Replace(html, "</body>", liveReloadJs+"</body>", 1)
    }

    return html
}
```

### 5. SSE Subscription

Immediately after page load, the browser connects to:

```
GET /events?id=2026-01-15T10-30-00.000Z_a1b2c3d4.jsonl
```

The server:
1. Creates an `sseClient` with buffered channel
2. Sends `:ok\n\n` (SSE comment to confirm connection)
3. Blocks reading from `client.ch` or `r.Context().Done()`

When the session file changes, the file watcher calls `broadcast(sessID, "reload")`. The browser fetches `/api/session`, appends new canonical entries, upserts live-rendered entries, and clears any temporary chat preview.

## Caching Behavior

If the same session is viewed multiple times in quick succession, `sessions.Cache` may return a cached `Session` without re-parsing, provided the file's `modTime` hasn't changed.
