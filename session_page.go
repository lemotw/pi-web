package main

import (
	_ "embed"
	"encoding/base64"
	"encoding/json"
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

//go:embed live_templates/menu.css
var liveMenuCss string

//go:embed live_templates/palette.css
var livePaletteCss string

//go:embed live_templates/chat_composer.html
var chatComposerTmplStr string

var chatComposerTmpl = template.Must(template.New("chat_composer").Parse(chatComposerTmplStr))

var precomputedThemeVarsDark, precomputedThemeVarsLight = computeThemeVars()

func themeVarLines(vars map[string]string) string {
	var lines []string
	for k, v := range vars {
		lines = append(lines, fmt.Sprintf("      --%s: %s;", k, v))
	}
	sort.Strings(lines)
	return strings.Join(lines, "\n")
}

func replaceRequired(s, placeholder, value string) string {
	if !strings.Contains(s, placeholder) {
		panic(fmt.Errorf("template placeholder %s not found", placeholder))
	}
	return strings.Replace(s, placeholder, value, 1)
}

func computeThemeVars() (dark, light string) {
	darkVars := map[string]string{
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
		"text": "#d4d4d4", "border": "#5f87ff", "borderAccent": "#00d7ff", "borderMuted": "#505050",
		"toolOutput": "#808080", "bg": "var(--body-bg)", "selected-bg": "#3a3a4a", "border-accent": "#00d7ff",
		"userMessageText": "#d4d4d4", "customMessageText": "#d4d4d4",
	}
	lightVars := map[string]string{
		"cyan": "#5a8080", "blue": "#547da7", "green": "#588458", "red": "#aa5555",
		"yellow": "#9a7326", "gray": "#6c6c6c", "dimGray": "#767676", "darkGray": "#b0b0b0",
		"accent": "#5a8080", "selectedBg": "#d0d0e0", "userMessageBg": "#e8e8e8",
		"toolPendingBg": "#e8e8f0", "toolSuccessBg": "#e8f0e8", "toolErrorBg": "#f0e8e8",
		"customMessageBg": "#ede7f6", "customMessageLabel": "#7e57c2", "thinkingText": "#6c6c6c",
		"mdHeading": "#9a7326", "mdLink": "#547da7", "mdLinkUrl": "#767676",
		"mdCode": "#5a8080", "mdCodeBlock": "#588458", "mdCodeBlockBorder": "#6c6c6c",
		"mdQuote": "#6c6c6c", "mdQuoteBorder": "#6c6c6c", "mdHr": "#6c6c6c",
		"mdListBullet": "#588458", "toolDiffAdded": "#588458", "toolDiffRemoved": "#aa5555",
		"toolDiffContext": "#6c6c6c", "syntaxComment": "#008000", "syntaxKeyword": "#0000FF",
		"syntaxFunction": "#795E26", "syntaxVariable": "#001080", "syntaxString": "#A31515",
		"syntaxNumber": "#098658", "syntaxType": "#267F99", "syntaxOperator": "#000000",
		"syntaxPunctuation": "#000000", "thinkingOff": "#b0b0b0", "thinkingMinimal": "#767676",
		"thinkingLow": "#547da7", "thinkingMedium": "#5a8080", "thinkingHigh": "#875f87",
		"thinkingXhigh": "#8b008b", "bashMode": "#588458", "success": "#588458",
		"error": "#aa5555", "warning": "#9a7326", "muted": "#6c6c6c", "dim": "#767676",
		"text": "#1f2328", "border": "#547da7", "borderAccent": "#5a8080", "borderMuted": "#b0b0b0",
		"toolOutput": "#6c6c6c", "bg": "var(--body-bg)", "selected-bg": "#d0d0e0", "border-accent": "#5a8080",
		"userMessageText": "#1f2328", "customMessageText": "#1f2328",
	}
	return themeVarLines(darkVars), themeVarLines(lightVars)
}

// prepareSessionPageData computes the shared payload (base64-encoded session
// data, themed CSS, and body attributes) used by both live and export renders.
func prepareSessionPageData(session sessions.Session, cssTemplate string) (dataBase64, css, bodyAttrs string) {
	leafID := ""
	for i := len(session.Entries) - 1; i >= 0; i-- {
		if id, ok := session.Entries[i]["id"].(string); ok && id != "" {
			leafID = id
			break
		}
	}

	sessionData := map[string]any{
		"header":        session.Header,
		"entries":       session.Entries,
		"name":          session.Name,
		"leafId":        leafID,
		"systemPrompt":  nil,
		"tools":         nil,
		"renderedTools": nil,
	}

	dataJSON, _ := json.Marshal(sessionData)
	dataBase64 = base64.StdEncoding.EncodeToString(dataJSON)

	css = cssTemplate
	css = replaceRequired(css, "{{THEME_VARS_DARK}}", precomputedThemeVarsDark)
	css = replaceRequired(css, "{{THEME_VARS_LIGHT}}", precomputedThemeVarsLight)
	css = replaceRequired(css, "{{BODY_BG}}", "#18181e")
	css = replaceRequired(css, "{{CONTAINER_BG}}", "#1e1e24")
	css = replaceRequired(css, "{{INFO_BG}}", "#3c3728")
	css = replaceRequired(css, "{{BODY_BG_LIGHT}}", "#f6f5f2")
	css = replaceRequired(css, "{{CONTAINER_BG_LIGHT}}", "#ffffff")
	css = replaceRequired(css, "{{INFO_BG_LIGHT}}", "#fffae6")

	if session.SessionUUID != "" {
		bodyAttrs = ` data-session-uuid="` + session.SessionUUID + `"`
	}
	return
}

// renderLiveSessionPage renders the interactive session viewer served at
// /session. It loads the Vite-built session module and includes the chat composer.
func renderLiveSessionPage(session sessions.Session) string {
	dataBase64, css, bodyAttrs := prepareSessionPageData(session, liveSessionCss+"\n"+liveMenuCss+"\n"+livePaletteCss)

	scriptSrc := template.HTMLEscapeString(sessionScriptPath)
	preload := `<link rel="modulepreload" href="` + scriptSrc + `">`

	html := liveSessionHtml
	html = strings.ReplaceAll(html, "{{TITLE}}", template.HTMLEscapeString(session.Name))
	html = replaceRequired(html, "{{SESSION_PRELOAD}}", preload)
	html = replaceRequired(html, "{{CSS}}", css)
	html = replaceRequired(html, "{{BODY_ATTRS}}", bodyAttrs)
	html = replaceRequired(html, "{{SESSION_COMMAND_MENU}}", string(sessionDesktopMenuHTML()))
	html = replaceRequired(html, "{{MOBILE_COMMAND_MENU}}", string(sessionMobileMenuHTML()))
	html = replaceRequired(html, "{{SESSION_PALETTE}}", string(renderPalette(paletteData{
		ID:       "sessionPalette",
		Label:    "List sessions",
		SearchID: "session-palette-search",
		Actions:  false,
	})))
	html = replaceRequired(html, "{{SESSION_DATA}}", dataBase64)
	html = replaceRequired(html, "{{SESSION_SCRIPT}}", `<script type="module" src="`+scriptSrc+`"></script>`)
	html = replaceRequired(html, "{{FIRST_MESSAGE_STUB}}", firstMessageStub(session))
	html = replaceRequired(html, "{{CHAT_COMPOSER}}", chatComposerHtmlForSession(session))

	return html
}

// firstMessageStub returns a minimal HTML stub for the first user message so
// the browser has an LCP candidate before the JS bundle finishes loading.
// The navigator clears #messages and re-renders everything when JS runs.
func firstMessageStub(session sessions.Session) string {
	for _, entry := range session.Entries {
		if entry["type"] != "message" {
			continue
		}
		msg, ok := entry["message"].(map[string]any)
		if !ok {
			continue
		}
		if role, _ := msg["role"].(string); role != "user" {
			continue
		}
		text := sessions.ExtractMessageText(msg["content"])
		if text == "" {
			continue
		}
		if len(text) > 500 {
			text = text[:500]
		}
		return `<div class="user-message" aria-hidden="true"><div class="markdown-content"><p>` +
			template.HTMLEscapeString(text) + `</p></div></div>`
	}
	return ""
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
	// Pre-render the model badge from the last-known model in the session so
	// the user doesn't see a flash when the worker-status fetch completes.
	modelLabel := ""
	if session.Model != "" {
		modelLabel = session.Model
		if session.ModelProvider != "" {
			modelLabel = modelLabel + " @ " + session.ModelProvider
		}
	}
	data := struct {
		SessionID          string
		ChatAvailable      bool
		ChatDisabledReason string
		Cwd                string
		ModelLabel         string
	}{
		SessionID:          session.ID,
		ChatAvailable:      chatAvailable,
		ChatDisabledReason: session.ChatDisabledReason,
		Cwd:                cwd,
		ModelLabel:         modelLabel,
	}
	if !data.ChatAvailable && data.ChatDisabledReason == "" {
		data.ChatDisabledReason = "This session can be viewed, but chat is disabled because its working directory no longer exists."
	}
	if err := chatComposerTmpl.Execute(&buf, data); err != nil {
		return ""
	}
	return buf.String()
}
