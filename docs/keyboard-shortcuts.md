# Keyboard Shortcuts

## Index page (`/`)

### Page scrolling (vim-style)

Same vim-style shortcuts work on all pages when focus is **not** in an input, textarea, or contenteditable element.

| Shortcut | Action |
|----------|--------|
| `j` | Scroll down 300px |
| `k` | Scroll up 300px |
| `g g` | Scroll to top of page |
| `G` (Shift+G) | Scroll to bottom of page |
| `Escape` | Blur the active input so j/k navigation works |

### Index commands

| Shortcut | Context | Action |
|----------|---------|--------|
| `⌘K` / `Ctrl+K` | Page-level | Open search/sessions palette |
| `Escape` | Page-level | Close palette, menu, or modal |
| `Enter` | New-session path input | Create new session |

> `⌘K` / `Ctrl+K` is also Chrome's "focus address bar" shortcut. The browser may intercept it unless focus is inside a text input.

## Session detail page (`/session?id=...`)

### Page scrolling (vim-style)

These work on both index and session pages when focus is **not** in an input, textarea, or contenteditable element.

| Shortcut | Action |
|----------|--------|
| `j` | Scroll down 300px |
| `k` | Scroll up 300px |
| `g g` | Scroll to top of page |
| `G` (Shift+G) | Scroll to bottom of page |
| `I` (Shift+I) | Focus the chat composer textarea |
| `Escape` | Blur the active input so j/k navigation works |

### Sidebar & navigation

| Shortcut | Context | Action |
|----------|---------|--------|
| `⌘B` / `Ctrl+B` | Page-level | Toggle sidebar visibility |
| `⌘K` / `Ctrl+K` | Page-level | Open session list palette |
| `⌘T` / `Ctrl+T` | Page-level | New session |

> `⌘K` and `⌘T` are also browser shortcuts (focus address bar / new tab). The browser may intercept them unless focus is inside a text input.

### Chat composer

| Shortcut | Context | Action |
|----------|---------|--------|
| `Enter` | Chat textarea | Submit message |
| `Shift+Enter` | Chat textarea | Insert newline |

### Entry visibility toggles

| Shortcut | Context | Action |
|----------|---------|--------|
| `t` | When focus is **not** in an input/textarea | Toggle thinking visibility |
| `o` | When focus is **not** in an input/textarea | Toggle tools visibility |
| `p` | When focus is **not** in an input/textarea | Toggle tool outputs |

### Palettes, menus & sheets

| Shortcut | Context | Action |
|----------|---------|--------|
| `Escape` | Page-level | Close any open palette, menu, or sheet |
| `ArrowUp` / `ArrowDown` | Model selector popup | Navigate model list |
| `Enter` | Model selector popup | Select highlighted model |
| `ArrowUp` / `ArrowDown` | Fork modal | Navigate messages |
| `Enter` | Fork modal | Fork from highlighted message |
| `Tab` | Full-screen sheet | Cycle focus within the sheet |
| `Escape` | Full-screen sheet | Close the sheet |
