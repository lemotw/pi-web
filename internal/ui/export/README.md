# Export

This directory contains the **standalone, server-independent session snapshot** that gets uploaded as a GitHub Gist when you click **↗ Share**.

The export is a single self-contained HTML file with all CSS and JS inlined. It has no chat composer, no SSE, no API calls, and no external asset dependencies.

## Directory Layout

```
internal/ui/export/
├── app/              # Export runtime JS modules
│   ├── 00-data.js    # Base64 session data decoding, URL params
│   ├── 10-tree.js    # Entry tree building & navigation state
│   ├── 20-filter.js  # Tool/branch filtering logic
│   ├── 30-format.js  # Markdown & code formatting helpers
│   ├── 40-render-tree.js    # Tree sidebar rendering
│   ├── 50-render-entry.js   # Entry detail rendering
│   ├── 60-header.js         # Session header rendering
│   ├── 70-navigation.js     # Tree selection & breadcrumb
│   └── 80-ui.js             # UI helpers, keyboard shortcuts, initial render
├── vendor/           # Vendored third-party libraries
│   ├── marked.min.js
│   └── highlight.min.js
└── README.md
```

## How the Export is Built

`internal/ui/export.go` (`RenderExportSessionPage`) produces the final HTML:

1. **Template & CSS** — Uses `internal/ui/export/index.html` and `internal/ui/export/template.css`.
2. **Vendor JS** — Inlines `vendor/marked.min.js` and `vendor/highlight.min.js`.
3. **App JS** — Reads the `app/*.js` files listed by `exportAppJSFiles` in `internal/ui/export.go`, concatenates them in that explicit order, and wraps the result in a single IIFE.
4. **Session data** — Embeds the session JSON as a base64 `<script type="application/json">` blob decoded by `00-data.js`.

## When to Edit These Files

| Change | Where |
|--------|-------|
| Fix export rendering / filtering / tree behavior | `internal/ui/export/app/*.js` |
| Update markdown or syntax-highlighting libraries | `internal/ui/export/vendor/*.js` |
| Change export snapshot layout or styling | `internal/ui/export/index.html` or `internal/ui/export/template.css` |
| Change live session viewer layout or styling | `internal/ui/live_templates/session.html` |

## Important Notes

- The numeric prefixes (`00-`, `10-`, …) on `app/*.js` **must** be preserved — they document and stabilize the explicit concatenation order in `exportAppJSFiles`.
- The live session page (`/session?id=...`) uses a **separate** template and CSS under `internal/ui/live_templates/`. It includes server-dependent chrome (action buttons, chat composer placeholder) that the export template deliberately omits. If you change either template, verify both render correctly.
- Unlike the live app (`web/src/session/`), the export JS is **not** built by Vite. It is plain ES5-ish JS concatenated at compile time by Go.
