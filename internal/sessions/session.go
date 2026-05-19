package sessions

import (
	"bufio"
	"bytes"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Typed structs for ParseSummary — avoid map[string]any per line.
type summaryLine struct {
	Type      string      `json:"type"`
	Timestamp string      `json:"timestamp"`
	Name      string      `json:"name"`
	CWD       string      `json:"cwd"`
	ID        string      `json:"id"`
	Provider  string      `json:"provider"`
	ModelID   string      `json:"modelId"`
	Message   *summaryMsg `json:"message"`
}

type summaryMsg struct {
	Role     string          `json:"role"`
	Provider string          `json:"provider"`
	Model    string          `json:"model"`
	Content  json.RawMessage `json:"content"`
	Usage    summaryUsage    `json:"usage"`
}

type summaryUsage struct {
	TotalTokens float64     `json:"totalTokens"`
	Cost        summaryCost `json:"cost"`
}

type summaryCost struct {
	Total float64 `json:"total"`
}

type SessionSummary struct {
	ID                 string
	SessionUUID        string
	Filename           string
	Project            string
	LastActivity       string
	Name               string
	MessageCount       int
	TokenTotal         int
	CostTotal          float64
	Model              string
	ModelProvider      string
	ChatAvailable      bool
	ChatDisabledReason string
}

type Session struct {
	SessionSummary
	Header  map[string]any
	Entries []map[string]any
}

func SortSummariesByActivity(s []SessionSummary) {
	type sortableSummary struct {
		summary  SessionSummary
		activity time.Time
		valid    bool
		index    int
	}

	items := make([]sortableSummary, len(s))
	for i := range s {
		activity, err := time.Parse(time.RFC3339, s[i].LastActivity)
		items[i] = sortableSummary{
			summary:  s[i],
			activity: activity,
			valid:    err == nil,
			index:    i,
		}
	}

	sort.Slice(items, func(i, j int) bool {
		if items[i].valid != items[j].valid {
			return items[i].valid
		}
		if !items[i].activity.Equal(items[j].activity) {
			return items[i].activity.After(items[j].activity)
		}
		return items[i].index < items[j].index
	})

	for i := range items {
		s[i] = items[i].summary
	}
}

// ParseSummary streams path line-by-line, accumulating only the fields the
// index page needs. Lines are discarded after parsing — unlike ParseFile,
// the full conversation is not retained in memory.
func ParseSummary(path, dirName, fileName string) (SessionSummary, error) {
	f, err := os.Open(path)
	if err != nil {
		return SessionSummary{}, err
	}
	defer f.Close()

	s := SessionSummary{
		ID:            fileName,
		Filename:      fileName,
		Project:       cleanProjectName(dirName),
		ChatAvailable: true,
	}

	var headerName, sessionInfoName, firstUserText, headerCwd string
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 64*1024), 4*1024*1024)
	for scanner.Scan() {
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}
		var raw summaryLine
		if err := json.Unmarshal(line, &raw); err != nil {
			continue
		}
		switch raw.Type {
		case "session":
			if raw.Name != "" {
				headerName = raw.Name
			}
			if raw.CWD != "" {
				headerCwd = raw.CWD
			}
			if raw.ID != "" {
				s.SessionUUID = raw.ID
			}
		case "session_info":
			if raw.Name != "" {
				sessionInfoName = raw.Name
			}
		case "message":
			if raw.Timestamp != "" {
				s.LastActivity = raw.Timestamp
			}
			if raw.Message == nil {
				continue
			}
			msg := raw.Message
			s.MessageCount++
			s.TokenTotal += int(msg.Usage.TotalTokens)
			s.CostTotal += msg.Usage.Cost.Total
			if msg.Model != "" {
				s.Model = msg.Model
				s.ModelProvider = msg.Provider
			}
			if firstUserText == "" && msg.Role == "user" {
				firstUserText = extractRawText(msg.Content)
			}
		case "model_change":
			if raw.Timestamp != "" {
				s.LastActivity = raw.Timestamp
			}
			if raw.ModelID != "" {
				s.Model = raw.ModelID
				s.ModelProvider = raw.Provider
			}
		default:
			if raw.Timestamp != "" {
				s.LastActivity = raw.Timestamp
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return SessionSummary{}, err
	}

	if s.LastActivity == "" {
		if info, err := os.Stat(path); err == nil {
			s.LastActivity = info.ModTime().Format(time.RFC3339)
		}
	}

	switch {
	case sessionInfoName != "":
		s.Name = sessionInfoName
	case headerName != "":
		s.Name = headerName
	case firstUserText != "":
		s.Name = truncate(firstUserText, 80)
	default:
		s.Name = fileName
	}

	if headerCwd != "" {
		s.Project = headerCwd
		if _, err := os.Stat(headerCwd); err != nil {
			s.ChatAvailable = false
			s.ChatDisabledReason = "This session can be viewed, but chat is disabled because its working directory no longer exists."
		}
	}

	return s, nil
}

// extractRawText pulls plain text from a json.RawMessage content value
// (string or content-block array). Used by ParseSummary.
func extractRawText(content json.RawMessage) string {
	if len(content) == 0 {
		return ""
	}
	if content[0] == '"' {
		var s string
		if json.Unmarshal(content, &s) == nil {
			return s
		}
	}
	if content[0] == '[' {
		var blocks []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		}
		if json.Unmarshal(content, &blocks) == nil {
			var buf strings.Builder
			for _, b := range blocks {
				if b.Type == "text" && b.Text != "" {
					buf.WriteString(b.Text)
				}
			}
			return buf.String()
		}
	}
	return ""
}

// ExtractMessageText pulls plain text from a message content value (string or
// content-block array). Used by both the parser and the session page renderer.
func ExtractMessageText(content any) string {
	return extractMessageText(content)
}

func extractMessageText(content any) string {
	switch v := content.(type) {
	case string:
		return v
	case []any:
		var buf strings.Builder
		for _, item := range v {
			if block, ok := item.(map[string]any); ok {
				if t, _ := block["type"].(string); t == "text" {
					if txt, _ := block["text"].(string); txt != "" {
						buf.WriteString(txt)
					}
				}
			}
		}
		return buf.String()
	}
	return ""
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

// ParseFile parses path in a single pass, collecting both the SessionSummary
// fields and the full Entries slice. This avoids the double-read that would
// result from calling ParseSummary followed by os.ReadFile.
func ParseFile(path, dirName, fileName string) (Session, error) {
	f, err := os.Open(path)
	if err != nil {
		return Session{}, err
	}
	defer f.Close()

	s := SessionSummary{
		ID:            fileName,
		Filename:      fileName,
		Project:       cleanProjectName(dirName),
		ChatAvailable: true,
	}

	var entries []map[string]any
	var header map[string]any
	var headerName, sessionInfoName, firstUserText, headerCwd string

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 64*1024), 4*1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var raw map[string]any
		if err := json.Unmarshal([]byte(line), &raw); err != nil {
			continue
		}
		entries = append(entries, raw)
		switch raw["type"] {
		case "session":
			header = raw
			if n, _ := raw["name"].(string); n != "" {
				headerName = n
			}
			if cwd, _ := raw["cwd"].(string); cwd != "" {
				headerCwd = cwd
			}
			if sid, _ := raw["id"].(string); sid != "" {
				s.SessionUUID = sid
			}
		case "session_info":
			if n, _ := raw["name"].(string); n != "" {
				sessionInfoName = n
			}
		case "message":
			if ts, ok := raw["timestamp"].(string); ok {
				s.LastActivity = ts
			}
			msg, ok := raw["message"].(map[string]any)
			if !ok {
				continue
			}
			s.MessageCount++
			if model, _ := msg["model"].(string); model != "" {
				s.Model = model
				s.ModelProvider, _ = msg["provider"].(string)
			}
			if usage, ok := msg["usage"].(map[string]any); ok {
				if t, ok := usage["totalTokens"].(float64); ok {
					s.TokenTotal += int(t)
				}
				if cost, ok := usage["cost"].(map[string]any); ok {
					if total, ok := cost["total"].(float64); ok {
						s.CostTotal += total
					}
				}
			}
			if firstUserText == "" {
				if role, _ := msg["role"].(string); role == "user" {
					firstUserText = extractMessageText(msg["content"])
				}
			}
		case "model_change":
			if ts, ok := raw["timestamp"].(string); ok {
				s.LastActivity = ts
			}
			if model, _ := raw["modelId"].(string); model != "" {
				s.Model = model
				s.ModelProvider, _ = raw["provider"].(string)
			}
		default:
			if ts, ok := raw["timestamp"].(string); ok {
				s.LastActivity = ts
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return Session{}, err
	}

	if s.LastActivity == "" {
		if info, err := os.Stat(path); err == nil {
			s.LastActivity = info.ModTime().Format(time.RFC3339)
		}
	}

	switch {
	case sessionInfoName != "":
		s.Name = sessionInfoName
	case headerName != "":
		s.Name = headerName
	case firstUserText != "":
		s.Name = truncate(firstUserText, 80)
	default:
		s.Name = fileName
	}

	if headerCwd != "" {
		s.Project = headerCwd
		if _, err := os.Stat(headerCwd); err != nil {
			s.ChatAvailable = false
			s.ChatDisabledReason = "This session can be viewed, but chat is disabled because its working directory no longer exists."
		}
	}

	return Session{SessionSummary: s, Header: header, Entries: entries}, nil
}

func cleanProjectName(dirName string) string {
	s := strings.TrimPrefix(dirName, "--")
	s = strings.TrimSuffix(s, "--")
	s = strings.ReplaceAll(s, "--", "/")
	return s
}

func EncodeProjectName(path string) string {
	s := strings.TrimSpace(path)
	s = strings.Trim(s, "/")
	s = strings.ReplaceAll(s, "/", "-")
	return "--" + s + "--"
}

func DecodeProjectName(dirName string) string {
	s := strings.TrimPrefix(dirName, "--")
	s = strings.TrimSuffix(s, "--")
	s = strings.ReplaceAll(s, "-", "/")
	if s != "" && !strings.HasPrefix(s, "/") {
		s = "/" + s
	}
	return s
}

const maxRecentLocations = 10

type recentLocationDir struct {
	name    string
	modTime time.Time
}

func ListRecentLocations(sessionsDir string) ([]string, error) {
	entries, err := os.ReadDir(sessionsDir)
	if err != nil {
		return nil, err
	}
	dirs := make([]recentLocationDir, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		dirs = append(dirs, recentLocationDir{name: e.Name(), modTime: info.ModTime()})
	}
	sort.Slice(dirs, func(i, j int) bool {
		return dirs[i].modTime.After(dirs[j].modTime)
	})

	locations := make([]string, 0, maxRecentLocations)
	seen := make(map[string]bool)
	for _, dir := range dirs {
		loc := DecodeProjectName(dir.name)
		if loc == "" || seen[loc] {
			continue
		}
		seen[loc] = true
		locations = append(locations, loc)
		if len(locations) == maxRecentLocations {
			break
		}
	}
	return locations, nil
}

func CreateSessionFile(sessionsDir, path string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return "", errors.New("path is required")
	}
	if strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		path = filepath.Join(home, path[2:])
	}
	path = filepath.Clean(path)
	if !filepath.IsAbs(path) {
		return "", errors.New("path must be absolute")
	}
	if _, err := os.Stat(path); os.IsNotExist(err) {
		if err := os.MkdirAll(path, 0755); err != nil {
			return "", err
		}
	}

	projectDir := filepath.Join(sessionsDir, EncodeProjectName(path))
	rel, err := filepath.Rel(sessionsDir, projectDir)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", errors.New("invalid path")
	}
	if err := os.MkdirAll(projectDir, 0755); err != nil {
		return "", err
	}

	id := randomUUID()
	timestamp := time.Now().UTC().Format("2006-01-02T15-04-05.000Z")
	filename := timestamp + "_" + id + ".jsonl"
	filePath := filepath.Join(projectDir, filename)

	header := map[string]any{
		"type":      "session",
		"version":   3,
		"id":        id,
		"timestamp": time.Now().UTC().Format(time.RFC3339Nano),
		"cwd":       path,
	}
	data, err := json.Marshal(header)
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(filePath, append(data, '\n'), 0644); err != nil {
		return "", err
	}
	return filename, nil
}

func randomUUID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}
