# Data Flow & Session File Format

## Session File Format

Sessions are stored as **JSONL** files (one JSON object per line):

```
~/.pi/agent/sessions/--project-name--/
└── 2026-01-15T10-30-00.000Z_a1b2c3d4.jsonl
```

### Example JSONL Content

```jsonl
{"type":"session","version":3,"id":"uuid","timestamp":"2026-01-15T10:30:00Z","cwd":"/Users/me/project","name":"My Session"}
{"type":"message","timestamp":"2026-01-15T10:30:01Z","message":{"role":"user","content":"Hello"}}
{"type":"message","timestamp":"2026-01-15T10:30:05Z","message":{"role":"assistant","content":"Hi!"},"usage":{"totalTokens":42,"cost":{"total":0.0001}}}
{"type":"tool_call","timestamp":"2026-01-15T10:30:06Z","tool":"bash","command":"ls -la"}
{"type":"tool_result","timestamp":"2026-01-15T10:30:07Z","tool":"bash","output":"..."}
{"type":"branch_summary","timestamp":"2026-01-15T10:35:00Z","branch":"main","summary":"..."}
{"type":"compaction","timestamp":"2026-01-15T10:40:00Z","before":"...","after":"..."}
```

### Entry Types

| `type` | Description |
|--------|-------------|
| `session` | Header metadata (cwd, name, version, id) |
| `message` | User or assistant message with optional `usage` and `cost` |
| `tool_call` | Agent invoked a tool |
| `tool_result` | Tool execution result |
| `bash` / `bash_output` | Shell command and its output |
| `branch_summary` | Summary of work on a git branch |
| `compaction` | Conversation history was compacted |
| `model_change` | Model switched mid-session |

### Project Directory Encoding

Project names are filesystem-safe encoded:

```go
EncodeProjectName("/Users/me/project") → "--Users-me-project--"
DecodeProjectName("--Users-me-project--") → "/Users/me/project"
```

## Parse Flow

```
File on disk
     │
     ▼
sessions.ParseFile(path, dirName, fileName)
     │
     ├──▶ os.ReadFile → string
     │
     ├──▶ strings.Split by "\n"
     │
     ├──▶ json.Unmarshal each line
     │        ├──▶ type=="session" → sess.Header
     │        ├──▶ type=="message" → increment MessageCount, sum tokens/cost
     │        └──▶ all types → append to Entries
     │
     ├──▶ LastActivity = latest timestamp (or file modtime fallback)
     │
     └──▶ ChatAvailable = cwd still exists?
```

## Cache Strategy

`sessions.Cache` avoids re-parsing unchanged files:

```
LoadAll(dir)
    │
    ├──▶ ReadDir all project subdirs
    │
    ├──▶ For each .jsonl file:
    │         ├──▶ Check modtime against cache
    │         ├──▶ MATCH → return cached Session
    │         └──▶ MISMATCH → ParseFile + store in cache
    │
    ├──▶ Evict files no longer on disk
    │
    └──▶ SortByActivity (descending by timestamp)
```

## Data Flow: Viewing a Session

```
Browser GET /session?id=<id>
           │
           ▼
    server.handleSession
           │
           ├──▶ sessions.ResolveByID → find file path
           │         └──▶ walk project dirs, match filename
           │
           ├──▶ sessions.ParseFile → Session struct
           │
           ├──▶ generateExportHtml(session, true)
           │         ├──▶ marshal session data → base64
           │         ├──▶ inject CSS/JS/templates
           │         └──▶ inject chat composer HTML
           │
           └──▶ Write HTML response
```

## Data Flow: Chat Message

```
Browser POST /api/chat?id=<id>
           │
           ▼
    server.handleChat
           │
           ├──▶ sessions.ResolveByID → Session + Path
           │
           ├──▶ chat.ParseRequest(r)
           │         ├──▶ ParseMultipartForm
           │         ├──▶ Extract text + image files
           │         └──▶ Validate (not empty, image size, mime type)
           │
           ├──▶ workers.Manager.Send(ctx, sessionID, sessionPath, chatReq)
           │         │
           │         ├──▶ Get or create ChatWorker for session
           │         │         └──▶ rpc.NewPiWorker(sessionPath)
           │         │               ├──▶ exec.Command("pi", "--mode", "rpc")
           │         │               ├──▶ Start subprocess
           │         │               ├──▶ switch_session RPC
           │         │               └──▶ Background goroutines: consume stdout, wait
           │         │
           │         └──▶ worker.Prompt(ctx, chatReq)
           │               ├──▶ BuildPromptCommand (JSONL to stdin)
           │               ├──▶ Await response on pending channel
           │               └──▶ Update status → running
           │
           └──▶ Return {"ok": true, "status": "accepted"}
```

## Data Flow: Live Reload

```
Editor saves session file
           │
           ▼
    fsnotify detects Write event
           │
           ▼
    debouncer.schedule(path)  (50ms debounce)
           │
           ▼
    Server.recordModTime(sessID, modTime)
           │
           ├──▶ Update fileMod map
           ├──▶ Broadcast "reload" to SSE clients for this sessID
           └──▶ Recompute running status → broadcast status-delta
           │
           ▼
    Browser EventSource receives "reload"
           └──▶ fetch /api/session
                └──▶ append/upsert canonical entries and clear preview
```

## Data Flow: Share to Gist

```
Browser POST /share?id=<id>
           │
           ▼
    server.handleShare
           │
           ├──▶ share.FindGh → locate `gh` CLI
           │
           ├──▶ gh auth status → verify login
           │
           ├──▶ loadSessions → find matching session
           │
           ├──▶ generateExportHtml(session, false)  (no buttons)
           │
           ├──▶ Write to temp file
           │
           ├──▶ gh gist create --public=false <tmpfile>
           │
           └──▶ Return {gistUrl, gistId, previewUrl}
```
