package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHandleSetModelRequiresProviderAndModelId(t *testing.T) {
	root := t.TempDir()
	writeSessionFile(t, root, "test-project", "session.jsonl")
	s := &server{
		sessionsDir: root,
		chatSender:  &fakeSender{},
	}
	req := httptest.NewRequest(http.MethodPost, "/api/set-model?id=session.jsonl", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.handleSetModel(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
	if !strings.Contains(w.Body.String(), "provider and modelId required") {
		t.Fatalf("body = %q", w.Body.String())
	}
}

func TestHandleSetModelRejectsMissingSession(t *testing.T) {
	s := &server{sessionsDir: t.TempDir(), chatSender: &fakeSender{}}
	req := httptest.NewRequest(http.MethodPost, "/api/set-model?id=missing.jsonl", strings.NewReader(`{"provider":"a","modelId":"b"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.handleSetModel(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", w.Code)
	}
}

func TestModelSelectorMarkupExists(t *testing.T) {
	jsChecks := []string{
		"model-dropdown",
		"/api/models",
		"/api/set-model?id=",
		"loadModelSelector",
		"model-search",
		"model-scope-badge",
		"isScoped",
		"modelChanges",
	}
	for _, check := range jsChecks {
		if !strings.Contains(templateJs, check) {
			t.Fatalf("missing %q in template.js", check)
		}
	}
	cssChecks := []string{
		"model-dropdown",
		"model-dropdown-menu",
		"model-search",
		"model-item",
		"model-scope-badge",
	}
	for _, check := range cssChecks {
		if !strings.Contains(templateCss, check) {
			t.Fatalf("missing %q in template.css", check)
		}
	}
}
