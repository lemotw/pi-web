package main

import (
	"strings"
	"testing"

	"pi-web/internal/sessions"
)

func TestMobileSidebarClosesWhenNavigatingTree(t *testing.T) {
	checks := []string{
		"function setSidebarOpen(open)",
		"document.body.classList.toggle('sidebar-open', open);",
		"if (isMobileLayout()) closeSidebar();",
	}
	for _, check := range checks {
		if !strings.Contains(exportJs, check) {
			t.Fatalf("template JS missing %q; mobile sidebar can remain stuck over chat", check)
		}
	}
}

func TestMobileSessionActionsStayAtTopAndHideBehindSidebar(t *testing.T) {
	checks := []string{
		`class="session-actions"`,
		"body.sidebar-open .session-actions",
		"@media (max-width: 900px)",
		"top: calc(10px + env(safe-area-inset-top));",
	}
	combined := liveSessionCss + exportHtml + exportJs + chatComposerHtmlForSession(sessions.Session{SessionSummary: sessions.SessionSummary{ID: "s.jsonl"}}) + renderLiveSessionPage(sessions.Session{SessionSummary: sessions.SessionSummary{ID: "s.jsonl"}})
	for _, check := range checks {
		if !strings.Contains(combined, check) {
			t.Fatalf("mobile action UI missing %q", check)
		}
	}
	if strings.Contains(liveSessionCss, "bottom: calc(12px + env(safe-area-inset-bottom));") {
		t.Fatalf("mobile session actions should stay at top, not overlap the bottom chat composer")
	}
}

func TestMobileSessionActionsDoNotCoverHeaderToggleButtons(t *testing.T) {
	checks := []string{
		"padding: calc(var(--line-height) * 3) 16px var(--line-height);",
		".header-toggle-btn",
		"data-action=\"toggle-thinking\"",
		"data-action=\"toggle-tools\"",
	}
	combined := liveSessionCss + exportJs
	for _, check := range checks {
		if !strings.Contains(combined, check) {
			t.Fatalf("mobile session header controls missing %q; fixed session actions can cover toggle buttons", check)
		}
	}
}
