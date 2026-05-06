package main

import (
	"strings"
	"testing"
)

func TestGenerateExportHtmlIncludesChatComposerWhenButtonsShown(t *testing.T) {
	session := Session{ID: "s.jsonl", Filename: "s.jsonl", Entries: []map[string]any{{"id": "aaaaaaaa"}}}
	html := generateExportHtml(session, true)
	if !strings.Contains(html, `id="pi-chat-composer"`) {
		t.Fatalf("chat composer missing from local session page")
	}
	if !strings.Contains(html, `data-session-id="s.jsonl"`) {
		t.Fatalf("session id missing from composer")
	}
}

func TestGenerateExportHtmlOmitsChatComposerForShare(t *testing.T) {
	session := Session{ID: "s.jsonl", Filename: "s.jsonl", Entries: []map[string]any{{"id": "aaaaaaaa"}}}
	html := generateExportHtml(session, false)
	if strings.Contains(html, `id="pi-chat-composer"`) {
		t.Fatalf("chat composer should not be included in share export")
	}
}
