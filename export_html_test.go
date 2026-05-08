package main

import (
	"strings"
	"testing"

	"pi-web/internal/sessions"
)

func TestGenerateExportHtmlIncludesChatComposerWhenButtonsShown(t *testing.T) {
	session := sessions.Session{SessionSummary: sessions.SessionSummary{ID: "s.jsonl", Filename: "s.jsonl"}, Entries: []map[string]any{{"id": "aaaaaaaa"}}}
	html := generateExportHtml(session, true)
	if !strings.Contains(html, `id="pi-chat-composer"`) {
		t.Fatalf("chat composer missing from local session page")
	}
	if !strings.Contains(html, `data-session-id="s.jsonl"`) {
		t.Fatalf("session id missing from composer")
	}
}

func TestGenerateExportHtmlOmitsChatComposerForShare(t *testing.T) {
	session := sessions.Session{SessionSummary: sessions.SessionSummary{ID: "s.jsonl", Filename: "s.jsonl"}, Entries: []map[string]any{{"id": "aaaaaaaa"}}}
	html := generateExportHtml(session, false)
	if strings.Contains(html, `id="pi-chat-composer"`) {
		t.Fatalf("chat composer should not be included in share export")
	}
}

func TestGenerateExportHtmlIncludesResumeButtonWhenButtonsShown(t *testing.T) {
	session := sessions.Session{SessionSummary: sessions.SessionSummary{ID: "s.jsonl", Filename: "s.jsonl"}, Entries: []map[string]any{{"id": "aaaaaaaa"}}}
	html := generateExportHtml(session, true)
	if !strings.Contains(html, `id="resume-btn"`) {
		t.Fatalf("resume button missing from local session page")
	}
	if !strings.Contains(html, `Resume in Terminal`) {
		t.Fatalf("resume button text missing from local session page")
	}
}

func TestResumeButtonClipboardGuardAndFallback(t *testing.T) {
	session := sessions.Session{SessionSummary: sessions.SessionSummary{ID: "s.jsonl", Filename: "s.jsonl"}, Entries: []map[string]any{{"id": "aaaaaaaa"}}}
	html := generateExportHtml(session, true)
	if !strings.Contains(html, "if (navigator.clipboard && navigator.clipboard.writeText) {\n        navigator.clipboard.writeText(cmd)") {
		t.Fatalf("resume clipboard code should guard navigator.clipboard before writeText")
	}
	if !strings.Contains(html, `document.execCommand('copy')`) {
		t.Fatalf("resume clipboard code should include execCommand fallback")
	}
}

func TestGenerateExportHtmlOmitsResumeButtonForShare(t *testing.T) {
	session := sessions.Session{SessionSummary: sessions.SessionSummary{ID: "s.jsonl", Filename: "s.jsonl"}, Entries: []map[string]any{{"id": "aaaaaaaa"}}}
	html := generateExportHtml(session, false)
	if strings.Contains(html, `id="resume-btn"`) {
		t.Fatalf("resume button should not be included in share export")
	}
}

func TestGenerateExportHtmlShowsDisabledChatNoticeForBrokenSession(t *testing.T) {
	session := sessions.Session{
		SessionSummary: sessions.SessionSummary{
			ID:                 "s.jsonl",
			Filename:           "s.jsonl",
			ChatAvailable:      false,
			ChatDisabledReason: "This session can be viewed, but chat is disabled because its working directory no longer exists.",
		},
		Entries: []map[string]any{{"id": "aaaaaaaa"}},
	}
	html := generateExportHtml(session, true)
	if !strings.Contains(html, `data-chat-available="false"`) {
		t.Fatalf("broken session should mark chat unavailable")
	}
	if !strings.Contains(html, session.ChatDisabledReason) {
		t.Fatalf("broken session notice missing from html")
	}
	if !strings.Contains(html, `disabled`) {
		t.Fatalf("broken session should disable chat controls")
	}
}
