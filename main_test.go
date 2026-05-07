package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLoadIndexScriptValidManifest(t *testing.T) {
	tmpDir := t.TempDir()
	viteDir := filepath.Join(tmpDir, ".vite")
	os.MkdirAll(viteDir, 0755)
	assetsDir := filepath.Join(tmpDir, "assets")
	os.MkdirAll(assetsDir, 0755)

	manifest := `{"src/index/index.js":{"file":"assets/index-abc123.js"}}`
	os.WriteFile(filepath.Join(viteDir, "manifest.json"), []byte(manifest), 0644)
	os.WriteFile(filepath.Join(assetsDir, "index-abc123.js"), []byte("console.log('hello')"), 0644)

	path, js, err := loadIndexScript(tmpDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if path != "/static/assets/index-abc123.js" {
		t.Errorf("path = %q, want %q", path, "/static/assets/index-abc123.js")
	}
	if js != "console.log('hello')" {
		t.Errorf("js = %q, want %q", js, "console.log('hello')")
	}
}

func TestLoadIndexScriptMissingManifest(t *testing.T) {
	tmpDir := t.TempDir()
	_, _, err := loadIndexScript(tmpDir)
	if err == nil {
		t.Fatal("expected error for missing manifest")
	}
}

func TestLoadIndexScriptEmptyFile(t *testing.T) {
	tmpDir := t.TempDir()
	viteDir := filepath.Join(tmpDir, ".vite")
	os.MkdirAll(viteDir, 0755)
	manifest := `{"src/index/index.js":{"file":""}}`
	os.WriteFile(filepath.Join(viteDir, "manifest.json"), []byte(manifest), 0644)
	_, _, err := loadIndexScript(tmpDir)
	if err == nil {
		t.Fatal("expected error for empty file")
	}
}

func TestLoadIndexScriptAbsolutePath(t *testing.T) {
	tmpDir := t.TempDir()
	viteDir := filepath.Join(tmpDir, ".vite")
	os.MkdirAll(viteDir, 0755)
	manifest := `{"src/index/index.js":{"file":"/etc/passwd"}}`
	os.WriteFile(filepath.Join(viteDir, "manifest.json"), []byte(manifest), 0644)
	_, _, err := loadIndexScript(tmpDir)
	if err == nil {
		t.Fatal("expected error for absolute path")
	}
}

func TestLoadIndexScriptPathTraversal(t *testing.T) {
	tmpDir := t.TempDir()
	viteDir := filepath.Join(tmpDir, ".vite")
	os.MkdirAll(viteDir, 0755)
	manifest := `{"src/index/index.js":{"file":"../etc/passwd"}}`
	os.WriteFile(filepath.Join(viteDir, "manifest.json"), []byte(manifest), 0644)
	_, _, err := loadIndexScript(tmpDir)
	if err == nil {
		t.Fatal("expected error for path traversal")
	}
}

func TestBroadcastStatusChangeNotifiesSubscribers(t *testing.T) {
	root := t.TempDir()
	srv := newServer(filepath.Join(root, "sessions"), nil)

	client := srv.addStatusClient("s1.jsonl")
	defer srv.removeStatusClient(client)

	srv.broadcastStatusChange("s1.jsonl")

	select {
	case <-client.ch:
		// success
	case <-time.After(2 * time.Second):
		t.Fatal("expected status broadcast")
	}
}

func TestBroadcastStatusChangeIgnoresOtherSessions(t *testing.T) {
	root := t.TempDir()
	srv := newServer(filepath.Join(root, "sessions"), nil)

	client := srv.addStatusClient("s1.jsonl")
	defer srv.removeStatusClient(client)

	srv.broadcastStatusChange("s2.jsonl")

	select {
	case <-client.ch:
		t.Fatal("should not receive broadcast for different session")
	case <-time.After(200 * time.Millisecond):
		// success
	}
}

func TestFileChangeBroadcastsStatusChange(t *testing.T) {
	root := t.TempDir()
	sessionsDir := filepath.Join(root, "sessions")
	srv := newServer(sessionsDir, nil)

	client := srv.addStatusClient("session.jsonl")
	defer srv.removeStatusClient(client)

	// First call establishes baseline
	srv.recordModTime("session.jsonl", time.Now().Add(-time.Second))

	// Second call with newer time triggers broadcast
	srv.recordModTime("session.jsonl", time.Now())

	select {
	case <-client.ch:
		// success
	case <-time.After(2 * time.Second):
		t.Fatal("expected status broadcast after file change")
	}
}

func TestHandleEventsWithIdsSendsInitialStatusMap(t *testing.T) {
	root := t.TempDir()
	sessionsDir := filepath.Join(root, "sessions")
	srv := newServer(sessionsDir, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	req := httptest.NewRequest(http.MethodGet, "/events?ids=s1.jsonl,s2.jsonl", nil).WithContext(ctx)
	w := httptest.NewRecorder()

	srv.handleEvents(w, req)

	body := w.Body.String()
	if !strings.Contains(body, "data:") {
		t.Fatalf("expected SSE data event, got: %s", body)
	}
	if !strings.Contains(body, `"s1.jsonl"`) || !strings.Contains(body, `"s2.jsonl"`) {
		t.Fatalf("expected both session IDs in response, got: %s", body)
	}
}
