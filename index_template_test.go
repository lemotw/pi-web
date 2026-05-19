package main

import (
	"strings"
	"testing"
	"time"
)

func TestFmtRelativeTime(t *testing.T) {
	if got := fmtRelativeTime(time.Now().Add(-5 * time.Minute).Format(time.RFC3339Nano)); got != "5 minutes ago" {
		t.Fatalf("fmtRelativeTime = %q, want 5 minutes ago", got)
	}
	if got := fmtRelativeTime(time.Now().Add(-1 * time.Hour).Format(time.RFC3339Nano)); got != "1 hour ago" {
		t.Fatalf("fmtRelativeTime = %q, want 1 hour ago", got)
	}
	if got := fmtRelativeTime("not-a-time"); got != "not-a-time" {
		t.Fatalf("fmtRelativeTime invalid = %q, want original", got)
	}
}

func TestIndexTemplateRendersRelativeTimeWithoutClientFlash(t *testing.T) {
	activity := time.Now().Add(-5 * time.Minute).Format(time.RFC3339Nano)
	html := renderIndexForTest(t, activity)
	if !strings.Contains(html, ">5 minutes ago</span>") {
		t.Fatalf("index HTML did not render relative time server-side:\n%s", html)
	}
	if strings.Contains(html, ">"+activity+"</span>") {
		t.Fatalf("index HTML rendered raw timestamp text, causing client-side flash")
	}
}

func renderIndexForTest(t *testing.T, activity string) string {
	t.Helper()
	var b strings.Builder
	err := indexTmpl.Execute(&b, []struct {
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
	}{
		{
			ID:            "s.jsonl",
			Project:       "proj",
			LastActivity:  activity,
			Name:          "hello",
			ChatAvailable: true,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	return b.String()
}
