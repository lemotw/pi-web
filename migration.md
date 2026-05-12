# pi-web Frontend Migration Roadmap

This document tracks the migration from the legacy `export/` + `live_templates/` hybrid frontend architecture to a fully Vite-owned build pipeline. The goal is to eliminate duplicated code, confusing directory naming, and the need to maintain two separate JavaScript implementations.

## Current Architecture (May 2026)

### Three directories with overlapping responsibilities

| Directory | Purpose | Build step? |
|-----------|---------|-------------|
| `web/` | Modern Vite frontend source (`web/src/`) + build output (`web/dist/`) | Yes (Vite) |
| `live_templates/` | Go-embedded HTML/JS templates for the **live** web UI | No |
| `export/` | Self-contained HTML + CSS + JS for **static export / share / gist** | No |

### Current problems

1. **CSS lives in `export/` but is used by the live UI too.** `export/template.css` is the global stylesheet for session pages. The live UI loads it via Go string injection. The name `export/template.css` implies it's export-only, but it isn't.

2. **Two JS codebases for the same UI.** The session viewer logic exists in:
   - `web/src/session/` — modern ES modules, built by Vite, used by the live UI
   - `export/app/*.js` — legacy concatenated IIFE scripts, used by share/gist
   Every bugfix or feature requires editing both.

3. ~~**Duplicated HTML shells.** `live_templates/session.html` and `export/template.html` are copy-paste twins with a `<!-- Keep in sync... -->` comment.~~ **Fixed** — live uses `live_templates/session.html` and export uses `export/index.html`.

4. ~~**Dead vendor files.** `export/vendor/marked.min.js` and `highlight.min.js` are still vendored separately for share/export inlining.~~ **Partially fixed** — `alpine.min.js` deleted.

5. **Empty `live.js` entry point.** `web/src/live/live.js` exports nothing and is built but never loaded.

---

## Phase 1: Move CSS into Vite

**Goal:** Stop `export/template.css` from being the live UI's stylesheet. Make `export/` truly export-only.

### 1.1 Session CSS

- [ ] Create `web/src/session/session.css`
- [ ] Copy session-specific CSS from `export/template.css` into it
- [ ] Add `import './session.css'` at the top of `web/src/session/session.js`
- [ ] Vite will automatically emit `session-xxx.css` during build

### 1.2 Index CSS

- [ ] Create `web/src/index/index.css`
- [ ] Extract the inline `<style>` block from `live_templates/index.html` into it
- [ ] Add `import './index.css'` at the top of `web/src/index/index.js`

### 1.3 Go server changes

- [ ] Update `dist_embed.go` / `main.go` to detect and serve emitted CSS files from `web/dist/assets/`
- [ ] Update the live session HTML shell to load CSS from `/static/assets/session-xxx.css` instead of the Go-injected `{{CSS}}` block
- [ ] Update `live_templates/index.html` to load CSS from `/static/assets/index-xxx.css` instead of the inline `<style>` block

**After this phase:** The live UI loads its own CSS from Vite assets. `export/template.css` is only used for share/export.

---

## Phase 2: Make share/export use Vite output

**Goal:** Eliminate the legacy `export/app/*.js` codebase. The share/gist path should inline the modern Vite bundle instead.

### 2.1 Share handler changes

- [ ] In `export.go` / `share.go`, read the Vite-built `session-xxx.js` and `session-xxx.css` from `web/dist/assets/` at runtime
- [ ] Inline the CSS and JS directly into the share HTML:
  ```html
  <style>/* contents of session-xxx.css */</style>
  <script type="module">/* contents of session-xxx.js */</script>
  ```
- [ ] Remove the `exportJs` variable and the `buildExportJsBundle()` function that concatenates `export/app/*.js`

### 2.2 Remove dead code

- [ ] Delete the entire `export/app/` directory (10 legacy JS files)
- [ ] Delete `export/vendor/marked.min.js` — `marked` is already an npm dep bundled by Vite
- [ ] Delete `export/vendor/highlight.min.js` — `highlight.js` is already an npm dep bundled by Vite
- [x] ~~Delete `export/vendor/alpine.min.js` — `alpinejs` is already an npm dep bundled by Vite~~
- [x] ~~Remove the `/static/alpine.js` HTTP handler from `main.go`~~
- [x] ~~Remove the `alpineJs` embed from `export.go`~~

**After this phase:** One JavaScript codebase. Share HTML is still fully self-contained (no external requests), but built from the modern Vite output.

---

## Phase 3: Deduplicate HTML shells ✅ Done

**Goal:** Kill the `<!-- Keep in sync -->` maintenance burden between `live_templates/session.html` and `export/index.html`.

**What we did:**
- Kept `live_templates/session.html` for the live session page and renamed the export shell to `export/index.html`.
- Deleted `live_templates/session.css` — it was unused; both live and export already shared `export/template.css`.
- Removed the `<!-- Keep in sync -->` comments.
- Split `renderSessionPage(session, showButtons)` into `renderLiveSessionPage(session)` and `renderExportSessionPage(session)`.
- Both functions use their own templates and apply the same placeholder replacements (`{{CSS}}`, `{{SESSION_DATA}}`, `{{SESSION_SCRIPT}}`, `{{CHAT_COMPOSER}}`), but with different values:
  - **Live:** external Vite module script, action buttons, chat composer
  - **Export:** inline JS/CSS, no buttons, no chat composer

**After this phase:** Two separate HTML templates with a shared CSS file. No more `<body>` string-injection hacks.

---

## Phase 4: Clean up remaining dead code

- [ ] Delete `web/src/live/live.js` (empty file) or repurpose it
- [ ] Delete `liveEntry` / `liveScriptPath` / `live` manifest handling from Go if `live.js` is removed
- [ ] Review `live_templates/live_reload.js` — consider moving to `web/src/` if it stays
- [ ] Remove `computeThemeVars()` from `export.go` if CSS is fully in Vite (variables move to CSS files)

---

## Phase 5 (Bonus): Fully client-side index page

**Goal:** Move the index page from Go template rendering to a static shell + client-side JS.

- [ ] Move session card rendering from Go template loops (`live_templates/index.html`) into `web/src/index/sessions-page.js`
- [ ] The index page fetches session data via `/api/...` endpoints
- [ ] `live_templates/index.html` becomes a thin static shell (like `export/index.html`)
- [ ] Fully eliminate inline CSS and server-side HTML generation for the index

This is lower priority since the current hybrid works.

---

## Post-Migration Directory Structure

```
web/
├── src/
│   ├── index/
│   │   ├── index.js
│   │   ├── index.css          ← moved from live_templates/index.html
│   │   └── sessions-page.js
│   ├── session/
│   │   ├── session.js
│   │   ├── session.css        ← moved from export/template.css
│   │   ├── chat/
│   │   ├── data/
│   │   ├── live/
│   │   ├── navigation/
│   │   ├── render/
│   │   ├── tree/
│   │   └── ui/
│   └── shared/
├── dist/                      ← Vite output (JS + CSS, embedded by Go)
│   └── assets/
│       ├── index-xxx.js
│       ├── index-xxx.css
│       ├── session-xxx.js
│       └── session-xxx.css
├── package.json
└── vite.config.js

live_templates/
├── index.html                 ← thin static shell (or fully client-side)
└── chat_composer.html

export/
└── (empty — CSS moved to Vite, JS deleted, vendor deleted)
```

---

## Quick Wins (do these first)

1. **Delete `export/vendor/`** — the files are unused by the live UI and will be replaced by Vite bundles for share.
2. ~~**Delete `/static/alpine.js` handler** — nothing references it.~~ **Done**
3. **Delete `web/src/live/live.js`** — empty, never loaded.

These are zero-risk deletions that reduce confusion immediately.
