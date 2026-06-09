# Development Rules

- Only create an abstraction if it's actually needed
- Prefer clear function/variable names over inline comments
- Avoid helper functions when a simple inline expression would suffice
- Use knip to remove unused code if making large changes

## Docs

Read the relevant doc in `@docs/` before structural changes, and update the matching doc whenever your change makes it out of date.

## Testing

```bash
make test   # vitest + go test ./...
make check  # lint + format-check + test + build + vet (run before pushing)
make e2e    # Playwright E2E; needs `make e2e-setup` once. Not in test/check
```

- **Go:** table-driven tests in `*_test.go` alongside source.
- **Frontend:** tests next to source (`foo.js` → `foo.test.js`); DOM helpers take `{ documentImpl, windowImpl }` for DI.
- **Lint/format (frontend):** ESLint (`eslint-plugin-svelte`) + Prettier, config in `web/`. `make check` runs `frontend-lint` + `frontend-format-check`. Fix locally with `cd web && npm run format` (auto-format) and `npm run lint`. Style is 2-space indent, single quotes (enforced by Prettier).
- **E2E:** lives in `e2e/` (Playwright, built binary across desktop/mobile/iPad, stub `pi`). See `docs/dev/e2e-testing.md`.
- **Always `make build`, never `go build` alone** — `//go:embed` needs `web/dist` + `internal/ui/embedded/export/export.js` from the frontend build.

## Critical Rules

1. **Live app and export are separate renders.** Live = Svelte SPA via `internal/ui/embedded/app.html` (`spa_page.go`). Export/share = static snapshot via `internal/ui/embedded/share-session.html` (`export.go`), built from `web/src/export/export-entry.js` which reuses the live `web/src/session/` modules. Never leak live-only chrome (SPA scripts, SSE, chat) into the export.
2. **Existing session files are append-only for `session_info`** (browser rename + auto-titling). Conversation entries come from the `pi --mode rpc` worker, not pi-web.
3. **One worker per session.** Reused; crashed = evicted + replaced; idle reaped after 10 min.
4. **Icons:** Lucide only, via `web/src/shared/icons.js` — no hand-drawn SVG or unicode glyphs.
5. **i18n:** user-facing strings go through `t()` from `web/src/shared/i18n.js`; add keys to `web/src/shared/locales/en.js` first. Session content is never translated.
6. **Default port `31415`.** State: `~/.pi/agent/pi-web/pi-web-state.json`. SSE topics: `__all__` for index-wide, session ID per-session.
7. **Never start pi-web yourself** (`make dev`, `./pi-web`, etc.). The user runs it. Build/test with `make build` / `make test` / `make check`, but leave running the server to the user.

## User Override

If the user's instructions conflict with any rule in this document, ask for explicit confirmation before overriding. Only then execute their instructions.
