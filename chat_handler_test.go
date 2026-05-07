package main

import (
	"bytes"
	"context"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"pi-web/internal/chat"
	"pi-web/internal/workers"
)

type fakeSender struct {
	sessionID         string
	sessionPath       string
	chat              chat.Request
	state             workers.WorkerStatus
	status            workers.WorkerStatus
	getStateCalls     int
	getStateErr       error
}

func (f *fakeSender) Send(ctx context.Context, sessionID, sessionPath string, chat chat.Request) error {
	f.sessionID = sessionID
	f.sessionPath = sessionPath
	f.chat = chat
	return nil
}

func (f *fakeSender) SetModel(ctx context.Context, sessionID, sessionPath, provider, modelID string) error {
	return nil
}

func (f *fakeSender) SetThinkingLevel(ctx context.Context, sessionID, sessionPath, level string) error {
	return nil
}

func (f *fakeSender) GetState(ctx context.Context, sessionID string) (workers.WorkerStatus, error) {
	f.getStateCalls++
	if f.getStateErr != nil {
		return workers.WorkerStatus{}, f.getStateErr
	}
	if f.state.State != "" || f.state.ThinkingLevel != "" || f.state.Model != "" || f.state.ModelProvider != "" {
		return f.state, nil
	}
	return workers.WorkerStatus{State: workers.WorkerStateIdle}, nil
}

func (f *fakeSender) Status(sessionID string) workers.WorkerStatus {
	if f.status.State != "" || f.status.ThinkingLevel != "" || f.status.Model != "" || f.status.ModelProvider != "" {
		return f.status
	}
	return workers.WorkerStatus{State: workers.WorkerStateIdle}
}

func TestHandleChatSendsResolvedSession(t *testing.T) {
	root := t.TempDir()
	wantPath := writeSessionFile(t, root, "--tmp--project--", "session.jsonl")
	fake := &fakeSender{}
	s := &server{sessionsDir: root, chatSender: fake}
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	mw.WriteField("message", "hello")
	mw.Close()
	req := httptest.NewRequest(http.MethodPost, "/api/chat?id=session.jsonl", &body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	w := httptest.NewRecorder()
	s.handleChat(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", w.Code, w.Body.String())
	}
	if fake.sessionID != "session.jsonl" || fake.sessionPath != wantPath || fake.chat.Message != "hello" {
		t.Fatalf("fake = %#v, want path %q", fake, wantPath)
	}
}

func TestHandleChatRejectsUnknownSession(t *testing.T) {
	s := &server{sessionsDir: t.TempDir(), chatSender: &fakeSender{}}
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	mw.WriteField("message", "hello")
	mw.Close()
	req := httptest.NewRequest(http.MethodPost, "/api/chat?id=missing.jsonl", &body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	w := httptest.NewRecorder()
	s.handleChat(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", w.Code)
	}
}

func TestHandleChatRejectsBrokenSession(t *testing.T) {
	root := t.TempDir()
	dir := root + "/--tmp-project--"
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dir+"/session.jsonl", []byte("{\"type\":\"session\",\"version\":3,\"id\":\"sid\",\"timestamp\":\"2026-05-06T00:00:00.000Z\",\"cwd\":\"/definitely/missing/path\"}\n"), 0644); err != nil {
		t.Fatal(err)
	}
	s := &server{sessionsDir: root, chatSender: &fakeSender{}}
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	mw.WriteField("message", "hello")
	mw.Close()
	req := httptest.NewRequest(http.MethodPost, "/api/chat?id=session.jsonl", &body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	w := httptest.NewRecorder()
	s.handleChat(w, req)
	if w.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409; body=%s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "working directory no longer exists") {
		t.Fatalf("body = %q", w.Body.String())
	}
}

func TestHandleWorkerStatusDefaultsIdle(t *testing.T) {
	s := &server{sessionsDir: t.TempDir(), chatSender: &fakeSender{}}
	req := httptest.NewRequest(http.MethodGet, "/api/worker-status?id=session.jsonl", nil)
	w := httptest.NewRecorder()
	s.handleWorkerStatus(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d", w.Code)
	}
	if got := w.Body.String(); got != "{\"state\":\"idle\"}\n" {
		t.Fatalf("body = %q", got)
	}
}

func TestHandleWorkerStatusUsesRecentSessionFileActivity(t *testing.T) {
	root := t.TempDir()
	writeSessionFile(t, root, "test-project", "session.jsonl")
	now := time.Date(2026, 5, 7, 21, 0, 0, 0, time.UTC)
	s := &server{
		sessionsDir: root,
		chatSender:  &fakeSender{},
		fileMod:     map[string]time.Time{"session.jsonl": now.Add(-1500 * time.Millisecond)},
		now:         func() time.Time { return now },
	}
	req := httptest.NewRequest(http.MethodGet, "/api/worker-status?id=session.jsonl", nil)
	w := httptest.NewRecorder()

	s.handleWorkerStatus(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d", w.Code)
	}
	if got := w.Body.String(); got != "{\"state\":\"running\"}\n" {
		t.Fatalf("body = %q", got)
	}
}

func TestHandleWorkerStatusIgnoresStaleSessionFileActivity(t *testing.T) {
	root := t.TempDir()
	writeSessionFile(t, root, "test-project", "session.jsonl")
	now := time.Date(2026, 5, 7, 21, 0, 0, 0, time.UTC)
	s := &server{
		sessionsDir: root,
		chatSender:  &fakeSender{},
		fileMod:     map[string]time.Time{"session.jsonl": now.Add(-10 * time.Second)},
		now:         func() time.Time { return now },
	}
	req := httptest.NewRequest(http.MethodGet, "/api/worker-status?id=session.jsonl", nil)
	w := httptest.NewRecorder()

	s.handleWorkerStatus(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d", w.Code)
	}
	if got := w.Body.String(); got != "{\"state\":\"idle\"}\n" {
		t.Fatalf("body = %q", got)
	}
}

func TestHandleWorkerStatusSkipsGetStateWhenLocalStatusRunning(t *testing.T) {
	sender := &fakeSender{status: workers.WorkerStatus{State: workers.WorkerStateRunning}}
	s := &server{sessionsDir: t.TempDir(), chatSender: sender}
	req := httptest.NewRequest(http.MethodGet, "/api/worker-status?id=session.jsonl", nil)
	w := httptest.NewRecorder()

	s.handleWorkerStatus(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d", w.Code)
	}
	if sender.getStateCalls != 0 {
		t.Fatalf("GetState calls = %d, want 0", sender.getStateCalls)
	}
	if got := w.Body.String(); got != "{\"state\":\"running\"}\n" {
		t.Fatalf("body = %q", got)
	}
}

func TestHandleSetThinkingLevelRequiresLevel(t *testing.T) {
	root := t.TempDir()
	writeSessionFile(t, root, "test-project", "session.jsonl")
	s := &server{sessionsDir: root, chatSender: &fakeSender{}}
	req := httptest.NewRequest(http.MethodPost, "/api/set-thinking-level?id=session.jsonl", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.handleSetThinkingLevel(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
	if !strings.Contains(w.Body.String(), "level required") {
		t.Fatalf("body = %q", w.Body.String())
	}
}

func TestHandleSetThinkingLevelRejectsMissingSession(t *testing.T) {
	s := &server{sessionsDir: t.TempDir(), chatSender: &fakeSender{}}
	req := httptest.NewRequest(http.MethodPost, "/api/set-thinking-level?id=missing.jsonl", strings.NewReader(`{"level":"high"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.handleSetThinkingLevel(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", w.Code)
	}
}

func TestHandleWorkerStatusUsesSessionStatusFile(t *testing.T) {
	root := t.TempDir()
	sessionsDir := filepath.Join(root, "sessions")
	statusDir := filepath.Join(root, "session-status")
	if err := os.MkdirAll(statusDir, 0755); err != nil {
		t.Fatal(err)
	}
	sessionID := "test-session.jsonl"
	status := map[string]any{
		"sessionId": sessionID,
		"state":     "running",
		"updatedAt": time.Now().UTC().Format(time.RFC3339),
	}
	data, _ := json.Marshal(status)
	if err := os.WriteFile(filepath.Join(statusDir, sessionID), data, 0644); err != nil {
		t.Fatal(err)
	}

	s := &server{sessionsDir: sessionsDir, chatSender: &fakeSender{}}
	req := httptest.NewRequest(http.MethodGet, "/api/worker-status?id="+sessionID, nil)
	w := httptest.NewRecorder()
	s.handleWorkerStatus(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d", w.Code)
	}
	if got := w.Body.String(); got != "{\"state\":\"running\"}\n" {
		t.Fatalf("body = %q", got)
	}
}

func TestHandleWorkerStatusIgnoresStaleSessionStatusFile(t *testing.T) {
	root := t.TempDir()
	sessionsDir := filepath.Join(root, "sessions")
	statusDir := filepath.Join(root, "session-status")
	if err := os.MkdirAll(statusDir, 0755); err != nil {
		t.Fatal(err)
	}
	sessionID := "test-session.jsonl"
	status := map[string]any{
		"sessionId": sessionID,
		"state":     "running",
		"updatedAt": time.Now().Add(-30 * time.Second).UTC().Format(time.RFC3339),
	}
	data, _ := json.Marshal(status)
	if err := os.WriteFile(filepath.Join(statusDir, sessionID), data, 0644); err != nil {
		t.Fatal(err)
	}

	s := &server{sessionsDir: sessionsDir, chatSender: &fakeSender{}}
	req := httptest.NewRequest(http.MethodGet, "/api/worker-status?id="+sessionID, nil)
	w := httptest.NewRecorder()
	s.handleWorkerStatus(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d", w.Code)
	}
	if got := w.Body.String(); got != "{\"state\":\"idle\"}\n" {
		t.Fatalf("body = %q", got)
	}
}

func TestHandleWorkerStatusFallsThroughForIdleStatusFile(t *testing.T) {
	root := t.TempDir()
	sessionsDir := filepath.Join(root, "sessions")
	statusDir := filepath.Join(root, "session-status")
	if err := os.MkdirAll(statusDir, 0755); err != nil {
		t.Fatal(err)
	}
	sessionID := "test-session.jsonl"
	status := map[string]any{
		"sessionId": sessionID,
		"state":     "idle",
		"updatedAt": time.Now().UTC().Format(time.RFC3339),
	}
	data, _ := json.Marshal(status)
	if err := os.WriteFile(filepath.Join(statusDir, sessionID), data, 0644); err != nil {
		t.Fatal(err)
	}

	s := &server{sessionsDir: sessionsDir, chatSender: &fakeSender{}}
	req := httptest.NewRequest(http.MethodGet, "/api/worker-status?id="+sessionID, nil)
	w := httptest.NewRecorder()
	s.handleWorkerStatus(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d", w.Code)
	}
	if got := w.Body.String(); got != "{\"state\":\"idle\"}\n" {
		t.Fatalf("body = %q", got)
	}
}

func TestComputeWorkerStatusUsesSessionStatusFile(t *testing.T) {
	root := t.TempDir()
	sessionsDir := filepath.Join(root, "sessions")
	statusDir := filepath.Join(root, "session-status")
	if err := os.MkdirAll(statusDir, 0755); err != nil {
		t.Fatal(err)
	}
	sessionID := "test-session.jsonl"
	status := map[string]any{
		"sessionId": sessionID,
		"state":     "running",
		"updatedAt": time.Now().UTC().Format(time.RFC3339),
	}
	data, _ := json.Marshal(status)
	if err := os.WriteFile(filepath.Join(statusDir, sessionID), data, 0644); err != nil {
		t.Fatal(err)
	}

	s := &server{sessionsDir: sessionsDir, chatSender: &fakeSender{}}
	result := s.computeWorkerStatus(context.Background(), sessionID)
	if result == nil || result.State != workers.WorkerStateRunning {
		t.Fatalf("expected running, got %v", result)
	}
}

func TestComputeWorkerStatusReturnsIdleWhenNoStatusFile(t *testing.T) {
	root := t.TempDir()
	sessionsDir := filepath.Join(root, "sessions")
	s := &server{sessionsDir: sessionsDir, chatSender: &fakeSender{}}
	result := s.computeWorkerStatus(context.Background(), "nonexistent.jsonl")
	if result == nil || result.State != workers.WorkerStateIdle {
		t.Fatalf("expected idle, got %v", result)
	}
}
