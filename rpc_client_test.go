package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"testing"
)

func TestSplitJSONLLinesHandlesCRLF(t *testing.T) {
	lines, err := readJSONLLines(bytes.NewBufferString("{\"a\":1}\r\n{\"b\":2}\n"))
	if err != nil {
		t.Fatalf("readJSONLLines error: %v", err)
	}
	if len(lines) != 2 || lines[0] != `{"a":1}` || lines[1] != `{"b":2}` {
		t.Fatalf("lines = %#v", lines)
	}
}

func TestBuildPromptCommandUsesSteerWhenStreaming(t *testing.T) {
	cmd := buildPromptCommand("req-1", ChatRequest{Message: "hello"}, true)
	if cmd["id"] != "req-1" || cmd["type"] != "prompt" || cmd["streamingBehavior"] != "steer" {
		t.Fatalf("cmd = %#v", cmd)
	}
}

func TestBuildPromptCommandOmitsSteerWhenIdle(t *testing.T) {
	cmd := buildPromptCommand("req-1", ChatRequest{Message: "hello"}, false)
	if _, ok := cmd["streamingBehavior"]; ok {
		t.Fatalf("streamingBehavior present for idle command")
	}
}

func TestWriteRPCCommandWritesJSONLine(t *testing.T) {
	var buf bytes.Buffer
	if err := writeRPCCommand(&buf, map[string]any{"type": "get_state"}); err != nil {
		t.Fatalf("writeRPCCommand error: %v", err)
	}
	if got := buf.String(); got != "{\"type\":\"get_state\"}\n" {
		t.Fatalf("output = %q", got)
	}
	var decoded map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(buf.Bytes()), &decoded); err != nil {
		t.Fatalf("invalid json: %v", err)
	}
}

func TestReadJSONLLinesPropagatesNonEOF(t *testing.T) {
	_, err := readJSONLLines(errReader{})
	if err == nil {
		t.Fatalf("expected error")
	}
}

type errReader struct{}

func (errReader) Read([]byte) (int, error) { return 0, errors.New("boom") }
