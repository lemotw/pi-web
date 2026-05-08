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

func TestResumeButtonShowsNotificationWithoutChangingButtonText(t *testing.T) {
	session := sessions.Session{SessionSummary: sessions.SessionSummary{ID: "2026-05-08T13-05-24.068Z_492e5bad-c6e9-4c74-9195-f7efc309a7c7.jsonl", Filename: "2026-05-08T13-05-24.068Z_492e5bad-c6e9-4c74-9195-f7efc309a7c7.jsonl"}, Entries: []map[string]any{{"id": "aaaaaaaa"}}}
	html := generateExportHtml(session, true)
	if strings.Contains(html, `resumeBtn.textContent = 'Copied!'`) {
		t.Fatalf("resume button text should not change to Copied")
	}
	if !strings.Contains(html, `Copied — tap to view`) {
		t.Fatalf("resume copy should show a nearby tap-to-view notification")
	}
	if !strings.Contains(html, `resumeSessionArg`) {
		t.Fatalf("resume copy should derive UUID-only session argument")
	}
	if !strings.Contains(html, `substring(underscore + 1)`) {
		t.Fatalf("resume copy should strip timestamp prefix from session filename")
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
