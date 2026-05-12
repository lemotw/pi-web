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

//go:embed live_templates/session.html
var liveSessionHtml string

//go:embed live_templates/session.css
var liveSessionCss string



//go:embed live_templates/chat_composer.html
var chatComposerTmplStr string

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

// prepareSessionPageData computes the shared payload (base64-encoded session
// data, themed CSS, and body attributes) used by both live and export renders.
func prepareSessionPageData(session sessions.Session, cssTemplate string) (dataBase64, css, bodyAttrs string) {
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
	dataBase64 = base64.StdEncoding.EncodeToString(dataJSON)

	bodyBg := "#18181e"
	cardBg := "#1e1e24"
	infoBg := "#3c3728"

	css = cssTemplate
	css = strings.Replace(css, "{{THEME_VARS}}", precomputedThemeVars, 1)
	css = strings.Replace(css, "{{BODY_BG}}", bodyBg, 1)
	css = strings.Replace(css, "{{CONTAINER_BG}}", cardBg, 1)
	css = strings.Replace(css, "{{INFO_BG}}", infoBg, 1)

	if session.SessionUUID != "" {
		bodyAttrs = ` data-session-uuid="` + session.SessionUUID + `"`
	}
	return
}

// renderLiveSessionPage renders the interactive session viewer served at
// /session. It loads the Vite-built session module and includes the chat composer.
func renderLiveSessionPage(session sessions.Session) string {
	dataBase64, css, bodyAttrs := prepareSessionPageData(session, liveSessionCss)

	html := liveSessionHtml
	html = strings.Replace(html, "{{TITLE}}", template.HTMLEscapeString(session.Name), 1)
	html = strings.Replace(html, "{{CSS}}", css, 1)
	html = strings.Replace(html, "{{BODY_ATTRS}}", bodyAttrs, 1)
	html = strings.Replace(html, "{{SESSION_DATA}}", dataBase64, 1)
	html = strings.Replace(html, "{{SESSION_SCRIPT}}", `<script type="module" src="`+template.HTMLEscapeString(sessionScriptPath)+`"></script>`, 1)
	html = strings.Replace(html, "{{CHAT_COMPOSER}}", chatComposerHtmlForSession(session), 1)

	return html
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
