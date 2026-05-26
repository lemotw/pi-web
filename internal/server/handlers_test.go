package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"pi-web/internal/sessions"
)

func TestHandleApiSessions_ProjectFilter(t *testing.T) {
	root := t.TempDir()

	// Session in project A
	writeSessionFile(t, root, "project-a", "a.jsonl")
	// Session in project B
	writeSessionFile(t, root, "project-b", "b.jsonl")

	// Use a subdirectory for sessionsDir so that Project is the cwd path,
	// not the session-file directory name.
	sessionsDir := filepath.Join(root, "sessions")
	if err := os.MkdirAll(sessionsDir, 0755); err != nil {
		t.Fatal(err)
	}
	s := &Server{
		sessionsDir: sessionsDir,
		cache:       sessions.NewCache(),
	}

	// Write sessions into sessionsDir/subdir so Project comes from the header cwd.
	// The writeSessionFile helper writes a header with cwd=<root>/cwd which makes
	// Project resolve to that path. We need sessions with different Projects.
	// Instead, let's write sessions directly with known header cwds.
	// Write sessions into sessionsDir/subdir so LoadAll finds them (it expects subdirectories).
	writeSessionWithCWD(t, filepath.Join(sessionsDir, "sub1"), "session-a.jsonl", "/home/user/project-a")
	writeSessionWithCWD(t, filepath.Join(sessionsDir, "sub2"), "session-b.jsonl", "/home/user/project-b")
	writeSessionWithCWD(t, filepath.Join(sessionsDir, "sub1"), "session-c.jsonl", "/home/user/project-a")

	// Without filter: all 3
	req := httptest.NewRequest(http.MethodGet, "/api/sessions", nil)
	w := httptest.NewRecorder()
	s.handleApiSessions(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d", w.Code)
	}
	var all map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &all); err != nil {
		t.Fatal(err)
	}
	allSessions, _ := all["sessions"].([]any)
	if len(allSessions) != 3 {
		t.Fatalf("expected 3 sessions, got %d", len(allSessions))
	}

	// Filter by project-a: 2 sessions
	req2 := httptest.NewRequest(http.MethodGet, "/api/sessions?project=/home/user/project-a", nil)
	w2 := httptest.NewRecorder()
	s.handleApiSessions(w2, req2)

	if w2.Code != http.StatusOK {
		t.Fatalf("status = %d", w2.Code)
	}
	var filtered map[string]any
	if err := json.Unmarshal(w2.Body.Bytes(), &filtered); err != nil {
		t.Fatal(err)
	}
	filteredSessions, _ := filtered["sessions"].([]any)
	if len(filteredSessions) != 2 {
		t.Fatalf("expected 2 sessions for project-a, got %d", len(filteredSessions))
	}

	// Filter by project-b: 1 session
	req3 := httptest.NewRequest(http.MethodGet, "/api/sessions?project=/home/user/project-b", nil)
	w3 := httptest.NewRecorder()
	s.handleApiSessions(w3, req3)

	var filteredB map[string]any
	if err := json.Unmarshal(w3.Body.Bytes(), &filteredB); err != nil {
		t.Fatal(err)
	}
	filteredBSessions, _ := filteredB["sessions"].([]any)
	if len(filteredBSessions) != 1 {
		t.Fatalf("expected 1 session for project-b, got %d", len(filteredBSessions))
	}

	// Filter by nonexistent project: 0 sessions
	req4 := httptest.NewRequest(http.MethodGet, "/api/sessions?project=/nonexistent", nil)
	w4 := httptest.NewRecorder()
	s.handleApiSessions(w4, req4)

	var filteredNone map[string]any
	if err := json.Unmarshal(w4.Body.Bytes(), &filteredNone); err != nil {
		t.Fatal(err)
	}
	filteredNoneSessions, _ := filteredNone["sessions"].([]any)
	if len(filteredNoneSessions) != 0 {
		t.Fatalf("expected 0 sessions for nonexistent project, got %d", len(filteredNoneSessions))
	}
}

// writeSessionWithCWD writes a session file with a specific cwd in its header.
func writeSessionWithCWD(t *testing.T, dir, name, cwd string) string {
	t.Helper()
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, name)
	content := `{"type":"session","version":3,"id":"sid","timestamp":"2026-05-06T00:00:00.000Z","cwd":"` + strings.ReplaceAll(cwd, `"`, `\"`) + `"}` + "\n" +
		`{"type":"message","id":"aaaaaaaa","parentId":null,"timestamp":"2026-05-06T00:00:01.000Z","message":{"role":"user","content":"hello","timestamp":1778025601000}}` + "\n"
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	return path
}
