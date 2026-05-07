package main

import (
	"bytes"
	"strings"
	"testing"
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
	} {
		if !strings.Contains(rendered, marker) {
			t.Fatalf("rendered index template missing %q", marker)
		}
	}
}

func TestIndexTemplateUsesViteModuleNotStandaloneAlpine(t *testing.T) {
	var buf bytes.Buffer
	if err := indexTmpl.Execute(&buf, []Session{}); err != nil {
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
