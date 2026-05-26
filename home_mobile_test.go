package main

import (
	"bytes"
	"strings"
	"testing"

	"pi-web/internal/sessions"
)

func TestHomePageMobilePreventsHorizontalOverflow(t *testing.T) {
	checks := []string{
		"overflow-x: hidden;",
		"min-width: 0;",
		"overflow-wrap: anywhere;",
	}
	for _, check := range checks {
		if !strings.Contains(indexCSS, check) {
			t.Fatalf("home page CSS missing %q; mobile home page can create horizontal scrollbar", check)
		}
	}
}

func TestHomePageRunningCountHasWorkspaceSummary(t *testing.T) {
	html := indexTmpl.Tree.Root.String()
	htmlChecks := []string{
		`<div class="workspace-summary">`,
		`<span class="stat-running" id="statRunning" data-running-stat>`,
		"document.querySelectorAll('[data-running-count]')",
		"document.querySelectorAll('[data-running-stat]')",
	}
	for _, check := range htmlChecks {
		if !strings.Contains(html, check) {
			t.Fatalf("home running-count workspace UI missing %q", check)
		}
	}
	cssChecks := []string{
		".workspace-summary {",
		".stat-running.visible { display: inline-flex; }",
		".status-dot {",
	}
	for _, check := range cssChecks {
		if !strings.Contains(indexCSS, check) {
			t.Fatalf("home running-count workspace CSS missing %q", check)
		}
	}
}

func TestHomePageNewSessionEntryPointsExist(t *testing.T) {
	var buf bytes.Buffer
	if err := indexTmpl.Execute(&buf, []sessions.Session{}); err != nil {
		t.Fatalf("failed to render index template: %v", err)
	}
	html := buf.String()
	htmlChecks := []string{
		`data-new-session-btn`,
		`New Session`,
		`New session`,
	}
	for _, check := range htmlChecks {
		if !strings.Contains(html, check) {
			t.Fatalf("home new-session entry point missing %q", check)
		}
	}
	// palette is rendered via {{ paletteHTML }} — check rendered output
	paletteChecks := []string{
		`id="commandPalette"`,
		`id="web-menu"`,
	}
	for _, check := range paletteChecks {
		if !strings.Contains(html, check) {
			t.Fatalf("home palette/menu entry point missing %q", check)
		}
	}
	css := indexCSS + "\n" + menuCSS + "\n" + paletteCSS
	cssChecks := []string{
		".command-palette-overlay {",
		".web-menu {",
		".palette-action",
	}
	for _, check := range cssChecks {
		if !strings.Contains(css, check) {
			t.Fatalf("home new-session entry point CSS missing %q", check)
		}
	}
}

func TestCommandPaletteSearchExists(t *testing.T) {
	var buf bytes.Buffer
	if err := indexTmpl.Execute(&buf, []sessions.Session{}); err != nil {
		t.Fatalf("failed to render index template: %v", err)
	}
	html := buf.String()
	checks := []string{
		`id="open-search"`,
		`id="search"`,
		`Search sessions...`,
		`⌘K`,
	}
	for _, check := range checks {
		if !strings.Contains(html, check) {
			t.Fatalf("index template rendered output missing %q", check)
		}
	}
}

func TestNewSessionModalExists(t *testing.T) {
	checks := []string{
		`id="modalOverlay"`,
		`class="modal-overlay"`,
		`id="sessionPath"`,
		`id="createBtn"`,
		`id="cancelBtn"`,
	}
	for _, check := range checks {
		if !strings.Contains(indexTmpl.Tree.Root.String(), check) {
			t.Fatalf("index template missing %q", check)
		}
	}
}

func TestHomePageSessionCardsExposeRunningStatusHook(t *testing.T) {
	if !strings.Contains(indexTmplStr, `data-session-id="{{ .ID }}"`) {
		t.Fatal("homepage should expose session ids for running-status cards")
	}
}
