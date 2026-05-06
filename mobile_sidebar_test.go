package main

import (
	"strings"
	"testing"
)

func TestMobileSidebarClosesWhenNavigatingTree(t *testing.T) {
	checks := []string{
		"function setSidebarOpen(open)",
		"document.body.classList.toggle('sidebar-open', open);",
		"if (isMobileLayout()) closeSidebar();",
	}
	for _, check := range checks {
		if !strings.Contains(templateJs, check) {
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
	combined := templateCss + templateHtml + liveReloadJs + templateJs + chatComposerHtml("s.jsonl") + generateExportHtml(Session{ID: "s.jsonl"}, true)
	for _, check := range checks {
		if !strings.Contains(combined, check) {
			t.Fatalf("mobile action UI missing %q", check)
		}
	}
	if strings.Contains(templateCss, "bottom: calc(12px + env(safe-area-inset-bottom));") {
		t.Fatalf("mobile session actions should stay at top, not overlap the bottom chat composer")
	}
}
