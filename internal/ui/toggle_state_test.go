package ui

import (
	"os"
	"strings"
	"testing"
)

func TestSessionToggleButtonsReflectPersistedActiveState(t *testing.T) {
	checks := []string{
		"const TOGGLE_STATE_STORAGE_KEY = 'pi.sessionDetail.toggleState';",
		"toolsVisible: true",
		"toolOutputsExpanded: false",
		"window.sessionToggleState = {",
		"localStorage.getItem(TOGGLE_STATE_STORAGE_KEY)",
		"localStorage.setItem(TOGGLE_STATE_STORAGE_KEY, JSON.stringify(toggleState));",
		"btn.classList.toggle('active', isActive);",
		"btn.setAttribute('aria-pressed', isActive ? 'true' : 'false');",
		"data-action=\"toggle-tool-output\"",
		"Tool output",
		"T show/hide thinking · O show/hide tools · P expand/collapse tool output",
		".header-toggle-btn.active",
	}
	combined := exportJs + liveSessionCss
	for _, check := range checks {
		if !strings.Contains(combined, check) {
			t.Fatalf("session toggle controls missing persisted active-state behavior %q", check)
		}
	}
}

func TestToolsVisibilityAndOutputExpansionAreSeparateStates(t *testing.T) {
	checks := []string{
		"const applyToolsVisibilityState = (root) => {",
		"root.querySelectorAll('.tool-execution, .compaction').forEach(el => {",
		"el.style.display = toggleState.toolsVisible ? '' : 'none';",
		"const applyToolOutputState = (root) => {",
		"el.classList.toggle('expanded', toggleState.toolOutputsExpanded);",
		"toggleState.toolsVisible = !toggleState.toolsVisible;",
		"toggleState.toolOutputsExpanded = !toggleState.toolOutputsExpanded;",
	}
	for _, check := range checks {
		if !strings.Contains(exportJs, check) {
			t.Fatalf("tools visibility and output expansion are not separate; missing %q", check)
		}
	}
}

func TestNavigationReappliesCurrentToggleStateAfterRenderingMessages(t *testing.T) {
	checks := []string{
		"messagesEl.appendChild(fragment);",
		"window.sessionToggleState?.applyToNode(messagesEl);",
	}
	for _, check := range checks {
		if !strings.Contains(exportJs, check) {
			t.Fatalf("navigation does not reapply persisted toggle state after rendering messages; missing %q", check)
		}
	}
}

func TestLiveReloadUpdatesExistingAssistantWhenToolResultsArrive(t *testing.T) {
	entriesSrc, err := os.ReadFile(repoPath("web/src/session/live/live-entries.js"))
	if err != nil {
		t.Fatalf("read web/src/session/live/live-entries.js: %v", err)
	}
	eventsSrc, err := os.ReadFile(repoPath("web/src/session/live/live-events.js"))
	if err != nil {
		t.Fatalf("read web/src/session/live/live-events.js: %v", err)
	}
	combined := string(entriesSrc) + string(eventsSrc)
	checks := []string{
		"export function upsertEntry(",
		".replaceWith(node)",
		"state.liveRendered.add(entry.id)",
		"liveRendered.has(entry.id)",
		"export function refreshEntriesAffectedByToolResult(",
		"block.type === 'toolCall'",
		"block.id === toolResultEntry.message.toolCallId",
		"refreshEntriesAffectedByToolResult(entry, entries)",
	}
	for _, check := range checks {
		if !strings.Contains(combined, check) {
			t.Fatalf("live reload does not refresh existing assistant entries when tool results arrive; missing %q", check)
		}
	}
}

func TestLiveReloadEntriesInheritCurrentToggleState(t *testing.T) {
	jsChecks := []string{
		"applyToNode(node) {",
		"applyThinkingState(node);",
		"applyToolsVisibilityState(node);",
		"applyToolOutputState(node);",
		"window.applyToggleStateToNode = (node) => window.sessionToggleState.applyToNode(node);",
	}
	for _, check := range jsChecks {
		if !strings.Contains(exportJs, check) {
			t.Fatalf("template JS missing reusable toggle-state hook %q", check)
		}
	}

	liveReloadChecks := []string{
		"applyToggleStateToNode: window.applyToggleStateToNode",
		"applyToggleStateToNode?.(node)",
	}
	liveRunner, err := os.ReadFile(repoPath("web/src/session/live/live-reload-runner.js"))
	if err != nil {
		t.Fatalf("read web/src/session/live/live-reload-runner.js: %v", err)
	}
	liveEntries, err := os.ReadFile(repoPath("web/src/session/live/live-entries.js"))
	if err != nil {
		t.Fatalf("read web/src/session/live/live-entries.js: %v", err)
	}
	combined := string(liveRunner) + string(liveEntries)
	for _, check := range liveReloadChecks {
		if !strings.Contains(combined, check) {
			t.Fatalf("live reload JS does not apply current toggle state to appended or replaced entries; missing %q", check)
		}
	}
}

func TestLiveReloadRendererUsesToggleableThinkingAndToolMarkup(t *testing.T) {
	source, err := os.ReadFile(repoPath("web/src/session/live/live-renderer.js"))
	if err != nil {
		t.Fatalf("read web/src/session/live/live-renderer.js: %v", err)
	}
	checks := []string{
		`<div class="thinking-block"><div class="thinking-text">`,
		`<div class="thinking-collapsed">Thinking ...</div>`,
		`tool-output expandable`,
		`output-preview`,
		`output-full`,
	}
	for _, check := range checks {
		if !strings.Contains(string(source), check) {
			t.Fatalf("live reload renderer missing toggle-compatible markup %q", check)
		}
	}
}
