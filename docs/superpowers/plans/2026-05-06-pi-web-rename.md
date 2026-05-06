# pi-web Rename Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Rename the local project, user-facing app identity, and repository references from `pi-sessions-viewer` / `ygncode/pi-sessions-viewer` to `pi-web` / `setkyar/pi-web` without changing runtime behavior.

**Architecture:** Apply a focused rename across metadata, docs, and user-facing configuration first, then rename file artifacts and local git/workspace wiring. Keep code behavior unchanged and verify with targeted search plus a clean build.

**Tech Stack:** Go, TypeScript, Git, GitHub CLI, macOS LaunchAgent plist

---

### Task 1: Update repository and product references in text/config files

**Files:**
- Modify: `README.md`
- Modify: `skill/SKILL.md`
- Modify: `view-sessions.ts`
- Modify: `go.mod`
- Modify: `com.pi-sessions-viewer.plist`

- [ ] **Step 1: Write the failing verification search**

```bash
rg -n "ygncode/pi-sessions-viewer|pi-sessions-viewer|com\.pi-sessions-viewer" README.md skill/SKILL.md view-sessions.ts go.mod com.pi-sessions-viewer.plist
```

Expected: matches showing the old repository name, binary name, module name, and plist label/path.

- [ ] **Step 2: Run the search to verify old references exist**

Run:

```bash
rg -n "ygncode/pi-sessions-viewer|pi-sessions-viewer|com\.pi-sessions-viewer" README.md skill/SKILL.md view-sessions.ts go.mod com.pi-sessions-viewer.plist
```

Expected: PASS with multiple matches, proving the rename work is still needed.

- [ ] **Step 3: Write the minimal rename changes**

Apply these exact replacements:

```diff
--- a/README.md
+++ b/README.md
@@
-# pi-sessions-viewer
+# pi-web
@@
-git clone https://github.com/ygncode/pi-sessions-viewer.git
-cd pi-sessions-viewer
-go build -o pi-sessions-viewer .
+git clone https://github.com/setkyar/pi-web.git
+cd pi-web
+go build -o pi-web .
@@
-sudo cp pi-sessions-viewer /usr/local/bin/
+sudo cp pi-web /usr/local/bin/
@@
-cp pi-sessions-viewer ~/.pi/agent/bin/
+cp pi-web ~/.pi/agent/bin/
@@
-pi-sessions-viewer
+pi-web
@@
-pi-sessions-viewer -o
+pi-web -o
@@
-pi-sessions-viewer -p 8080
+pi-web -p 8080
@@
-pi-sessions-viewer --host 127.0.0.1
-pi-sessions-viewer --host 100.x.y.z
+pi-web --host 127.0.0.1
+pi-web --host 100.x.y.z
@@
-cp com.pi-sessions-viewer.plist ~/Library/LaunchAgents/
-launchctl load ~/Library/LaunchAgents/com.pi-sessions-viewer.plist
+cp com.pi-web.plist ~/Library/LaunchAgents/
+launchctl load ~/Library/LaunchAgents/com.pi-web.plist
@@
-cp -r skill ~/.pi/agent/skills/pi-sessions-viewer
+cp -r skill ~/.pi/agent/skills/pi-web
```

```diff
--- a/skill/SKILL.md
+++ b/skill/SKILL.md
@@
-Run the `pi-sessions-viewer` command:
+Run the `pi-web` command:
@@
-pi-sessions-viewer -o
+pi-web -o
@@
-pi-sessions-viewer -p 31483 -o
+pi-web -p 31483 -o
@@
-The binary is installed at `~/.pi/agent/bin/pi-sessions-viewer`.
+The binary is installed at `~/.pi/agent/bin/pi-web`.
```

```diff
--- a/view-sessions.ts
+++ b/view-sessions.ts
@@
-          `Pi Sessions Viewer does not appear to be running on port ${port}. Try starting it: pi-sessions-viewer -o`,
+          `Pi Web does not appear to be running on port ${port}. Try starting it: pi-web -o`,
```

```diff
--- a/go.mod
+++ b/go.mod
@@
-module pi-sessions-viewer
+module pi-web
```

```diff
--- a/com.pi-sessions-viewer.plist
+++ b/com.pi-sessions-viewer.plist
@@
-    <string>com.pi-sessions-viewer</string>
+    <string>com.pi-web</string>
@@
-        <string>/Users/setkyar/.pi/agent/bin/pi-sessions-viewer</string>
+        <string>/Users/setkyar/.pi/agent/bin/pi-web</string>
@@
-    <string>/tmp/pi-sessions-viewer.log</string>
+    <string>/tmp/pi-web.log</string>
@@
-    <string>/tmp/pi-sessions-viewer.error.log</string>
+    <string>/tmp/pi-web.error.log</string>
```

- [ ] **Step 4: Run targeted verification after edits**

Run:

```bash
rg -n "ygncode/pi-sessions-viewer|pi-sessions-viewer|com\.pi-sessions-viewer" README.md skill/SKILL.md view-sessions.ts go.mod com.pi-sessions-viewer.plist
```

Expected: FAIL with no matches.

- [ ] **Step 5: Commit**

```bash
git add README.md skill/SKILL.md view-sessions.ts go.mod com.pi-sessions-viewer.plist
git commit -m "refactor: rename project references to pi-web"
```

### Task 2: Rename the plist artifact and preserve clean references

**Files:**
- Create: `com.pi-web.plist`
- Remove: `com.pi-sessions-viewer.plist`
- Modify: `README.md`

- [ ] **Step 1: Write the failing file-state check**

```bash
test -f com.pi-sessions-viewer.plist && echo old_exists
test -f com.pi-web.plist && echo new_exists
```

Expected: `old_exists` only.

- [ ] **Step 2: Run the check to confirm the old filename is still present**

Run:

```bash
test -f com.pi-sessions-viewer.plist && echo old_exists
test -f com.pi-web.plist && echo new_exists
```

Expected: `old_exists` only.

- [ ] **Step 3: Rename the plist file and update any remaining filename references**

Run:

```bash
mv com.pi-sessions-viewer.plist com.pi-web.plist
```

Then verify `README.md` uses this exact snippet:

```md
```bash
cp com.pi-web.plist ~/Library/LaunchAgents/
launchctl load ~/Library/LaunchAgents/com.pi-web.plist
```
```

- [ ] **Step 4: Run verification for the renamed artifact**

Run:

```bash
test -f com.pi-web.plist && echo new_exists
! test -f com.pi-sessions-viewer.plist && echo old_missing
rg -n "com\.pi-sessions-viewer\.plist" README.md com.pi-web.plist
```

Expected:
- `new_exists`
- `old_missing`
- no ripgrep matches

- [ ] **Step 5: Commit**

```bash
git add com.pi-web.plist README.md
git rm -f com.pi-sessions-viewer.plist
git commit -m "refactor: rename launch agent plist to pi-web"
```

### Task 3: Rename the local workspace folder and ensure git remotes stay correct

**Files:**
- Modify: local workspace path `/Users/setkyar/pi-sessions-viewer` → `/Users/setkyar/pi-web`
- Modify: `.git/config`

- [ ] **Step 1: Write the failing workspace/remotes check**

```bash
pwd
git remote -v
```

Expected before rename:
- current path ends with `/pi-sessions-viewer`
- `origin` points to `https://github.com/ygncode/pi-web.git`
- `setkyar` points to `https://github.com/setkyar/pi-web.git`
- `upstream` points to `https://github.com/ygncode/pi-sessions-viewer.git`

- [ ] **Step 2: Run the check to capture the pre-rename state**

Run:

```bash
pwd
git remote -v
```

Expected: current path still ends in `/pi-sessions-viewer`.

- [ ] **Step 3: Rename the local folder and reopen from the new path**

Run from the parent directory:

```bash
cd /Users/setkyar
mv pi-sessions-viewer pi-web
cd /Users/setkyar/pi-web
pwd
```

Then confirm remotes are exactly:

```bash
git remote set-url origin https://github.com/ygncode/pi-web.git
git remote set-url setkyar https://github.com/setkyar/pi-web.git
git remote set-url upstream https://github.com/ygncode/pi-sessions-viewer.git
git remote -v
```

- [ ] **Step 4: Verify the new workspace path and remotes**

Run:

```bash
cd /Users/setkyar/pi-web
pwd
git remote -v
```

Expected:
- path is `/Users/setkyar/pi-web`
- `origin` fetch/push is `https://github.com/ygncode/pi-web.git`
- `setkyar` fetch/push is `https://github.com/setkyar/pi-web.git`
- `upstream` fetch/push is `https://github.com/ygncode/pi-sessions-viewer.git`

- [ ] **Step 5: Commit**

No git commit is required for the folder rename itself because the working tree contents do not change. Instead, record the operational completion with:

```bash
cd /Users/setkyar/pi-web
git status --short
```

Expected: no unexpected file changes beyond the tracked rename work from earlier tasks.

### Task 4: Run full rename verification and build the project

**Files:**
- Verify: repository root search results
- Verify: build artifact from project root

- [ ] **Step 1: Write the failing broad search commands**

```bash
rg -n "ygncode/pi-sessions-viewer|pi-sessions-viewer|com\.pi-sessions-viewer" . --glob '!**/.git/**' --glob '!docs/superpowers/**'
```

Expected before completion: no matches outside intentionally preserved historical planning/spec docs; if matches appear in live project files, the rename is incomplete.

- [ ] **Step 2: Run the broad search to verify no live references remain**

Run:

```bash
rg -n "ygncode/pi-sessions-viewer|pi-sessions-viewer|com\.pi-sessions-viewer" . --glob '!**/.git/**' --glob '!docs/superpowers/**'
```

Expected: FAIL with no matches.

- [ ] **Step 3: Build the renamed binary**

Run:

```bash
go build -o pi-web .
```

Expected: command exits successfully and produces a `pi-web` executable in the repository root.

- [ ] **Step 4: Verify the build artifact and current git state**

Run:

```bash
test -x ./pi-web && echo built_ok
git status --short
```

Expected:
- `built_ok`
- working tree shows only intended tracked changes and optionally the untracked `pi-web` binary if not ignored

- [ ] **Step 5: Commit**

```bash
git add README.md skill/SKILL.md view-sessions.ts go.mod com.pi-web.plist
git commit -m "chore: finish pi-web rename"
```
