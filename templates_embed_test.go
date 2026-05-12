package main

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"pi-web/internal/sessions"
)

func TestChatComposerTemplateEscapesSessionID(t *testing.T) {
	got := chatComposerHtmlForSession(sessions.Session{SessionSummary: sessions.SessionSummary{ID: `"><script>alert(1)</script>`, ChatAvailable: true}})
	if strings.Contains(got, "<script>alert(1)</script>") {
		t.Fatalf("chat composer leaked unescaped session id: %s", got)
	}
	if !strings.Contains(got, `id="pi-chat-composer"`) {
		t.Fatal("chat composer template did not render the form")
	}
}

func TestChatComposerTemplateInterpolatesPlainSessionID(t *testing.T) {
	got := chatComposerHtmlForSession(sessions.Session{SessionSummary: sessions.SessionSummary{ID: "abc123.jsonl", ChatAvailable: true}})
	if !strings.Contains(got, `data-session-id="abc123.jsonl"`) {
		t.Fatalf("chat composer missing expected session id attribute, got: %s", got)
	}
}

func TestChatComposerTemplateRendersCwdFromHeader(t *testing.T) {
	got := chatComposerHtmlForSession(sessions.Session{
		SessionSummary: sessions.SessionSummary{ID: "s.jsonl", ChatAvailable: true},
		Header:         map[string]any{"cwd": "/home/user/project"},
	})
	if !strings.Contains(got, "cwd: /home/user/project") {
		t.Fatalf("chat composer missing expected cwd text, got: %s", got)
	}
	if !strings.Contains(got, `class="pi-chat-cwd"`) {
		t.Fatal("chat composer cwd span missing pi-chat-cwd class")
	}
	if !strings.Contains(got, `data-cwd="/home/user/project"`) {
		t.Fatal("chat composer cwd span missing data-cwd attribute")
	}
	if !strings.Contains(got, `title="Click to copy path"`) {
		t.Fatal("chat composer cwd span missing copy tooltip")
	}
}

func TestChatComposerTemplateEscapesCwd(t *testing.T) {
	got := chatComposerHtmlForSession(sessions.Session{
		SessionSummary: sessions.SessionSummary{ID: "s.jsonl", ChatAvailable: true},
		Header:         map[string]any{"cwd": "/tmp/<script>alert(1)</script>"},
	})
	if strings.Contains(got, "<script>alert(1)</script>") {
		t.Fatalf("chat composer leaked unescaped cwd: %s", got)
	}
	if !strings.Contains(got, "&lt;script&gt;") {
		t.Fatal("chat composer did not escape cwd properly")
	}
}

func TestChatComposerTemplateOmitsCwdWhenEmpty(t *testing.T) {
	got := chatComposerHtmlForSession(sessions.Session{
		SessionSummary: sessions.SessionSummary{ID: "s.jsonl", ChatAvailable: true},
		Header:         nil,
	})
	if strings.Contains(got, "pi-chat-cwd") {
		t.Fatal("chat composer should omit cwd span when cwd is empty")
	}
	if strings.Contains(got, "cwd:") {
		t.Fatal("chat composer should omit cwd text when cwd is empty")
	}
}

func TestIndexTemplateLoadedFromEmbeddedFile(t *testing.T) {
	if indexTmplStr == "" {
		t.Fatal("indexTmplStr is empty; live_templates/index.html was not embedded")
	}
	rendered := indexTmpl.Tree.Root.String()
	for _, marker := range []string{
		`id="newSessionBtn"`,
		`id="modalOverlay"`,
		`session-running-loader`,
	} {
		if !strings.Contains(rendered, marker) {
			t.Fatalf("rendered index template missing %q", marker)
		}
	}
}

func TestIndexTemplateUsesViteModuleNotStandaloneAlpine(t *testing.T) {
	var buf bytes.Buffer
	if err := indexTmpl.Execute(&buf, []sessions.Session{}); err != nil {
		t.Fatalf("failed to render index template: %v", err)
	}
	rendered := buf.String()
	if strings.Contains(rendered, "/static/alpine.js") {
		t.Fatal("rendered index page still contains standalone /static/alpine.js script")
	}
	if !strings.Contains(rendered, "/static/assets/index.js") {
		t.Fatal("rendered index page missing Vite module script /static/assets/index.js")
	}
}

func TestSessionPageUsesViteModuleForInteractiveViewer(t *testing.T) {
	sessionScriptPath = "/static/assets/session-test.js"
	html := renderLiveSessionPage(sessions.Session{SessionSummary: sessions.SessionSummary{ID: "s.jsonl", Name: "Session"}})
	if !strings.Contains(html, `<script type="module" src="/static/assets/session-test.js"></script>`) {
		t.Fatal("session page missing Vite session module script")
	}
	if strings.Contains(html, "new EventSource(") {
		t.Fatal("session page still inlines live reload JS instead of using Vite session module")
	}
	if strings.Contains(html, "{{SESSION_SCRIPT}}") || strings.Contains(html, "{{JS}}") {
		t.Fatal("session page still contains unreplaced script placeholders")
	}
}

func TestStaticExportKeepsInlineSessionRenderer(t *testing.T) {
	html := renderExportSessionPage(sessions.Session{SessionSummary: sessions.SessionSummary{ID: "s.jsonl", Name: "Session"}})
	if !strings.Contains(html, "function renderTree()") {
		t.Fatal("static export missing inline legacy session renderer")
	}
	if strings.Contains(html, `src="/static/assets/session`) {
		t.Fatal("static export should not depend on external Vite session asset")
	}
}

func TestIndexJsSourceReferencesAPINewSession(t *testing.T) {
	data, err := os.ReadFile("web/src/index/sessions-page.js")
	if err != nil {
		t.Fatalf("read web/src/index/sessions-page.js: %v", err)
	}
	if !strings.Contains(string(data), "/api/new-session") {
		t.Fatal("web/src/index/sessions-page.js missing /api/new-session reference")
	}
}

func TestIndexJsSourceReferencesAPIRecentLocations(t *testing.T) {
	data, err := os.ReadFile("web/src/index/sessions-page.js")
	if err != nil {
		t.Fatalf("read web/src/index/sessions-page.js: %v", err)
	}
	if !strings.Contains(string(data), "/api/recent-locations") {
		t.Fatal("web/src/index/sessions-page.js missing /api/recent-locations reference")
	}
}

func TestIndexTemplateShowsViewOnlyBadgeForBrokenSessions(t *testing.T) {
	var buf bytes.Buffer
	data := []sessions.Session{{SessionSummary: sessions.SessionSummary{
		ID:                 "broken.jsonl",
		Project:            "/tmp/project",
		LastActivity:       "2026-05-07T00:00:00Z",
		ChatAvailable:      false,
		ChatDisabledReason: "missing cwd",
	}}}
	if err := indexTmpl.Execute(&buf, data); err != nil {
		t.Fatalf("failed to render index template: %v", err)
	}
	rendered := buf.String()
	if !strings.Contains(rendered, `session-card-badge`) {
		t.Fatal("rendered index page missing session badge markup")
	}
	if !strings.Contains(rendered, `View only`) {
		t.Fatal("rendered index page missing view only label")
	}
}

func TestIndexTemplateOmitsViewOnlyBadgeForChatableSessions(t *testing.T) {
	var buf bytes.Buffer
	data := []sessions.Session{{SessionSummary: sessions.SessionSummary{
		ID:            "ok.jsonl",
		Project:       "/tmp/project",
		LastActivity:  "2026-05-07T00:00:00Z",
		ChatAvailable: true,
	}}}
	if err := indexTmpl.Execute(&buf, data); err != nil {
		t.Fatalf("failed to render index template: %v", err)
	}
	if strings.Contains(buf.String(), `View only`) {
		t.Fatal("rendered index page should not show view only badge for chatable sessions")
	}
}

func TestIndexTemplateDoesNotRegisterFmtTime(t *testing.T) {
	if _, ok := funcMap["fmtTime"]; ok {
		t.Fatal("funcMap should not contain fmtTime; timestamps are formatted client-side")
	}
}

func TestIndexTemplateRendersDataTimestampAttribute(t *testing.T) {
	var buf bytes.Buffer
	data := []sessions.Session{{SessionSummary: sessions.SessionSummary{
		ID:            "s1.jsonl",
		Project:       "/tmp/project",
		LastActivity:  "2026-05-08T09:49:41.591Z",
		ChatAvailable: true,
	}}}
	if err := indexTmpl.Execute(&buf, data); err != nil {
		t.Fatalf("failed to render index template: %v", err)
	}
	rendered := buf.String()
	if !strings.Contains(rendered, `data-timestamp="2026-05-08T09:49:41.591Z"`) {
		t.Fatalf("rendered index page missing data-timestamp attribute, got: %s", rendered)
	}
	// Ensure the old server-side formatted text is NOT present
	if strings.Contains(rendered, "May 8, 2026 9:49 AM") {
		t.Fatal("rendered index page still contains server-side formatted timestamp")
	}
}
