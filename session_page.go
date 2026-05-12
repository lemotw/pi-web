package main

import (
	"encoding/base64"
	"encoding/json"
	_ "embed"
	"fmt"
	"html/template"
	"sort"
	"strings"

	"pi-web/internal/sessions"
)

//go:embed export/template.html
var templateHtml string

//go:embed export/template.css
var templateCss string

//go:embed live_templates/session.html
var liveSessionHtml string

//go:embed live_templates/live_reload.js
var liveReloadJsBody string

//go:embed live_templates/chat_composer.html
var chatComposerTmplStr string

var liveReloadJs = "<script>\n" + liveReloadJsBody + "</script>\n"

var chatComposerTmpl = template.Must(template.New("chat_composer").Parse(chatComposerTmplStr))

var precomputedThemeVars = computeThemeVars()

func computeThemeVars() string {
	vars := map[string]string{
		"cyan": "#00d7ff", "blue": "#5f87ff", "green": "#b5bd68", "red": "#cc6666",
		"yellow": "#ffff00", "gray": "#808080", "dimGray": "#666666", "darkGray": "#505050",
		"accent": "#8abeb7", "selectedBg": "#3a3a4a", "userMessageBg": "#343541",
		"toolPendingBg": "#282832", "toolSuccessBg": "#283228", "toolErrorBg": "#3c2828",
		"customMessageBg": "#2d2838", "customMessageLabel": "#9575cd", "thinkingText": "#808080",
		"mdHeading": "#f0c674", "mdLink": "#81a2be", "mdLinkUrl": "#666666",
		"mdCode": "#8abeb7", "mdCodeBlock": "#b5bd68", "mdCodeBlockBorder": "#808080",
		"mdQuote": "#808080", "mdQuoteBorder": "#808080", "mdHr": "#808080",
		"mdListBullet": "#8abeb7", "toolDiffAdded": "#b5bd68", "toolDiffRemoved": "#cc6666",
		"toolDiffContext": "#808080", "syntaxComment": "#6A9955", "syntaxKeyword": "#569CD6",
		"syntaxFunction": "#DCDCAA", "syntaxVariable": "#9CDCFE", "syntaxString": "#CE9178",
		"syntaxNumber": "#B5CEA8", "syntaxType": "#4EC9B0", "syntaxOperator": "#D4D4D4",
		"syntaxPunctuation": "#D4D4D4", "thinkingOff": "#505050", "thinkingMinimal": "#6e6e6e",
		"thinkingLow": "#5f87af", "thinkingMedium": "#81a2be", "thinkingHigh": "#b294bb",
		"thinkingXhigh": "#d183e8", "bashMode": "#b5bd68", "success": "#b5bd68",
		"error": "#cc6666", "warning": "#ffff00", "muted": "#808080", "dim": "#666666",
		"text": "#c9d1d9", "border": "#5f87ff", "borderAccent": "#00d7ff", "borderMuted": "#505050",
		"toolOutput": "#808080",
	}
	var lines []string
	for k, v := range vars {
		lines = append(lines, fmt.Sprintf("      --%s: %s;", k, v))
	}
	sort.Strings(lines)
	return strings.Join(lines, "\n")
}

func renderSessionPage(session sessions.Session, showButtons bool) string {
	leafID := ""
	if len(session.Entries) > 0 {
		if id, ok := session.Entries[len(session.Entries)-1]["id"].(string); ok {
			leafID = id
		}
	}

	sessionData := map[string]any{
		"header":        session.Header,
		"entries":       session.Entries,
		"leafId":        leafID,
		"systemPrompt":  nil,
		"tools":         nil,
		"renderedTools": nil,
	}

	dataJSON, _ := json.Marshal(sessionData)
	dataBase64 := base64.StdEncoding.EncodeToString(dataJSON)

	bodyBg := "#18181e"
	cardBg := "#1e1e24"
	infoBg := "#3c3728"

	// Both live and export sessions currently share the same CSS.
	// live_templates/session.css was removed because it was an exact duplicate.
	css := templateCss
	css = strings.Replace(css, "{{THEME_VARS}}", precomputedThemeVars, 1)
	css = strings.Replace(css, "{{BODY_BG}}", bodyBg, 1)
	css = strings.Replace(css, "{{CONTAINER_BG}}", cardBg, 1)
	css = strings.Replace(css, "{{INFO_BG}}", infoBg, 1)

	html := templateHtml
	if showButtons {
		html = liveSessionHtml
	}
	html = strings.Replace(html, "<title>Session Export</title>", "<title>"+template.HTMLEscapeString(session.Name)+"</title>", 1)
	html = strings.Replace(html, "{{CSS}}", css, 1)
	html = strings.Replace(html, "{{SESSION_DATA}}", dataBase64, 1)

	bodyAttrs := ""
	if session.SessionUUID != "" {
		bodyAttrs = ` data-session-uuid="` + session.SessionUUID + `"`
	}
	if showButtons {
		html = strings.Replace(html, "{{SESSION_SCRIPT}}", `<script type="module" src="`+template.HTMLEscapeString(sessionScriptPath)+`"></script>`, 1)
		btns := `<div class="session-actions">
<a href="/" class="session-action" title="Back to sessions">← Sessions</a>
<button id="share-btn" class="session-action" title="Share session as GitHub Gist">↗ Share</button>
<button id="resume-btn" class="session-action" title="Copy pi --session command to clipboard">$_ Terminal</button>
</div>`
		html = strings.Replace(html, "<body>", "<body"+bodyAttrs+">"+btns, 1)
		html = strings.Replace(html, "{{CHAT_COMPOSER}}", chatComposerHtmlForSession(session), 1)
	} else {
		inlineScript := "<script>\n" + markedJs + "\n</script>\n<script>\n" + hljsJs + "\n</script>\n<script>\n" + templateJs + "\n</script>"
		html = strings.Replace(html, "{{SESSION_SCRIPT}}", inlineScript, 1)
		html = strings.Replace(html, "<body>", "<body"+bodyAttrs+">", 1)
		html = strings.Replace(html, "{{CHAT_COMPOSER}}", "", 1)
	}

	return html
}

func chatComposerHtml(sessionID string) string {
	return chatComposerHtmlForSession(sessions.Session{SessionSummary: sessions.SessionSummary{ID: sessionID, ChatAvailable: true}})
}

func chatComposerHtmlForSession(session sessions.Session) string {
	var buf strings.Builder
	chatAvailable := session.ChatAvailable || session.ChatDisabledReason == ""
	cwd := ""
	if session.Header != nil {
		if c, ok := session.Header["cwd"].(string); ok {
			cwd = c
		}
	}
	data := struct {
		SessionID          string
		ChatAvailable      bool
		ChatDisabledReason string
		Cwd                string
	}{
		SessionID:          session.ID,
		ChatAvailable:      chatAvailable,
		ChatDisabledReason: session.ChatDisabledReason,
		Cwd:                cwd,
	}
	if !data.ChatAvailable && data.ChatDisabledReason == "" {
		data.ChatDisabledReason = "This session can be viewed, but chat is disabled because its working directory no longer exists."
	}
	if err := chatComposerTmpl.Execute(&buf, data); err != nil {
		return ""
	}
	return buf.String()
}
