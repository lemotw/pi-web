package main

import (
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
	combined := templateJs + templateCss
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
		if !strings.Contains(templateJs, check) {
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
		if !strings.Contains(templateJs, check) {
			t.Fatalf("navigation does not reapply persisted toggle state after rendering messages; missing %q", check)
		}
	}
}

func TestLiveReloadUpdatesExistingAssistantWhenToolResultsArrive(t *testing.T) {
	checks := []string{
		"var LIVE_RENDERED = new Set();",
		"function upsertEntry(entry, allEntries) {",
		"replaceEntryNode(existing, node);",
		"LIVE_RENDERED.add(entry.id);",
		"LIVE_RENDERED.has(entry.id)",
		"function refreshEntriesAffectedByToolResult(toolResultEntry, allEntries) {",
		"if (block.type === 'toolCall' && block.id === toolResultEntry.message.toolCallId)",
		"refreshEntriesAffectedByToolResult(entry, entries);",
	}
	for _, check := range checks {
		if !strings.Contains(liveReloadJs, check) {
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
		if !strings.Contains(templateJs, check) {
			t.Fatalf("template JS missing reusable toggle-state hook %q", check)
		}
	}

	liveReloadChecks := []string{
		"if (typeof window.applyToggleStateToNode === 'function') {",
		"window.applyToggleStateToNode(node);",
	}
	for _, check := range liveReloadChecks {
		if !strings.Contains(liveReloadJs, check) {
			t.Fatalf("live reload JS does not apply current toggle state to appended or replaced entries; missing %q", check)
		}
	}
}

func TestLiveReloadRendererUsesToggleableThinkingAndToolMarkup(t *testing.T) {
	checks := []string{
		`<div class="thinking-block"><div class="thinking-text">`,
		`<div class="thinking-collapsed">Thinking ...</div>`,
		`tool-output expandable`,
		`output-preview`,
		`output-full`,
	}
	for _, check := range checks {
		if !strings.Contains(liveReloadJs, check) {
			t.Fatalf("live reload renderer missing toggle-compatible markup %q", check)
		}
	}
}
