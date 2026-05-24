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
		"@media (max-width: 900px)",
		".mobile-header {",
		"position: fixed;",
		"top: 0;",
	}
	combined := liveSessionCss + exportHtml + exportJs + chatComposerHtmlForSession(sessions.Session{SessionSummary: sessions.SessionSummary{ID: "s.jsonl"}}) + renderLiveSessionPage(sessions.Session{SessionSummary: sessions.SessionSummary{ID: "s.jsonl"}})
	for _, check := range checks {
		if !strings.Contains(combined, check) {
			t.Fatalf("mobile action UI missing %q", check)
		}
	}
	// Mobile session actions should be hidden; the mobile header bar stays at top.
	cssAfterMobile := liveSessionCss[strings.Index(liveSessionCss, "@media (max-width: 900px)"):]
	sessionActionsIdx := strings.Index(cssAfterMobile, ".session-actions")
	if sessionActionsIdx == -1 {
		t.Fatalf("missing .session-actions in mobile media query")
	}
	blockIdx := strings.Index(cssAfterMobile[sessionActionsIdx:], "}")
	if blockIdx == -1 {
		t.Fatalf("unclosed .session-actions block in mobile media query")
	}
	sessionActionsBlock := cssAfterMobile[sessionActionsIdx : sessionActionsIdx+blockIdx+1]
	if !strings.Contains(sessionActionsBlock, "display: none") {
		t.Fatalf("mobile .session-actions should be hidden; replaced by mobile-header + command panel")
	}
	// The mobile header should use top positioning, not bottom.
	mobileHeaderIdx := strings.Index(cssAfterMobile, ".mobile-header")
	if mobileHeaderIdx == -1 {
		t.Fatalf("missing .mobile-header in mobile media query")
	}
	headerBlockIdx := strings.Index(cssAfterMobile[mobileHeaderIdx:], "}")
	if headerBlockIdx == -1 {
		t.Fatalf("unclosed .mobile-header block in mobile media query")
	}
	mobileHeaderBlock := cssAfterMobile[mobileHeaderIdx : mobileHeaderIdx+headerBlockIdx+1]
	if strings.Contains(mobileHeaderBlock, "\nbottom:") && !strings.Contains(mobileHeaderBlock, "\nbottom: auto") {
		t.Fatalf("mobile header should use top positioning, not bottom, to avoid overlapping chat composer")
	}
}

func TestMobileSessionActionsDoNotCoverHeaderToggleButtons(t *testing.T) {
	checks := []string{
		"padding: calc(44px + env(safe-area-inset-top) + var(--line-height))",
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
