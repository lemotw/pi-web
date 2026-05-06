# Web Chat Resume for Pi Sessions Viewer Design

## Overview

Add direct chat controls to the existing pi sessions viewer so a user can open a session page in the browser, type instructions, attach images, and have pi continue that same session headlessly. The existing session renderer remains the source of truth for display; the chat feature only adds a compact composer and a server-side bridge to pi RPC mode.

## Goals

- Resume an existing pi session from its browser session page.
- Reuse the current exported-session UI style and add only a bottom chat section.
- Support text prompts and image attachments in the first version.
- Allow multiple sessions to run in parallel with separate headless pi workers.
- Prefer binding to the Tailscale network by default, with localhost fallback.

## Non-goals

- Redesign the session tree, message renderer, or index page.
- Support arbitrary file attachments in the first version.
- Add authentication in the first version.
- Implement branch-point selection from the browser in the first version.
- Replace the existing JSONL file watcher/SSE update mechanism.

## User Experience

On a session page such as:

```text
http://<tailscale-ip>:31483/session?id=2026-05-05T15-28-44-256Z_019df8c1-80da-74b6-8633-447313aaa942.jsonl
```

The page shows the existing session export UI unchanged, plus a compact bottom composer:

- a small image icon button for image uploads,
- a textarea for instructions,
- a Send button,
- a small status line showing idle/running/queued/error.

The composer uses the existing monospace font, dark colors, border style, spacing, and compact button styling. Images are represented in the composer by small attachment indicators rather than large previews.

Keyboard behavior:

- Enter sends the prompt.
- Shift+Enter inserts a newline.

When the user sends a prompt, the UI posts to the Go server. The server routes the prompt to a headless pi RPC worker for that session. As pi writes new entries to the same JSONL session file, the existing file watcher and SSE/live reload logic updates the browser.

## Session Continuation Behavior

The first version continues from the current/latest active state of the selected session file. It does not use `leafId` or `targetId` URL parameters to branch from the viewed point.

If the same session's worker is idle, sending uses pi RPC `prompt`.

If the same session's worker is already streaming, sending uses steering behavior so the message is delivered after the current tool-call phase and before the next LLM call. This matches the normal pi steering-message behavior.

## Parallel Worker Behavior

The server owns a `WorkerManager`. The manager keeps a separate headless pi RPC subprocess per active session file. Different sessions can run concurrently, including sessions from different projects.

The first version has no default global active-worker cap. If a cap is added later, it should be exposed as configuration without changing the core per-session worker model.

Each worker:

1. starts `pi --mode rpc`,
2. switches to the absolute JSONL session path using RPC `switch_session`,
3. accepts prompt requests for that session,
4. tracks status from RPC state/events,
5. exits or is cleaned up when no longer needed or when the viewer shuts down.

## Network Binding

Startup should prefer a Tailscale-only bind by default:

1. Detect a Tailscale IP address.
2. If found, bind to that Tailscale IP only.
3. If no Tailscale IP is detected, bind to `127.0.0.1`.

Add a manual host override flag:

```text
--host <addr>
```

The existing `-p <port>` flag remains. Startup output should clearly print the listening URL, for example:

```text
Pi Sessions Viewer -> http://100.x.y.z:31483
Serving from: /Users/me/.pi/agent/sessions
```

or, when Tailscale is unavailable:

```text
Pi Sessions Viewer -> http://127.0.0.1:31483
Tailscale IP not detected; using localhost.
Serving from: /Users/me/.pi/agent/sessions
```

No authentication is added in the first version. The README must warn that anyone who can reach the bound Tailscale address can view sessions and send instructions to pi.

## Server API

Add a chat endpoint:

```text
POST /api/chat?id=<session-jsonl-filename>
```

Request body should support multipart form data so image files can be uploaded with the prompt:

- `message`: prompt text, required unless at least one image is provided,
- `images`: zero or more image files.

Validation:

- Resolve the session only by looking it up in `loadAllSessions`; do not build paths directly from query input.
- Reject unknown session IDs.
- Reject non-image uploads.
- Enforce a maximum per-image size and total request size.
- Return JSON errors with explicit HTTP status codes: `400` for invalid input, `404` for unknown sessions, `413` for oversized uploads, `415` for non-image uploads, and `500` for worker/RPC failures.

Response body:

```json
{
  "ok": true,
  "status": "accepted"
}
```

For errors:

```json
{
  "error": "human readable error"
}
```

Add worker status support. This can be either a dedicated endpoint or included in existing session API/SSE updates. The UI needs enough information to show idle/running/queued/error for the selected session.

## RPC Protocol Use

Use pi RPC mode documented in `docs/rpc.md`:

- Start: `pi --mode rpc`
- Switch session:

```json
{"type":"switch_session","sessionPath":"/absolute/path/to/session.jsonl"}
```

- Send prompt while idle:

```json
{"type":"prompt","message":"Fix the tests","images":[{"type":"image","data":"base64-image-data","mimeType":"image/png"}]}
```

- Send while streaming:

```json
{"type":"prompt","message":"Also update the README","streamingBehavior":"steer","images":[{"type":"image","data":"base64-image-data","mimeType":"image/png"}]}
```

The worker must parse stdout as JSONL using `\n` delimiters and correlate command responses with request IDs. Agent progress and failures after prompt acceptance are reported through RPC events and session-file updates, not as a second response to the original prompt.

## Error Handling

The UI should surface:

- pi executable not found,
- RPC worker startup failure,
- session switch rejected or cancelled,
- prompt rejected,
- image validation errors,
- worker process exit,
- request too large,
- network request failure.

Errors should appear in the compact status line and should not break the existing session viewer.

## Testing Strategy

Use TDD for implementation.

Add Go tests for:

- Tailscale IP detection and localhost fallback.
- Manual `--host` override behavior.
- Session lookup by ID without path traversal.
- Chat request validation for empty message, valid images, non-images, and oversized uploads.
- RPC JSONL parsing.
- Worker manager per-session routing.
- Same-session busy request using steering behavior.
- Parallel sessions getting separate worker instances.

Keep existing verification:

```bash
go test ./...
go vet ./...
gofmt -l .
```

## Documentation Updates

Update README with:

- browser chat usage,
- image attachment support,
- Tailscale binding behavior,
- `--host` override,
- security warning for unauthenticated network access,
- note that multiple sessions can run in parallel and may edit files concurrently.
