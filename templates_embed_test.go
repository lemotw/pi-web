package main

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"pi-web/internal/sessions"
)

func TestLiveReloadJsIsEmbeddedAndWrapped(t *testing.T) {
	if liveReloadJsBody == "" {
		t.Fatal("liveReloadJsBody is empty; templates/live_reload.js was not embedded")
	}
	if !strings.HasPrefix(liveReloadJs, "<script>\n") {
		t.Fatalf("liveReloadJs missing <script> open tag, got prefix %q", liveReloadJs[:min(20, len(liveReloadJs))])
	}
	if !strings.HasSuffix(strings.TrimRight(liveReloadJs, "\n"), "</script>") {
		t.Fatal("liveReloadJs missing </script> close tag")
	}
	for _, marker := range []string{
		"function appendEntry(",
		"new EventSource(",
		"share-btn",
		"resume-btn",
	} {
		if !strings.Contains(liveReloadJs, marker) {
			t.Fatalf("liveReloadJs missing expected JS marker %q", marker)
		}
	}
	// Must NOT contain a nested <script> tag inside the body — otherwise the
	// embedded file accidentally contains the wrapper too.
	if strings.Count(liveReloadJs, "<script>") != 1 {
		t.Fatalf("liveReloadJs should contain exactly one <script> tag, got %d", strings.Count(liveReloadJs, "<script>"))
	}
}

func TestChatComposerTemplateEscapesSessionID(t *testing.T) {
	got := chatComposerHtml(`"><script>alert(1)</script>`)
	if strings.Contains(got, "<script>alert(1)</script>") {
		t.Fatalf("chat composer leaked unescaped session id: %s", got)
	}
	if !strings.Contains(got, `id="pi-chat-composer"`) {
		t.Fatal("chat composer template did not render the form")
	}
}

func TestChatComposerTemplateInterpolatesPlainSessionID(t *testing.T) {
	got := chatComposerHtml("abc123.jsonl")
	if !strings.Contains(got, `data-session-id="abc123.jsonl"`) {
		t.Fatalf("chat composer missing expected session id attribute, got: %s", got)
	}
}

func TestIndexTemplateLoadedFromEmbeddedFile(t *testing.T) {
	if indexTmplStr == "" {
		t.Fatal("indexTmplStr is empty; templates/index.html was not embedded")
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

func TestIndexJsSourceReferencesAPINewSession(t *testing.T) {
	data, err := os.ReadFile("web/src/index/index.js")
	if err != nil {
		t.Fatalf("read web/src/index/index.js: %v", err)
	}
	if !strings.Contains(string(data), "/api/new-session") {
		t.Fatal("web/src/index/index.js missing /api/new-session reference")
	}
}

func TestIndexJsSourceReferencesAPIRecentLocations(t *testing.T) {
	data, err := os.ReadFile("web/src/index/index.js")
	if err != nil {
		t.Fatalf("read web/src/index/index.js: %v", err)
	}
	if !strings.Contains(string(data), "/api/recent-locations") {
		t.Fatal("web/src/index/index.js missing /api/recent-locations reference")
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
