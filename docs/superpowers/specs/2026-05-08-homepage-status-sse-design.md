# Homepage Running-Status via SSE — Design

## Problem

The homepage renders 300–400 session cards and currently polls
`GET /api/worker-status?id=<id>` once per card every 1.5s. That is roughly
200+ requests per second per open homepage, and gets worse as session count
grows.

A previous attempt (`7b9ff01`, reverted in `ec3daf9`) added "SSE batch status"
but was reverted because it only reflected chat-worker status and missed the
other status sources (terminal sessions writing `session-status/<id>` files,
and the recent-mtime fallback). The new design must keep all three sources
unified so HTTP and SSE views can never diverge again.

## Goal

- Sub-second freshness of running indicators on the homepage
- Eliminate per-card polling: ≤ 1 SSE connection per open page, ~zero idle traffic
- Single source of truth for "is session X running" shared by HTTP and SSE paths

Non-goals: per-session pages, view-only/broken-session badges, status beyond
`running`/not-running on the homepage.

## Architecture

### Single source of truth

A new function `computeRunningStatus(sessionID string) bool` becomes the only
place that decides whether a session is running. It composes the three
existing sources (in priority order):

1. `session-status/<id>` file with `state == "running"` and `updatedAt`
   within `sessionStatusTTL` (10s) — terminal sessions
2. In-process `chatSender.Status(id)` returning `WorkerStateRunning` — chat
   workers
3. `hasRecentSessionActivity(id)` — jsonl mod-time within
   `recentSessionActivityWindow` (3s) fallback

`handleWorkerStatus` is refactored to call `computeRunningStatus`, so HTTP
clients (e.g. session pages) and SSE clients (homepage) cannot diverge.

### One SSE channel for the homepage

The homepage continues to use the existing `/events?id=__all__` connection.
Two new named events are added:

- `status-snapshot` — sent once, immediately after the `:ok` handshake.
  Payload: `{"running": ["id1", "id2", ...]}`
- `status-delta` — sent whenever a session's running state flips.
  Payload: `{"id": "<sessionID>", "running": true|false}`

The existing default-typed messages (`new-session`, `reload`) are unchanged;
the homepage opts into the new events with `addEventListener`.

### Triggers (fan-in) and broadcast (fan-out)

`lastKnown` is a `map[string]struct{}` of currently-running session ids
(absence ≡ not running). All four triggers below call a single
`recomputeAndBroadcastStatus(sessionID)` function that, under
`lastKnownMu`:

1. Computes `now := computeRunningStatus(sessionID)`
2. Computes `was := sessionID ∈ lastKnown`
3. If `now == was`, returns (no broadcast)
4. Otherwise updates `lastKnown` (insert or delete) and broadcasts
   `status-delta {id, running: now}` to all `__all__` subscribers

This guarantees the first-ever recompute of an idle session does **not**
emit a spurious `running: false` delta, and the same id can never be
broadcast twice with the same value back-to-back.

| Trigger | Source | Status |
|---|---|---|
| jsonl write | `recordModTime` in `watcher.go` | exists, add status call after `reload` broadcast |
| `session-status/<id>` write | new fsnotify watch on `session-status/` dir | new |
| chat worker lifecycle | `statusBroadcast` callback wired through `chatSender` | partially exists |
| TTL expiry | 1s ticker that re-checks every `lastKnown[id]==running` entry | new |

The sweeper is what handles "session was running, no further events arrive,
the 3s mtime window or 10s status-file TTL elapses". Without it, idle
transitions would be missed when no fsnotify event fires.

## Components

### New / changed files

- **`internal/server/status.go`** (new)
  - `computeRunningStatus(sessionID string) bool`
  - `lastKnown map[string]struct{}` + `lastKnownMu sync.Mutex` (presence ≡ running)
  - `recomputeAndBroadcastStatus(sessionID string)`
  - `broadcastStatusSnapshot(client *sseClient)` (used by events.go)
- **`internal/server/status_watcher.go`** (new)
  - fsnotify watcher on `<sessionsDir>/../session-status/`. If the directory
    doesn't exist, watch its parent and add it on Create. If fsnotify
    initialization fails, log and rely on the sweeper.
  - On Write or Create of a file inside, calls
    `recomputeAndBroadcastStatus(filename)`.
- **`internal/server/status_sweeper.go`** (new)
  - 1s `time.Ticker`. Each tick: read a snapshot of running ids from
    `lastKnown` under the mutex, then call `recomputeAndBroadcastStatus` on
    each (no lock held during recompute). Idle entries are not swept; they
    only enter `lastKnown` once they become running.
- **`internal/server/chat.go`** (changed)
  - `handleWorkerStatus` calls `computeRunningStatus` for the
    running/not-running decision. Other fields it returns
    (`thinkingLevel`, etc.) are computed alongside but using the same path.
- **`internal/server/watcher.go`** (changed)
  - `recordModTime`: after the existing `broadcast(sessID, "reload")`,
    also call `recomputeAndBroadcastStatus(sessID)`.
- **`internal/server/events.go`** (changed)
  - For `id=__all__` connections, emit a `status-snapshot` event before the
    main loop. Other connection ids are unaffected.
- **`internal/server/server.go`** (changed)
  - Construct and start the status watcher and sweeper from `New(...)`.
  - Route the chat worker `statusBroadcast` callback through
    `recomputeAndBroadcastStatus`.
- **`web/src/index/index.js`** (changed)
  - Remove `refreshRunningStatuses`, `startStatusPolling`,
    `stopStatusPolling`, `_pollTimer`, and the `pollIntervalMs` option.
  - In `subscribe()`, add `es.addEventListener('status-snapshot', ...)` and
    `es.addEventListener('status-delta', ...)` handlers that mutate
    `runningSessionIds` and call `syncRunningCardClasses()`.
  - `es.onmessage` (for `new-session`) is unchanged.
- **`web/src/index/index.test.js`** (changed)
  - Replace fetch-based polling tests with an EventSource fake that
    dispatches `status-snapshot` and `status-delta` and asserts the DOM/state
    updates.

### SSE wire format on `/events?id=__all__`

```
:ok

event: status-snapshot
data: {"running":["id1","id2"]}

event: status-delta
data: {"id":"id1","running":false}

event: new-session
data: ...
```

## Data flow example: terminal session starts

```
external process writes session-status/<id>
  → fsnotify Write event in status_watcher.go
  → recomputeAndBroadcastStatus(id)
    → computeRunningStatus(id) == true
    → lastKnown[id] was false → flip to true
    → broadcast `event: status-delta, data: {id, running:true}` on __all__
  → homepage handler adds id to runningSessionIds, toggles
    `.session-card--running` via syncRunningCardClasses()
```

## Error handling

- **Missing `session-status/` dir:** create it on startup (or watch parent
  and add it on Create). If fsnotify init fails entirely, log and rely on
  the 1s sweeper for correctness — the system stays correct, just slower
  for terminal sessions.
- **`lastKnown` concurrency:** guarded by its own mutex. Any
  read-modify-write is atomic so two concurrent triggers cannot
  double-broadcast.
- **SSE channel saturation:** existing buffer is `chan string` size 4 with
  a non-blocking select; bursts beyond 4 drop deltas. The client recovers
  because the next `recomputeAndBroadcastStatus` for that session re-emits
  the correct edge, and the 1s sweeper backstops any drop.
- **SSE reconnect:** the fresh `status-snapshot` rebuilds the client's
  `runningSessionIds` from scratch. No special diff/version negotiation is
  needed.
- **Unknown id in `status-delta`** (session created mid-page-life and not
  yet in DOM): client-side handler is a no-op; the existing `new-session`
  event triggers a page reload, which then receives a fresh snapshot.

## Testing

- **Unit:** `computeRunningStatus` covering all three sources × running /
  idle. Most cases already exist as `handleWorkerStatus` tests; redirect
  them at the new function.
- **Unit:** `recomputeAndBroadcastStatus` only broadcasts on edge
  transitions, not on every call.
- **Integration:** writing `session-status/<id>` triggers a `status-delta`
  on an `__all__` SSE subscriber.
- **Integration:** sweeper flips a stale running entry to idle within ~1s
  of its underlying source going stale, with no inbound file event.
- **Integration:** an `__all__` subscriber receives a `status-snapshot`
  event immediately after connect.
- **HTTP:** `handleWorkerStatus` continues to return correct
  running/not-running state for all three sources (existing tests, retargeted
  at `computeRunningStatus`).
- **Frontend:** `index.test.js` drives an `EventSource` fake that fires
  `status-snapshot` and `status-delta` events; asserts `runningSessionIds`
  and `.session-card--running` classes update correctly. The fetch-mocking
  paths for polling are removed.

## Edge cases

- **Hidden tab:** SSE keeps streaming; no `visibilityState` gating is
  needed (the polling code's check is removed).
- **Sweeper vs. event race on the same id:** `lastKnown` mutex serializes
  them; only one broadcast goes out.
- **Sweeper cost:** `lastKnown` only contains currently-running ids, so
  even with thousands of total sessions, the sweep set is small (typically
  ≪ total). Each entry is one `computeRunningStatus` call (file stat +
  small reads).
- **Many open homepages:** broadcast is a fan-out over `__all__`
  subscribers; each delta is a small JSON object. Cost scales linearly
  with `len(__all__ clients)`, not with session count.

## Out of scope

- Compressing snapshots or coalescing rapid deltas (defer until measured)
- Reconnect-resilient cursor / sequence numbers (snapshot-on-connect is
  sufficient today)
- Status fields beyond `running` on the homepage
