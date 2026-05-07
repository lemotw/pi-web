# Homepage Running-Status via SSE Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace per-card polling of `/api/worker-status` (300+ req/s on a 300-card homepage) with a single SSE connection that pushes initial running-set + per-session deltas. Keep HTTP and SSE views perfectly consistent by routing both through one `computeRunningStatus` function so terminal sessions, chat workers, and recent-mtime fallback can never diverge again.

**Architecture:** A new `computeRunningStatus(sessionID) bool` becomes the single source of truth for "running" on the server. A new `recomputeAndBroadcastStatus(sessionID)` recomputes and pushes a delta whenever any of three triggers fires: jsonl write (existing fsnotify), `session-status/` directory write (new fsnotify watch), or 1s TTL sweep (new goroutine, backstops time-based transitions and chat-worker stops). The homepage subscribes to the existing `/events?id=__all__` connection, which now also emits a `status-snapshot` event on connect and `status-delta` events on every flip. The client mutates an in-memory running-set and toggles a CSS class — no fetches, no polling.

**Tech Stack:** Go 1.22, fsnotify, SSE; vanilla JS + Vitest on the frontend.

---

## File Structure

| File | Responsibility |
|---|---|
| `internal/server/status.go` | New. `computeRunningStatus`, `lastKnown`, `recomputeAndBroadcastStatus`, snapshot helper. |
| `internal/server/status_watcher.go` | New. fsnotify watch on `session-status/` dir. |
| `internal/server/status_sweeper.go` | New. 1s ticker that re-evaluates entries currently marked running. |
| `internal/server/chat.go` | Refactor `handleWorkerStatus` to use `computeRunningStatus`. |
| `internal/server/watcher.go` | After `recordModTime` broadcasts `reload`, call `recomputeAndBroadcastStatus`. |
| `internal/server/events.go` | On `__all__` connects, emit `status-snapshot` after `:ok`. |
| `internal/server/server.go` | Hold the status-watcher state; start watcher + sweeper from `New`. |
| `internal/server/status_test.go` | New. Unit tests for `computeRunningStatus` + `recomputeAndBroadcastStatus`. |
| `internal/server/status_watcher_test.go` | New. End-to-end test that a `session-status/<id>` write triggers a delta. |
| `internal/server/status_sweeper_test.go` | New. Sweeper flips stale running entries to idle. |
| `internal/server/events_test.go` | New. `status-snapshot` is sent on `__all__` connect. |
| `web/src/index/index.js` | Remove polling; consume SSE `status-snapshot` and `status-delta`. |
| `web/src/index/index.test.js` | Replace fetch-based tests with EventSource fake driving SSE events. |

---

## Conventions used in this plan

- **Working directory:** `/Users/setkyar/pi-web`. All commands assume this CWD.
- **Build/test commands:**
  - Go tests for one package: `go test ./internal/server/ -run <Name> -v`
  - Go tests for the package: `go test ./internal/server/`
  - Frontend tests: `cd web && npx vitest run src/index/index.test.js`
  - Frontend full: `cd web && npx vitest run`
- **TDD discipline:** Each task starts with a failing test (run it, confirm the failure mode), then implements, then re-runs and confirms PASS. Commit per task.
- **No chat-worker lifecycle callback in this plan.** The reverted attempt added one and broke correctness in subtle ways; the jsonl watcher already detects chat-worker activity via mtime, and the 1s sweeper handles termination. We can revisit if a measurement shows a real gap.

---

## Task 1: Extract `computeRunningStatus` from `handleWorkerStatus`

Pure refactor. Establishes the single source of truth that subsequent tasks reuse.

**Files:**
- Create: `internal/server/status.go`
- Modify: `internal/server/chat.go` (lines 106–126: `handleWorkerStatus`)
- Create: `internal/server/status_test.go`

- [ ] **Step 1: Write the failing test**

Append to a new file `internal/server/status_test.go`:

```go
package server

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"pi-web/internal/workers"
)

func TestComputeRunningStatusFromStatusFile(t *testing.T) {
	root := t.TempDir()
	sessionsDir := filepath.Join(root, "sessions")
	statusDir := filepath.Join(root, "session-status")
	if err := os.MkdirAll(sessionsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(statusDir, 0o755); err != nil {
		t.Fatal(err)
	}
	payload, _ := json.Marshal(sessionStatusFile{State: "running", UpdatedAt: time.Now().UTC().Format(time.RFC3339)})
	if err := os.WriteFile(filepath.Join(statusDir, "session.jsonl"), payload, 0o644); err != nil {
		t.Fatal(err)
	}

	s := &Server{sessionsDir: sessionsDir, chatSender: &fakeSender{}}
	if !s.computeRunningStatus("session.jsonl") {
		t.Fatalf("expected running=true from session-status file")
	}
}

func TestComputeRunningStatusFromChatSender(t *testing.T) {
	s := &Server{
		sessionsDir: t.TempDir(),
		chatSender:  &fakeSender{status: workers.WorkerStatus{State: workers.WorkerStateRunning}},
	}
	if !s.computeRunningStatus("session.jsonl") {
		t.Fatalf("expected running=true from chatSender")
	}
}

func TestComputeRunningStatusFromRecentMtime(t *testing.T) {
	now := time.Date(2026, 5, 8, 12, 0, 0, 0, time.UTC)
	s := &Server{
		sessionsDir: t.TempDir(),
		chatSender:  &fakeSender{},
		fileMod:     map[string]time.Time{"session.jsonl": now.Add(-1 * time.Second)},
		now:         func() time.Time { return now },
	}
	if !s.computeRunningStatus("session.jsonl") {
		t.Fatalf("expected running=true from recent mtime")
	}
}

func TestComputeRunningStatusIdleByDefault(t *testing.T) {
	s := &Server{sessionsDir: t.TempDir(), chatSender: &fakeSender{}}
	if s.computeRunningStatus("session.jsonl") {
		t.Fatalf("expected running=false by default")
	}
}

func TestComputeRunningStatusEmptyID(t *testing.T) {
	s := &Server{sessionsDir: t.TempDir(), chatSender: &fakeSender{}}
	if s.computeRunningStatus("") {
		t.Fatalf("empty id must be idle")
	}
}
```

- [ ] **Step 2: Run the test, confirm it fails**

Run: `go test ./internal/server/ -run TestComputeRunningStatus -v`

Expected: compile error — `s.computeRunningStatus undefined`. Good.

- [ ] **Step 3: Create `internal/server/status.go` with the function**

```go
package server

import "pi-web/internal/workers"

// computeRunningStatus is the single source of truth for "is this session
// running right now". Both the HTTP handler (handleWorkerStatus) and the SSE
// broadcaster (recomputeAndBroadcastStatus) call this; that is what keeps
// terminal sessions, chat workers, and the recent-activity fallback from
// drifting apart.
//
// Order matches the historical behaviour of handleWorkerStatus:
//  1. session-status/<id> file (terminal sessions)
//  2. in-process chat worker status
//  3. recent jsonl mtime within recentSessionActivityWindow
func (s *Server) computeRunningStatus(sessionID string) bool {
	if sessionID == "" {
		return false
	}
	if status := s.readSessionStatus(sessionID); status != nil && status.State == workers.WorkerStateRunning {
		return true
	}
	if s.chatSender != nil && s.chatSender.Status(sessionID).State == workers.WorkerStateRunning {
		return true
	}
	return s.hasRecentSessionActivity(sessionID)
}
```

- [ ] **Step 4: Refactor `handleWorkerStatus` to use it**

Modify `internal/server/chat.go` lines 106–126. Replace the body of `handleWorkerStatus` with:

```go
func (s *Server) handleWorkerStatus(w http.ResponseWriter, r *http.Request) {
	sessionID := r.URL.Query().Get("id")

	status := workers.WorkerStatus{State: workers.WorkerStateIdle}
	if s.computeRunningStatus(sessionID) {
		status.State = workers.WorkerStateRunning
	} else if s.chatSender != nil {
		// Preserve the existing behaviour: when not running, fetch GetState
		// to populate ThinkingLevel for the session page.
		if state, err := s.chatSender.GetState(r.Context(), sessionID); err == nil {
			status.ThinkingLevel = state.ThinkingLevel
		}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(status)
}
```

- [ ] **Step 5: Run all server tests**

Run: `go test ./internal/server/`

Expected: PASS — both the new `TestComputeRunningStatus*` cases and the existing `TestHandleWorkerStatus*` cases. If `TestHandleWorkerStatusSkipsGetStateWhenLocalStatusRunning` still passes (it asserts `getStateCalls == 0`), the refactor preserved its short-circuit behaviour.

- [ ] **Step 6: Commit**

```bash
git add internal/server/status.go internal/server/status_test.go internal/server/chat.go
git commit -m "refactor: extract computeRunningStatus, route handler through it"
```

---

## Task 2: Add `lastKnown` state and `recomputeAndBroadcastStatus`

Adds the broadcaster but does not yet wire any triggers into it.

**Files:**
- Modify: `internal/server/server.go` (add fields to `Server` struct around lines 41–55, init in `New` around lines 62–73)
- Modify: `internal/server/status.go`
- Modify: `internal/server/status_test.go`

- [ ] **Step 1: Write the failing test**

Append to `internal/server/status_test.go`:

```go
func TestRecomputeAndBroadcastStatusEmitsDeltaOnFlip(t *testing.T) {
	now := time.Date(2026, 5, 8, 12, 0, 0, 0, time.UTC)
	s := &Server{
		sessionsDir: t.TempDir(),
		chatSender:  &fakeSender{},
		clients:     make([]*sseClient, 0),
		lastKnown:   make(map[string]struct{}),
		fileMod:     map[string]time.Time{"a.jsonl": now.Add(-1 * time.Second)},
		now:         func() time.Time { return now },
	}
	c := s.addClient(globalSessID)
	defer s.removeClient(c)

	s.recomputeAndBroadcastStatus("a.jsonl")

	select {
	case msg := <-c.ch:
		want := `event: status-delta\ndata: {"id":"a.jsonl","running":true}`
		if msg != want {
			t.Fatalf("msg = %q want %q", msg, want)
		}
	case <-time.After(time.Second):
		t.Fatalf("expected status-delta broadcast")
	}
}

func TestRecomputeAndBroadcastStatusNoBroadcastWhenUnchanged(t *testing.T) {
	s := &Server{
		sessionsDir: t.TempDir(),
		chatSender:  &fakeSender{},
		clients:     make([]*sseClient, 0),
		lastKnown:   make(map[string]struct{}),
	}
	c := s.addClient(globalSessID)
	defer s.removeClient(c)

	// First call on an idle session: idle was never recorded, computeRunning
	// returns false → was==false, now==false → no broadcast.
	s.recomputeAndBroadcastStatus("a.jsonl")

	select {
	case msg := <-c.ch:
		t.Fatalf("unexpected broadcast: %q", msg)
	case <-time.After(50 * time.Millisecond):
	}
}

func TestRecomputeAndBroadcastStatusFlipsBackToIdle(t *testing.T) {
	s := &Server{
		sessionsDir: t.TempDir(),
		chatSender:  &fakeSender{},
		clients:     make([]*sseClient, 0),
		lastKnown:   map[string]struct{}{"a.jsonl": {}},
	}
	c := s.addClient(globalSessID)
	defer s.removeClient(c)

	s.recomputeAndBroadcastStatus("a.jsonl")

	select {
	case msg := <-c.ch:
		want := `event: status-delta\ndata: {"id":"a.jsonl","running":false}`
		if msg != want {
			t.Fatalf("msg = %q want %q", msg, want)
		}
	case <-time.After(time.Second):
		t.Fatalf("expected idle delta")
	}
	if _, ok := s.lastKnown["a.jsonl"]; ok {
		t.Fatalf("lastKnown should no longer contain a.jsonl")
	}
}
```

The escaped `\n` in the `want` strings above are real newline characters in the SSE wire format. Replace the literal `\n` with actual newlines in the test source. The exact body is shown in step 3.

- [ ] **Step 2: Run the test, confirm it fails**

Run: `go test ./internal/server/ -run TestRecomputeAndBroadcastStatus -v`

Expected: compile error — `s.recomputeAndBroadcastStatus undefined`, `s.lastKnown undefined`. Good.

- [ ] **Step 3: Add fields to `Server` and the broadcaster function**

Modify `internal/server/server.go`. In the `Server` struct (around line 41), add:

```go
	lastKnown     map[string]struct{} // ids currently broadcast as running
	lastKnownMu   sync.Mutex
```

In `New(...)` around line 62, add to the literal:

```go
		lastKnown: make(map[string]struct{}),
```

Modify `internal/server/status.go`, append:

```go
import (
	"fmt"
	// keep existing imports
)

// recomputeAndBroadcastStatus recomputes the running state for sessionID and,
// if it changed since the last broadcast, sends a status-delta SSE event to
// every __all__ subscriber.
//
// `lastKnown` is the set of session ids currently broadcast as running.
// Absence == idle. We only emit when (now == running) != (id ∈ lastKnown).
// First-touch idle is therefore silent (no spurious running:false flood when
// the sweeper rescans).
func (s *Server) recomputeAndBroadcastStatus(sessionID string) {
	if sessionID == "" {
		return
	}
	now := s.computeRunningStatus(sessionID)

	s.lastKnownMu.Lock()
	_, was := s.lastKnown[sessionID]
	if now == was {
		s.lastKnownMu.Unlock()
		return
	}
	if now {
		s.lastKnown[sessionID] = struct{}{}
	} else {
		delete(s.lastKnown, sessionID)
	}
	s.lastKnownMu.Unlock()

	payload := fmt.Sprintf(`{"id":%q,"running":%t}`, sessionID, now)
	s.broadcast(globalSessID, "event: status-delta\ndata: "+payload)
}
```

(Adjust the imports of `internal/server/status.go` to include `"fmt"`.)

- [ ] **Step 4: Run the new tests, expect PASS**

Run: `go test ./internal/server/ -run TestRecomputeAndBroadcastStatus -v`

Expected: PASS. Then run the whole package: `go test ./internal/server/`. Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/server/server.go internal/server/status.go internal/server/status_test.go
git commit -m "feat: add recomputeAndBroadcastStatus and lastKnown set"
```

---

## Task 3: Update `events.go` to send `status-snapshot` and to deliver named events

The current `handleEvents` writes plain `data: <msg>\n\n` for everything. We need it to pass through messages that already start with `event: `, and to send a snapshot for `__all__` clients.

**Files:**
- Modify: `internal/server/events.go`
- Modify: `internal/server/server.go` (the `broadcast` writer; specifically the existing `\"data: %s\\n\\n\"` format in `handleEvents`)
- Create: `internal/server/events_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/server/events_test.go`:

```go
package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestHandleEventsSendsStatusSnapshotForAllSubscribers(t *testing.T) {
	s := &Server{
		sessionsDir: t.TempDir(),
		chatSender:  &fakeSender{},
		clients:     make([]*sseClient, 0),
		lastKnown:   map[string]struct{}{"a.jsonl": {}, "b.jsonl": {}},
	}

	req := httptest.NewRequest(http.MethodGet, "/events?id=__all__", nil)
	ctx, cancel := context.WithCancel(req.Context())
	req = req.WithContext(ctx)
	w := httptest.NewRecorder()

	done := make(chan struct{})
	go func() {
		s.handleEvents(w, req)
		close(done)
	}()

	// Wait briefly for the snapshot to be written, then close.
	time.Sleep(50 * time.Millisecond)
	cancel()
	<-done

	body := w.Body.String()
	if !strings.Contains(body, "event: status-snapshot") {
		t.Fatalf("missing snapshot event header in body:\n%s", body)
	}
	if !strings.Contains(body, `"a.jsonl"`) || !strings.Contains(body, `"b.jsonl"`) {
		t.Fatalf("snapshot did not include both ids:\n%s", body)
	}
}

func TestHandleEventsForwardsNamedDeltaEvents(t *testing.T) {
	s := &Server{
		sessionsDir: t.TempDir(),
		chatSender:  &fakeSender{},
		clients:     make([]*sseClient, 0),
		lastKnown:   make(map[string]struct{}),
	}

	req := httptest.NewRequest(http.MethodGet, "/events?id=__all__", nil)
	ctx, cancel := context.WithCancel(req.Context())
	req = req.WithContext(ctx)
	w := httptest.NewRecorder()

	done := make(chan struct{})
	go func() {
		s.handleEvents(w, req)
		close(done)
	}()

	// Wait for snapshot, then push a delta and a legacy reload.
	time.Sleep(50 * time.Millisecond)
	s.broadcast(globalSessID, "event: status-delta\ndata: {\"id\":\"x\",\"running\":true}")
	s.broadcast(globalSessID, "new-session")
	time.Sleep(50 * time.Millisecond)
	cancel()
	<-done

	body := w.Body.String()
	if !strings.Contains(body, "event: status-delta\ndata: {\"id\":\"x\",\"running\":true}") {
		t.Fatalf("expected named delta passthrough, got:\n%s", body)
	}
	if !strings.Contains(body, "data: new-session") {
		t.Fatalf("expected legacy plain-data passthrough, got:\n%s", body)
	}
}
```

- [ ] **Step 2: Run the test, confirm failure**

Run: `go test ./internal/server/ -run TestHandleEvents -v`

Expected: FAIL — current `handleEvents` always wraps every msg as `data: %s\n\n`, so the named delta is mangled and no snapshot is emitted.

- [ ] **Step 3: Update `handleEvents` to send snapshot and pass named events through**

Replace the body of `internal/server/events.go` with:

```go
package server

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
)

func (s *Server) handleEvents(w http.ResponseWriter, r *http.Request) {
	sessID := r.URL.Query().Get("id")
	if sessID == "" {
		http.Error(w, "missing id", 400)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	client := s.addClient(sessID)
	defer s.removeClient(client)

	fmt.Fprintf(w, ":ok\n\n")
	flusher.Flush()

	if sessID == globalSessID {
		s.writeStatusSnapshot(w)
		flusher.Flush()
	}

	for {
		select {
		case msg, open := <-client.ch:
			if !open {
				return
			}
			if strings.HasPrefix(msg, "event: ") {
				// Already-formatted named SSE event; pass through with the
				// terminating blank line.
				fmt.Fprint(w, msg+"\n\n")
			} else {
				fmt.Fprintf(w, "data: %s\n\n", msg)
			}
			flusher.Flush()
		case <-r.Context().Done():
			return
		}
	}
}

// writeStatusSnapshot emits a single SSE event listing every session id that
// is currently broadcast as running. Sorted for deterministic test output.
func (s *Server) writeStatusSnapshot(w http.ResponseWriter) {
	s.lastKnownMu.Lock()
	ids := make([]string, 0, len(s.lastKnown))
	for id := range s.lastKnown {
		ids = append(ids, id)
	}
	s.lastKnownMu.Unlock()
	sort.Strings(ids)

	var sb strings.Builder
	sb.WriteString(`{"running":[`)
	for i, id := range ids {
		if i > 0 {
			sb.WriteByte(',')
		}
		fmt.Fprintf(&sb, "%q", id)
	}
	sb.WriteString("]}")

	fmt.Fprintf(w, "event: status-snapshot\ndata: %s\n\n", sb.String())
}
```

- [ ] **Step 4: Run the events tests, expect PASS**

Run: `go test ./internal/server/ -run TestHandleEvents -v`

Expected: PASS.

- [ ] **Step 5: Run the whole package**

Run: `go test ./internal/server/`

Expected: PASS. The existing `watcher_test.go` `drainBroadcast` only checks that *something* arrived, so the new format does not break it.

- [ ] **Step 6: Commit**

```bash
git add internal/server/events.go internal/server/events_test.go
git commit -m "feat: send status-snapshot on __all__ connect; pass named SSE events through"
```

---

## Task 4: Wire jsonl watcher to call `recomputeAndBroadcastStatus`

The cheapest trigger to wire — `recordModTime` already detects every jsonl write.

**Files:**
- Modify: `internal/server/watcher.go` (function `recordModTime`, lines 65–73)
- Modify: `internal/server/watcher_test.go`

- [ ] **Step 1: Write the failing test**

Add `"strings"` to the imports of `internal/server/watcher_test.go`, then append:

```go
func TestRecordModTimeBroadcastsStatusDelta(t *testing.T) {
	root := t.TempDir()
	now := time.Now()
	s := &Server{
		sessionsDir: root,
		fileMod:     map[string]time.Time{"session.jsonl": now.Add(-10 * time.Second)},
		clients:     make([]*sseClient, 0),
		lastKnown:   make(map[string]struct{}),
		chatSender:  &fakeSender{},
		now:         time.Now,
	}
	c := s.addClient(globalSessID)
	defer s.removeClient(c)

	// Advance modtime to "now"; this is a recent-activity flip from idle to running.
	s.recordModTime("session.jsonl", time.Now())

	// __all__ subscriber should receive a status-delta. (recordModTime also
	// broadcasts "reload" but to sessID="session.jsonl", a different topic.)
	select {
	case msg := <-c.ch:
		if !strings.Contains(msg, "status-delta") || !strings.Contains(msg, "session.jsonl") || !strings.Contains(msg, "true") {
			t.Fatalf("unexpected first msg: %q", msg)
		}
	case <-time.After(time.Second):
		t.Fatalf("expected status-delta on __all__")
	}
}
```

- [ ] **Step 2: Run the test, confirm failure**

Run: `go test ./internal/server/ -run TestRecordModTimeBroadcastsStatusDelta -v`

Expected: FAIL — only the existing `reload` is broadcast, and it goes to a different sessID, so the `__all__` client receives nothing.

- [ ] **Step 3: Update `recordModTime` to also broadcast a status delta**

Modify `internal/server/watcher.go` lines 65–73. Replace `recordModTime` with:

```go
func (s *Server) recordModTime(sessID string, mod time.Time) {
	s.fileModMu.Lock()
	lastMod, known := s.fileMod[sessID]
	s.fileMod[sessID] = mod
	s.fileModMu.Unlock()
	if known && mod.After(lastMod) {
		s.broadcast(sessID, "reload")
	}
	// Always recompute status for this session — the running state depends
	// on the live mtime regardless of whether reload was emitted (e.g. the
	// first observation of a brand-new session file).
	s.recomputeAndBroadcastStatus(sessID)
}
```

- [ ] **Step 4: Run the new test, expect PASS**

Run: `go test ./internal/server/ -run TestRecordModTimeBroadcastsStatusDelta -v`

Expected: PASS.

- [ ] **Step 5: Run the whole package**

Run: `go test ./internal/server/`

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/server/watcher.go internal/server/watcher_test.go
git commit -m "feat: emit status-delta when jsonl mtime advances"
```

---

## Task 5: Watch the `session-status/` directory with fsnotify

Picks up terminal-session status changes — the source the previous attempt missed.

**Files:**
- Create: `internal/server/status_watcher.go`
- Create: `internal/server/status_watcher_test.go`
- Modify: `internal/server/server.go` (`New(...)` to start the watcher)

- [ ] **Step 1: Write the failing test**

Create `internal/server/status_watcher_test.go`:

```go
package server

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSessionStatusWatcherEmitsDelta(t *testing.T) {
	root := t.TempDir()
	sessionsDir := filepath.Join(root, "sessions")
	statusDir := filepath.Join(root, "session-status")
	if err := os.MkdirAll(sessionsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(statusDir, 0o755); err != nil {
		t.Fatal(err)
	}

	s := &Server{
		sessionsDir: sessionsDir,
		clients:     make([]*sseClient, 0),
		lastKnown:   make(map[string]struct{}),
		chatSender:  &fakeSender{},
		now:         time.Now,
	}
	if err := s.startSessionStatusWatcher(); err != nil {
		t.Skipf("fsnotify unavailable: %v", err)
	}

	c := s.addClient(globalSessID)
	defer s.removeClient(c)

	time.Sleep(20 * time.Millisecond)

	payload, _ := json.Marshal(sessionStatusFile{
		State:     "running",
		UpdatedAt: time.Now().UTC().Format(time.RFC3339),
	})
	if err := os.WriteFile(filepath.Join(statusDir, "term.jsonl"), payload, 0o644); err != nil {
		t.Fatal(err)
	}

	select {
	case msg := <-c.ch:
		if !strings.Contains(msg, "status-delta") || !strings.Contains(msg, "term.jsonl") || !strings.Contains(msg, "true") {
			t.Fatalf("unexpected msg: %q", msg)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("expected status-delta after status-file write")
	}
}
```

- [ ] **Step 2: Run the test, confirm failure**

Run: `go test ./internal/server/ -run TestSessionStatusWatcherEmitsDelta -v`

Expected: compile error — `s.startSessionStatusWatcher undefined`.

- [ ] **Step 3: Implement the watcher**

Create `internal/server/status_watcher.go`:

```go
package server

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/fsnotify/fsnotify"
)

// sessionStatusDir returns the directory that terminal sessions write
// status files into. Mirrors readSessionStatus's path computation so a
// single change keeps both callers consistent.
func (s *Server) sessionStatusDir() string {
	return filepath.Join(s.sessionsDir, "..", "session-status")
}

// startSessionStatusWatcher watches the session-status/ directory for
// writes/creates. On every event it triggers a status recompute for the
// affected session id (file basename). Returns an error if fsnotify cannot
// be initialised; callers may choose to log and continue (the 1s sweeper
// is a sufficient correctness backstop).
func (s *Server) startSessionStatusWatcher() error {
	dir := s.sessionStatusDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("ensure session-status dir: %w", err)
	}
	w, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}
	if err := w.Add(dir); err != nil {
		_ = w.Close()
		return err
	}

	go func() {
		defer w.Close()
		for {
			select {
			case ev, ok := <-w.Events:
				if !ok {
					return
				}
				if ev.Op&(fsnotify.Write|fsnotify.Create) == 0 {
					continue
				}
				s.recomputeAndBroadcastStatus(filepath.Base(ev.Name))
			case err, ok := <-w.Errors:
				if !ok {
					return
				}
				fmt.Fprintf(os.Stderr, "session-status watcher: %v\n", err)
			}
		}
	}()
	return nil
}
```

- [ ] **Step 4: Wire it into `New(...)`**

Modify `internal/server/server.go` `New(...)` (around line 73, just before `return s`). Replace `go s.watchFiles()` block with:

```go
	go s.watchFiles()
	if err := s.startSessionStatusWatcher(); err != nil {
		fmt.Fprintf(os.Stderr, "session-status watcher unavailable: %v\n", err)
	}
	return s
```

Add `"fmt"` and `"os"` to the file's imports if not already present. (`fmt` is present; check `os`.)

- [ ] **Step 5: Run the new test, expect PASS**

Run: `go test ./internal/server/ -run TestSessionStatusWatcherEmitsDelta -v`

Expected: PASS (or `SKIP` on a platform without fsnotify, mirroring `TestFsnotifyWatcherBroadcastsOnAppend`).

- [ ] **Step 6: Run the whole package**

Run: `go test ./internal/server/`

Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add internal/server/status_watcher.go internal/server/status_watcher_test.go internal/server/server.go
git commit -m "feat: watch session-status/ dir, emit status-delta on writes"
```

---

## Task 6: 1s status sweeper — backstop for time-based transitions

Without this, a session that was running but stops emitting events (chat worker exits silently, or 3s mtime window elapses) never flips back to idle.

**Files:**
- Create: `internal/server/status_sweeper.go`
- Create: `internal/server/status_sweeper_test.go`
- Modify: `internal/server/server.go` (`New(...)`)

- [ ] **Step 1: Write the failing test**

Create `internal/server/status_sweeper_test.go`:

```go
package server

import (
	"strings"
	"testing"
	"time"
)

func TestSweepStatusFlipsStaleRunningToIdle(t *testing.T) {
	now := time.Date(2026, 5, 8, 12, 0, 0, 0, time.UTC)
	s := &Server{
		sessionsDir: t.TempDir(),
		chatSender:  &fakeSender{},
		clients:     make([]*sseClient, 0),
		// stale mtime well beyond the 3s recent-activity window
		fileMod:   map[string]time.Time{"a.jsonl": now.Add(-10 * time.Second)},
		lastKnown: map[string]struct{}{"a.jsonl": {}},
		now:       func() time.Time { return now },
	}
	c := s.addClient(globalSessID)
	defer s.removeClient(c)

	s.sweepStatusOnce()

	select {
	case msg := <-c.ch:
		if !strings.Contains(msg, "status-delta") || !strings.Contains(msg, `"running":false`) {
			t.Fatalf("unexpected msg: %q", msg)
		}
	case <-time.After(time.Second):
		t.Fatalf("expected idle delta")
	}

	if _, still := s.lastKnown["a.jsonl"]; still {
		t.Fatalf("lastKnown should no longer contain a.jsonl")
	}
}

func TestSweepStatusKeepsStillRunning(t *testing.T) {
	now := time.Now()
	s := &Server{
		sessionsDir: t.TempDir(),
		chatSender:  &fakeSender{},
		clients:     make([]*sseClient, 0),
		fileMod:     map[string]time.Time{"a.jsonl": now.Add(-1 * time.Second)},
		lastKnown:   map[string]struct{}{"a.jsonl": {}},
		now:         func() time.Time { return now },
	}
	c := s.addClient(globalSessID)
	defer s.removeClient(c)

	s.sweepStatusOnce()

	select {
	case msg := <-c.ch:
		t.Fatalf("unexpected broadcast on still-running session: %q", msg)
	case <-time.After(50 * time.Millisecond):
	}
	if _, ok := s.lastKnown["a.jsonl"]; !ok {
		t.Fatalf("lastKnown should still contain a.jsonl")
	}
}
```

- [ ] **Step 2: Run the test, confirm failure**

Run: `go test ./internal/server/ -run TestSweepStatus -v`

Expected: compile error — `s.sweepStatusOnce undefined`.

- [ ] **Step 3: Implement the sweeper**

Create `internal/server/status_sweeper.go`:

```go
package server

import "time"

// sweepStatusOnce re-evaluates every session id currently marked running
// in lastKnown. Idle sessions are not in the set, so the sweep cost scales
// with the number of *running* sessions — typically tiny.
//
// We snapshot the keys under the mutex, then drop the lock before calling
// recomputeAndBroadcastStatus (which takes the same lock when it has
// something to publish).
func (s *Server) sweepStatusOnce() {
	s.lastKnownMu.Lock()
	ids := make([]string, 0, len(s.lastKnown))
	for id := range s.lastKnown {
		ids = append(ids, id)
	}
	s.lastKnownMu.Unlock()

	for _, id := range ids {
		s.recomputeAndBroadcastStatus(id)
	}
}

// runStatusSweeper ticks every interval until ctx is done, calling
// sweepStatusOnce each tick.
func (s *Server) runStatusSweeper(stop <-chan struct{}, interval time.Duration) {
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-t.C:
			s.sweepStatusOnce()
		case <-stop:
			return
		}
	}
}
```

- [ ] **Step 4: Wire the sweeper into `New(...)`**

Modify `internal/server/server.go` `New(...)`. Add a stop channel field to `Server`:

```go
	stopCh chan struct{}
```

In `New`, after the watcher start lines:

```go
	s.stopCh = make(chan struct{})
	go s.runStatusSweeper(s.stopCh, time.Second)
```

(`time` is already imported.)

- [ ] **Step 5: Run the new tests, expect PASS**

Run: `go test ./internal/server/ -run TestSweepStatus -v`

Expected: PASS.

- [ ] **Step 6: Run the whole package**

Run: `go test ./internal/server/`

Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add internal/server/status_sweeper.go internal/server/status_sweeper_test.go internal/server/server.go
git commit -m "feat: 1s sweeper flips stale running entries back to idle"
```

---

## Task 7: Frontend — consume SSE status events, drop polling

**Files:**
- Modify: `web/src/index/index.js`
- Modify: `web/src/index/index.test.js`

- [ ] **Step 1: Update `index.test.js` to drive the new behaviour**

Replace `web/src/index/index.test.js` with:

```js
import { describe, expect, it, vi, beforeEach, afterEach } from 'vitest';
import { createSessionsPage } from './index.js';

function mountSessionCards() {
  document.body.innerHTML = `
    <div class="project-group">
      <div class="session-card" data-id="alpha.jsonl" data-session-id="alpha.jsonl" data-search="alpha"></div>
      <div class="session-card" data-id="beta.jsonl" data-session-id="beta.jsonl" data-search="beta"></div>
    </div>
  `;
}

class FakeEventSource {
  constructor(url) {
    this.url = url;
    this.onmessage = null;
    this.listeners = {};
    this.close = vi.fn();
    FakeEventSource.instances.push(this);
  }
  addEventListener(name, fn) {
    (this.listeners[name] ||= []).push(fn);
  }
  emit(name, data) {
    const evt = { data };
    if (name === 'message') {
      this.onmessage?.(evt);
      return;
    }
    for (const fn of this.listeners[name] || []) fn(evt);
  }
}
FakeEventSource.instances = [];

describe('createSessionsPage', () => {
  beforeEach(() => {
    FakeEventSource.instances = [];
    globalThis.EventSource = FakeEventSource;
  });

  afterEach(() => {
    document.body.innerHTML = '';
  });

  it('creates the sessions page Alpine state object', () => {
    const page = createSessionsPage();
    expect(page).toMatchObject({ query: '', modal: false, path: '', recent: [], creating: false, error: '' });
    expect(typeof page.filter).toBe('function');
    expect(typeof page.openModal).toBe('function');
    expect(typeof page.create).toBe('function');
  });

  it('sets error and does not set creating when create() is called with blank path', async () => {
    const page = createSessionsPage();
    page.path = '   ';
    await page.create();
    expect(page.error).toBe('Please enter a path');
    expect(page.creating).toBe(false);
  });

  it('applies running class from a status-snapshot event', () => {
    mountSessionCards();
    const page = createSessionsPage();
    page.subscribe();
    const es = FakeEventSource.instances[0];

    es.emit('status-snapshot', JSON.stringify({ running: ['alpha.jsonl'] }));

    expect(document.querySelector('[data-session-id="alpha.jsonl"]').classList.contains('session-card--running')).toBe(true);
    expect(document.querySelector('[data-session-id="beta.jsonl"]').classList.contains('session-card--running')).toBe(false);
  });

  it('toggles running class on status-delta events', () => {
    mountSessionCards();
    const page = createSessionsPage();
    page.subscribe();
    const es = FakeEventSource.instances[0];

    es.emit('status-snapshot', JSON.stringify({ running: [] }));
    es.emit('status-delta', JSON.stringify({ id: 'beta.jsonl', running: true }));
    expect(document.querySelector('[data-session-id="beta.jsonl"]').classList.contains('session-card--running')).toBe(true);

    es.emit('status-delta', JSON.stringify({ id: 'beta.jsonl', running: false }));
    expect(document.querySelector('[data-session-id="beta.jsonl"]').classList.contains('session-card--running')).toBe(false);
  });

  it('reloads the page on a new-session message', () => {
    const reloadSpy = vi.fn();
    Object.defineProperty(window, 'location', {
      configurable: true,
      value: { reload: reloadSpy },
    });
    const page = createSessionsPage();
    page.subscribe();
    const es = FakeEventSource.instances[0];
    es.emit('message', 'new-session');
    expect(reloadSpy).toHaveBeenCalled();
  });

  it('rebuilds running set on a fresh status-snapshot after reconnect', () => {
    mountSessionCards();
    const page = createSessionsPage();
    page.subscribe();
    const es = FakeEventSource.instances[0];
    es.emit('status-snapshot', JSON.stringify({ running: ['alpha.jsonl', 'beta.jsonl'] }));
    expect(document.querySelector('[data-session-id="alpha.jsonl"]').classList.contains('session-card--running')).toBe(true);

    // Simulate reconnect: subscribe again, new EventSource, fresh snapshot omits alpha.
    page.subscribe();
    const es2 = FakeEventSource.instances[1];
    es2.emit('status-snapshot', JSON.stringify({ running: ['beta.jsonl'] }));
    expect(document.querySelector('[data-session-id="alpha.jsonl"]').classList.contains('session-card--running')).toBe(false);
    expect(document.querySelector('[data-session-id="beta.jsonl"]').classList.contains('session-card--running')).toBe(true);
  });

  it('closes the previous EventSource when subscribe is called twice', () => {
    const page = createSessionsPage();
    page.subscribe();
    const first = FakeEventSource.instances[0];
    page.subscribe();
    expect(first.close).toHaveBeenCalled();
    expect(FakeEventSource.instances.length).toBe(2);
  });
});
```

- [ ] **Step 2: Run the test, confirm failure**

Run: `cd web && npx vitest run src/index/index.test.js`

Expected: FAIL — current `subscribe()` doesn't call `addEventListener` for the new event types and current code references the removed polling APIs.

- [ ] **Step 3: Update `index.js`**

Replace `web/src/index/index.js` with:

```js
import Alpine from 'alpinejs';
import { getJSON, postJSON } from '../shared/api.js';

export function createSessionsPage({ fetchImpl = globalThis.fetch?.bind(globalThis) } = {}) {
  return {
    query: '',
    modal: false,
    path: '',
    recent: [],
    creating: false,
    error: '',
    runningSessionIds: new Set(),
    _es: null,
    _unloadHandler: null,

    sessionCards() {
      return Array.from(document.querySelectorAll('.session-card[data-session-id]'));
    },

    syncRunningCardClasses() {
      this.sessionCards().forEach((card) => {
        const id = card.dataset.sessionId;
        card.classList.toggle('session-card--running', !!id && this.runningSessionIds.has(id));
      });
    },

    applySnapshot(data) {
      try {
        const payload = JSON.parse(data);
        const ids = Array.isArray(payload?.running) ? payload.running : [];
        this.runningSessionIds = new Set(ids);
        this.syncRunningCardClasses();
      } catch {
        /* malformed snapshot — ignore */
      }
    },

    applyDelta(data) {
      try {
        const payload = JSON.parse(data);
        if (!payload || typeof payload.id !== 'string') return;
        if (payload.running) this.runningSessionIds.add(payload.id);
        else this.runningSessionIds.delete(payload.id);
        this.syncRunningCardClasses();
      } catch {
        /* malformed delta — ignore */
      }
    },

    cleanup() {
      if (this._es) {
        this._es.close();
        this._es = null;
      }
      if (this._unloadHandler) {
        window.removeEventListener('beforeunload', this._unloadHandler);
        this._unloadHandler = null;
      }
    },

    subscribe() {
      try {
        this.cleanup();
        const es = new EventSource('/events?id=__all__');
        this._es = es;
        es.onmessage = (e) => {
          if (e.data === 'new-session') window.location.reload();
        };
        es.addEventListener('status-snapshot', (e) => this.applySnapshot(e.data));
        es.addEventListener('status-delta', (e) => this.applyDelta(e.data));
        this._unloadHandler = () => this.cleanup();
        window.addEventListener('beforeunload', this._unloadHandler);
      } catch {
        /* EventSource unavailable — page degrades to no live status */
      }
    },

    filter() {
      const q = this.query.toLowerCase();
      document.querySelectorAll('.session-card').forEach((card) => {
        const match = card.dataset.search.toLowerCase().includes(q);
        card.classList.toggle('hidden', !match);
      });
      document.querySelectorAll('.project-group').forEach((group) => {
        const anyVisible = group.querySelector('.session-card:not(.hidden)') !== null;
        group.style.display = anyVisible ? '' : 'none';
      });
    },

    async openModal() {
      this.modal = true;
      this.path = '';
      this.error = '';
      this.recent = [];
      this.$nextTick(() => this.$refs.sessionPath.focus());
      try {
        const response = await getJSON('/api/recent-locations');
        this.recent = (response.locations || []).slice(0, 10);
      } catch {
        // Intentional no-op: recent locations are optional.
      }
    },

    async create() {
      const p = this.path.trim();
      if (!p) {
        this.error = 'Please enter a path';
        return;
      }
      this.creating = true;
      this.error = '';
      try {
        const response = await postJSON('/api/new-session', { path: p });
        if (response.ok && response.id) {
          window.location = '/session?id=' + encodeURIComponent(response.id);
          return;
        }
        this.error = response.error || 'Failed to create session';
      } catch (error) {
        this.error = error.message || 'Network error';
      } finally {
        this.creating = false;
      }
    }
  };
}

if (typeof window !== 'undefined') {
  window.sessionsPage = createSessionsPage;
  if (!window.Alpine) {
    window.Alpine = Alpine;
    Alpine.start();
  }
}
```

The `fetchImpl` parameter is kept (unused in current code) so that any other test file or template still passing it does not break compile-time.

- [ ] **Step 4: Run the index test file, expect PASS**

Run: `cd web && npx vitest run src/index/index.test.js`

Expected: PASS.

- [ ] **Step 5: Run the whole frontend test suite**

Run: `cd web && npx vitest run`

Expected: PASS. If any other suite imported `createSessionsPage` with `pollIntervalMs`, that argument is now silently ignored — confirm no test asserts on the removed methods.

- [ ] **Step 6: Build the frontend bundle**

Run: `cd web && npx vite build`

Expected: build succeeds. The Go server embeds `web/dist`, so this output is what the binary will serve.

- [ ] **Step 7: Commit**

```bash
git add web/src/index/index.js web/src/index/index.test.js web/dist
git commit -m "feat: homepage consumes SSE status snapshot/delta, drops polling"
```

---

## Task 8: Manual smoke test

A 5-minute sanity check before declaring done.

**Files:** none (runtime check)

- [ ] **Step 1: Build and run**

Run: `make run` (or `go run . --port 8080` if no make target — check `Makefile`).

- [ ] **Step 2: Open the homepage in a browser**

Navigate to `http://localhost:8080/`. In DevTools Network tab, filter by `/api/worker-status`. Confirm **zero** such requests are made.

- [ ] **Step 3: Confirm the SSE connection**

Filter Network tab by `/events`. Confirm exactly **one** open EventSource to `/events?id=__all__`. Click it and observe the `status-snapshot` event arrives within ~100ms of page load.

- [ ] **Step 4: Trigger a running session**

Open another shell. Touch a jsonl file under the sessions dir:

```bash
touch <sessionsDir>/some-project/some-session.jsonl
```

In the browser, the corresponding card should pick up the `session-card--running` class within ~1s. After ~3s of inactivity, it should drop the class (the sweeper firing).

- [ ] **Step 5: Trigger a terminal session status**

```bash
mkdir -p <sessionsDir>/../session-status
echo '{"state":"running","updatedAt":"'$(date -u +%Y-%m-%dT%H:%M:%SZ)'"}' > <sessionsDir>/../session-status/some-terminal-session.jsonl
```

The card should immediately gain the running class. After 10s (the `sessionStatusTTL`), the sweeper expires it.

- [ ] **Step 6: Check no regressions on per-session pages**

Open one session page (`/session?id=...`). Confirm `/api/worker-status?id=...` still works (it's used by chat UI for `thinkingLevel`) and the page itself still receives `reload` events when the file changes.

---

## Out of scope

- Compressing rapid deltas (defer — measure first)
- Reconnect cursors / sequence numbers (snapshot-on-connect already handles this)
- A chat-worker `OnStatusChange` callback (jsonl watcher + sweeper covers it; revisit if needed)
- Deleting the `/api/worker-status` endpoint — it's still used by per-session chat UI for `thinkingLevel`
