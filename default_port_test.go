package main

import (
	"os"
	"strings"
	"testing"
)

const expectedDefaultPort = "31415"

func TestDefaultPortMatchesPublishedDefaults(t *testing.T) {
	if defaultPort != expectedDefaultPort {
		t.Fatalf("defaultPort = %q, want %q", defaultPort, expectedDefaultPort)
	}

	files := []string{
		"com.pi-web.plist",
		"README.md",
		"skill/SKILL.md",
		".pi/extensions/pi-web.ts",
	}

	for _, path := range files {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		if !strings.Contains(string(data), expectedDefaultPort) {
			t.Fatalf("%s does not mention default port %s", path, expectedDefaultPort)
		}
	}
}
