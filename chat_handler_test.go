package main

import (
	"bytes"
	"context"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"pi-web/internal/chat"
)

type fakeSender struct {
	sessionID   string
	sessionPath string
	chat        chat.Request
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

func (f *fakeSender) GetState(ctx context.Context, sessionID string) (WorkerStatus, error) {
	return WorkerStatus{State: WorkerStateIdle}, nil
}

func (f *fakeSender) Status(sessionID string) WorkerStatus {
	return WorkerStatus{State: WorkerStateIdle}
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
