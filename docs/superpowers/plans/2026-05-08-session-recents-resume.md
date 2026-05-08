# Session Recents and Resume Clipboard Fix Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make Start New Session recents respond quickly and make Resume in Terminal clipboard copying safe when `navigator.clipboard` is unavailable.

**Architecture:** Keep the backend stateless by bounding and sorting the top-level project directory scan in `internal/sessions`. Keep the session-page copy behavior local to `templates/live_reload.js` with a guarded Clipboard API call and existing textarea fallback. Tests drive both changes before production edits.

**Tech Stack:** Go 1.25 tests, standard library filesystem APIs, embedded HTML/JS template tests, browser Clipboard API fallback.

---

## File Structure

- `internal/sessions/session.go`: owns session path encoding/decoding and recent-location listing.
- `internal/sessions/session_test.go`: regression tests for recent-location ordering and bounding.
- `templates/live_reload.js`: session page browser behavior, including Resume in Terminal copy handling.
- `export_html_test.go`: regression tests for generated local session HTML and Resume button script behavior.

---

### Task 1: Bound and order recent locations

**Files:**
- Modify: `internal/sessions/session_test.go`
- Modify: `internal/sessions/session.go`

- [ ] **Step 1: Write the failing test**

Add this test to `internal/sessions/session_test.go`:

```go
func TestListRecentLocationsReturnsNewestBoundedLocations(t *testing.T) {
	tmp := t.TempDir()
	base := time.Date(2026, 5, 8, 12, 0, 0, 0, time.UTC)

	for i := 0; i < 15; i++ {
		project := filepath.Join("/tmp", fmt.Sprintf("project-%02d", i))
		dir := filepath.Join(tmp, EncodeProjectName(project))
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatalf("mkdir project dir: %v", err)
		}
		mtime := base.Add(time.Duration(i) * time.Minute)
		if err := os.Chtimes(dir, mtime, mtime); err != nil {
			t.Fatalf("chtimes project dir: %v", err)
		}
	}

	locations, err := ListRecentLocations(tmp)
	if err != nil {
		t.Fatalf("ListRecentLocations failed: %v", err)
	}
	if len(locations) != 10 {
		t.Fatalf("expected 10 bounded locations, got %d: %#v", len(locations), locations)
	}
	if locations[0] != "/tmp/project-14" {
		t.Fatalf("expected newest project first, got %q", locations[0])
	}
	if locations[9] != "/tmp/project-05" {
		t.Fatalf("expected tenth newest project last, got %q", locations[9])
	}
}
```

Ensure imports include `fmt` if not already present.

- [ ] **Step 2: Run test to verify it fails**

Run:

```bash
go test ./internal/sessions -run TestListRecentLocationsReturnsNewestBoundedLocations -count=1
```

Expected: FAIL because `ListRecentLocations` currently returns all locations in directory order rather than 10 newest-first.

- [ ] **Step 3: Write minimal implementation**

In `internal/sessions/session.go`, replace `ListRecentLocations` with a bounded sorted implementation:

```go
const maxRecentLocations = 10

type recentLocationDir struct {
	name    string
	modTime time.Time
}

func ListRecentLocations(sessionsDir string) ([]string, error) {
	entries, err := os.ReadDir(sessionsDir)
	if err != nil {
		return nil, err
	}
	dirs := make([]recentLocationDir, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		dirs = append(dirs, recentLocationDir{name: e.Name(), modTime: info.ModTime()})
	}
	sort.Slice(dirs, func(i, j int) bool {
		return dirs[i].modTime.After(dirs[j].modTime)
	})

	locations := make([]string, 0, maxRecentLocations)
	seen := make(map[string]bool)
	for _, dir := range dirs {
		loc := DecodeProjectName(dir.name)
		if loc == "" || seen[loc] {
			continue
		}
		seen[loc] = true
		locations = append(locations, loc)
		if len(locations) == maxRecentLocations {
			break
		}
	}
	return locations, nil
}
```

Add `sort` to the imports in `internal/sessions/session.go`.

- [ ] **Step 4: Run test to verify it passes**

Run:

```bash
go test ./internal/sessions -run TestListRecentLocationsReturnsNewestBoundedLocations -count=1
```

Expected: PASS.

- [ ] **Step 5: Run package tests**

Run:

```bash
go test ./internal/sessions -count=1
```

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/sessions/session.go internal/sessions/session_test.go
git commit -m "fix: bound recent session locations"
```

---

### Task 2: Guard Resume in Terminal clipboard access

**Files:**
- Modify: `export_html_test.go`
- Modify: `templates/live_reload.js`

- [ ] **Step 1: Write the failing test**

Add this test to `export_html_test.go`:

```go
func TestResumeButtonClipboardGuardAndFallback(t *testing.T) {
	html := generateExportHtml(sampleSession(), true)
	if !strings.Contains(html, `navigator.clipboard && navigator.clipboard.writeText`) {
		t.Fatalf("resume clipboard code should guard navigator.clipboard before writeText")
	}
	if !strings.Contains(html, `document.execCommand('copy')`) {
		t.Fatalf("resume clipboard code should include execCommand fallback")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run:

```bash
go test . -run TestResumeButtonClipboardGuardAndFallback -count=1
```

Expected: FAIL because the current Resume handler directly calls `navigator.clipboard.writeText`.

- [ ] **Step 3: Write minimal implementation**

In `templates/live_reload.js`, replace the Resume button listener body with guarded copy logic:

```js
  // Resume in Terminal button
  var resumeBtn = document.getElementById('resume-btn');
  if (resumeBtn) {
    resumeBtn.addEventListener('click', function() {
      var cmd = 'pi --session ' + sessId;
      function markCopied() {
        resumeBtn.textContent = 'Copied!';
        setTimeout(function() {
          if (resumeBtn) resumeBtn.textContent = 'Resume in Terminal';
        }, 1500);
      }
      function fallbackCopy() {
        var textarea = document.createElement('textarea');
        textarea.value = cmd;
        textarea.style.position = 'fixed';
        textarea.style.opacity = '0';
        document.body.appendChild(textarea);
        textarea.select();
        var ok = document.execCommand('copy');
        document.body.removeChild(textarea);
        if (ok) markCopied();
      }
      if (navigator.clipboard && navigator.clipboard.writeText) {
        navigator.clipboard.writeText(cmd).then(markCopied).catch(fallbackCopy);
      } else {
        fallbackCopy();
      }
    });
  }
```

- [ ] **Step 4: Run test to verify it passes**

Run:

```bash
go test . -run TestResumeButtonClipboardGuardAndFallback -count=1
```

Expected: PASS.

- [ ] **Step 5: Run root tests**

Run:

```bash
go test . -count=1
```

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add export_html_test.go templates/live_reload.js
git commit -m "fix: guard resume clipboard access"
```

---

### Task 3: Full verification

**Files:**
- No code changes expected.

- [ ] **Step 1: Ensure frontend bundle exists**

Run:

```bash
npm --prefix web run build
```

Expected: Vite build succeeds and `web/dist` exists for Go embed tests.

- [ ] **Step 2: Run all Go tests**

Run:

```bash
go test ./... -count=1
```

Expected: all packages PASS.

- [ ] **Step 3: Inspect final diff**

Run:

```bash
git status --short
git log --oneline -5
git diff main...HEAD --stat
```

Expected: source/test changes are committed; ignored build artifacts may exist but no unintended tracked files remain.

---

## Self-Review

- Spec coverage: Task 1 covers bounded fast recents; Task 2 covers Clipboard API guard/fallback; Task 3 covers full verification.
- Placeholder scan: no TBD/TODO/fill-in steps remain.
- Type consistency: `ListRecentLocations`, `EncodeProjectName`, `generateExportHtml`, and `sampleSession` match existing code references.
