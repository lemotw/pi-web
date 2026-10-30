# Confirmed worker idle

A quiet model connection is not an idle agent. In Pi 0.85.1, `agent_end` ends a low-level run but may be followed by retry, compaction or a queued continuation. `get_state.isStreaming` covers the session-level run, including those continuations; `isCompacting` also covers compaction/branch summarization.

## Invariants

- Only a valid `get_state` response with both busy flags explicitly false, an unchanged generation and no pending mutation can commit managed-worker idle. Missing/null/invalid flags are not false.
- Start/activity events and mutating commands invalidate older checks. Model changes and abort acknowledgements do not directly set idle.
- One state query per worker is shared by concurrent callers. No state/manager mutex or stdout reader waits for an RPC reply. Checks have a two-second deadline, including queued/writing pipe access. Each caller may abandon its wait without canceling the shared query.
- The worker requests checks at lifecycle boundaries and every five seconds while not confirmed idle, independently of open browsers. Failed checks become `unknown`; process exits remain `error`. The manager reports missing workers as `absent`, without spawning them.
- `Status()` and `/api/worker-status` read cached state. Polling never changes a worker's idle timer.
- `idleSince` starts on the first confirmed transition to idle. A busy/unknown interval clears it. Repeated idle confirmations preserve it.

## Retirement

The manager snapshots expired idle candidates, releases its mutex, then asks each worker for fresh state. It rechecks instance identity, its per-session operation epoch, pending sends/operations and the idle duration. The worker atomically claims retirement only if still idle with no pending mutation. A per-session retirement gate prevents a new worker from being created before the old Close finishes. New operations arriving during confirmation invalidate retirement instead of being sent to a closing worker.

Unknown never authorizes retirement, completion push or queue dequeue. Missing workers can be created on an explicit/queued send; terminal worker errors retain the existing replacement behavior. External terminal session-status/mtime fallbacks are observations only; they cannot establish managed-worker idle for the reaper.

## Diagnostics

`/api/worker-status` exposes `state`, metadata, `checkedAt` (Unix milliseconds when applicable) and an optional `error`. `/api/metrics` includes `checked_at`, `error` and actual confirmed `idle_for_s`. Timestamped stderr lifecycle records contain session, PID, old/new state and reason; retirement records distinguish confirmed idle TTL from error replacement. No prompt text is added to these records.

## Regression coverage

Go tests cover retry/compaction events, explicit busy flags, missing flags, stale replies after new activity, coalescing/cancellation, timeout/EOF, pipe write deadlines, idle timers, retirement races and unknown queue handling. Frontend tests cover unknown/absence, old HTTP snapshots after a send, cancellation and polling cleanup. The Playwright stub reports busy flags and settled events; `worker-status.spec.ts` exercises retry plus stream silence while the browser is away.

This patch does not change provider transport/retry policy, replay prompts, or automatically restart an unresponsive worker.
