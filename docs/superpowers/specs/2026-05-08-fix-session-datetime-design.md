# Fix Session DateTime Mismatch Between Homepage and Detail Page

## Problem

The homepage session list renders timestamps in UTC (server-side Go formatting), while the session detail page renders timestamps in the browser's local timezone (JavaScript `new Date().toLocaleString()`). This causes the same session to display different times depending on which page the user is viewing.

Example (UTC+7 browser):
- Homepage: "May 8, 2026 9:49 AM" (UTC)
- Detail page: "May 8, 2026 4:49:41 PM" (local)

## Root Cause

- `index_template.go` defines `fmtTime` which parses RFC3339 and formats with `time.Format("Jan 2, 2006 3:04 PM")` in the server's default timezone (UTC for Go's `time.Parse`).
- `templates/app/60-header.js` uses `new Date(header.timestamp).toLocaleString()`, which respects the browser locale and timezone.

## Design

### Approach
Client-side JavaScript formatting (Approach A from brainstorming). Render the raw ISO timestamp in the HTML and let the browser format it locally. This matches the detail page behavior, requires minimal code, and degrades gracefully.

### Changes

#### 1. Template (`templates/index.html`)
Replace:
```html
<span>{{ fmtTime .LastActivity }}</span>
```
With:
```html
<span data-timestamp="{{ .LastActivity }}">{{ .LastActivity }}</span>
```

#### 2. Go template functions (`index_template.go`)
- Remove `fmtTime` function entirely.
- Remove `"fmtTime"` entry from `funcMap`.

#### 3. Client-side formatting
Add a small script block to `templates/index.html` (before the module script) that runs on DOMContentLoaded:
```js
document.querySelectorAll('[data-timestamp]').forEach(el => {
  const ts = el.dataset.timestamp;
  if (ts) {
    const d = new Date(ts);
    if (!isNaN(d)) {
      el.textContent = d.toLocaleString(undefined, {
        month: 'short', day: 'numeric', year: 'numeric',
        hour: 'numeric', minute: '2-digit'
      });
    }
  }
});
```

### Testing

1. **Go test:** Verify `fmtTime` is no longer present in `funcMap` and the template no longer references it. Existing `templates_embed_test.go` can be extended.
2. **Manual verification:** Load the homepage, confirm timestamps match local time, open a session detail page for the same session, confirm the time is consistent.

### Backwards Compatibility
- No API or data format changes.
- If JavaScript is disabled, the raw ISO timestamp is displayed — acceptable degradation.
