# Web Chat Resume Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a compact browser chat composer that resumes pi sessions through headless pi RPC workers, with image attachments, parallel per-session workers, and Tailscale-preferred binding.

**Architecture:** Split new behavior out of `main.go` into focused Go files: network binding, session lookup/chat validation, RPC JSONL client, worker manager, and chat HTTP handler. Keep the existing renderer and live reload path; chat writes happen through pi RPC and the existing JSONL watcher updates the browser.

**Tech Stack:** Go standard library HTTP/multipart/exec/net, pi RPC JSONL protocol, existing embedded HTML/CSS/JS templates.

---

## File Structure

- Create `network.go`: Tailscale IP detection and bind host selection.
- Create `network_test.go`: TDD coverage for host selection.
- Create `session_lookup.go`: resolve a session ID to the `Session` and absolute JSONL path without trusting user input as a path.
- Create `session_lookup_test.go`: path traversal and valid lookup coverage.
- Create `chat_request.go`: multipart chat request parser, image validation, payload limits.
- Create `chat_request_test.go`: empty, image, non-image, and oversized request tests.
- Create `rpc_client.go`: protocol-level JSONL reader/writer and command response correlation helpers.
- Create `rpc_client_test.go`: JSONL framing and command shape tests.
- Create `worker_manager.go`: per-session worker interface, manager, status, prompt routing, and real pi RPC process worker.
- Create `worker_manager_test.go`: parallel worker and steering behavior tests using fake workers.
- Create `chat_handler.go`: `/api/chat` and `/api/worker-status` HTTP handlers.
- Create `chat_handler_test.go`: endpoint behavior with fake manager.
- Modify `main.go`: wire new flags, host selection, server fields, routes, and small safe rendering fixes.
- Modify `templates/template.css`: compact composer styles matching current theme.
- Modify `templates/template.html`: add a `{{CHAT_COMPOSER}}` insertion point near the end of `#app`.
- Modify `templates/template.js`: composer behavior for Enter/Shift+Enter, image selection, send, status display.
- Modify `README.md`: chat usage, Tailscale binding, `--host`, no-auth warning, parallel worker warning.

---

### Task 1: Network Binding

**Files:**
- Create: `network.go`
- Create: `network_test.go`
- Modify: `main.go:544-575`

- [ ] **Step 1: Write the failing tests**

Create `network_test.go`:

```go
package main

import (
	"net"
	"testing"
)

func TestChooseBindHostUsesOverride(t *testing.T) {
	host, usedTailscale := chooseBindHost("192.168.1.50", func() (string, bool) {
		return "100.64.0.10", true
	})

	if host != "192.168.1.50" {
		t.Fatalf("host = %q, want override", host)
	}
	if usedTailscale {
		t.Fatalf("usedTailscale = true, want false for manual override")
	}
}

func TestChooseBindHostPrefersTailscale(t *testing.T) {
	host, usedTailscale := chooseBindHost("", func() (string, bool) {
		return "100.64.0.10", true
	})

	if host != "100.64.0.10" {
		t.Fatalf("host = %q, want tailscale address", host)
	}
	if !usedTailscale {
		t.Fatalf("usedTailscale = false, want true")
	}
}

func TestChooseBindHostFallsBackLocalhost(t *testing.T) {
	host, usedTailscale := chooseBindHost("", func() (string, bool) {
		return "", false
	})

	if host != "127.0.0.1" {
		t.Fatalf("host = %q, want localhost", host)
	}
	if usedTailscale {
		t.Fatalf("usedTailscale = true, want false")
	}
}

func TestIsTailscaleIP(t *testing.T) {
	cases := []struct {
		ip   string
		want bool
	}{
		{"100.64.0.1", true},
		{"100.127.255.254", true},
		{"100.128.0.1", false},
		{"192.168.1.10", false},
		{"fd7a:115c:a1e0::1", true},
		{"fe80::1", false},
	}

	for _, tc := range cases {
		got := isTailscaleIP(net.ParseIP(tc.ip))
		if got != tc.want {
			t.Fatalf("isTailscaleIP(%q) = %v, want %v", tc.ip, got, tc.want)
		}
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run:

```bash
go test ./... -run 'TestChooseBindHost|TestIsTailscaleIP'
```

Expected: FAIL with undefined functions `chooseBindHost` and `isTailscaleIP`.

- [ ] **Step 3: Implement network binding helpers**

Create `network.go`:

```go
package main

import "net"

func chooseBindHost(override string, detect func() (string, bool)) (string, bool) {
	if override != "" {
		return override, false
	}
	if host, ok := detect(); ok {
		return host, true
	}
	return "127.0.0.1", false
}

func detectTailscaleIP() (string, bool) {
	ifaces, err := net.Interfaces()
	if err != nil {
		return "", false
	}
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			var ip net.IP
			switch v := addr.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			}
			if isTailscaleIP(ip) {
				return ip.String(), true
			}
		}
	}
	return "", false
}

func isTailscaleIP(ip net.IP) bool {
	if ip == nil {
		return false
	}
	if v4 := ip.To4(); v4 != nil {
		return v4[0] == 100 && v4[1] >= 64 && v4[1] <= 127
	}
	return ip.IsPrivate() && len(ip) == net.IPv6len && ip[0] == 0xfd && ip[1] == 0x7a && ip[2] == 0x11 && ip[3] == 0x5c && ip[4] == 0xa1 && ip[5] == 0xe0
}
```

Modify `main.go` flag/bind block:

```go
func main() {
	port := flag.String("p", defaultPort, "port to listen on")
	hostOverride := flag.String("host", "", "host/IP to bind; defaults to Tailscale IP when available, otherwise 127.0.0.1")
	open := flag.Bool("o", false, "auto-open browser")
	flag.Parse()

	sessionsDir := filepath.Join(os.Getenv("HOME"), ".pi", "agent", "sessions")
	if _, err := os.Stat(sessionsDir); os.IsNotExist(err) {
		fmt.Fprintf(os.Stderr, "sessions directory not found: %s\n", sessionsDir)
		os.Exit(1)
	}

	bindHost, usedTailscale := chooseBindHost(*hostOverride, detectTailscaleIP)
	srv := newServer(sessionsDir)
	http.HandleFunc("/", srv.handleIndex)
	http.HandleFunc("/session", srv.handleSession)
	http.HandleFunc("/api/session", srv.handleApiSession)
	http.HandleFunc("/api/chat", srv.handleChat)
	http.HandleFunc("/api/worker-status", srv.handleWorkerStatus)
	http.HandleFunc("/share", srv.handleShare)
	http.HandleFunc("/events", srv.handleEvents)

	addr := net.JoinHostPort(bindHost, *port)
	url := "http://" + addr
	fmt.Printf("Pi Sessions Viewer -> %s\n", url)
	if !usedTailscale && *hostOverride == "" {
		fmt.Println("Tailscale IP not detected; using localhost.")
	}
	fmt.Printf("Serving from: %s\n", sessionsDir)

	if *open {
		go func() {
			time.Sleep(300 * time.Millisecond)
			openBrowser(url)
		}()
	}

	if err := http.ListenAndServe(addr, nil); err != nil {
		fmt.Fprintf(os.Stderr, "server error: %v\n", err)
		os.Exit(1)
	}
}
```

Also add `net` to `main.go` imports.

- [ ] **Step 4: Run tests to verify pass**

Run:

```bash
go test ./... -run 'TestChooseBindHost|TestIsTailscaleIP'
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
gofmt -w network.go network_test.go main.go
go test ./...
git add network.go network_test.go main.go
git commit -m "feat: prefer tailscale bind host"
```

---

### Task 2: Safe Session Lookup

**Files:**
- Create: `session_lookup.go`
- Create: `session_lookup_test.go`

- [ ] **Step 1: Write the failing tests**

Create `session_lookup_test.go`:

```go
package main

import (
	"os"
	"path/filepath"
	"testing"
)

func writeSessionFile(t *testing.T, root, project, name string) string {
	t.Helper()
	dir := filepath.Join(root, project)
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, name)
	content := `{"type":"session","version":3,"id":"sid","timestamp":"2026-05-06T00:00:00.000Z","cwd":"/tmp/project"}` + "\n" +
		`{"type":"message","id":"aaaaaaaa","parentId":null,"timestamp":"2026-05-06T00:00:01.000Z","message":{"role":"user","content":"hello","timestamp":1778025601000}}` + "\n"
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestResolveSessionByIDReturnsKnownPath(t *testing.T) {
	root := t.TempDir()
	wantPath := writeSessionFile(t, root, "--tmp--project--", "session.jsonl")

	resolved, err := resolveSessionByID(root, "session.jsonl")
	if err != nil {
		t.Fatalf("resolveSessionByID returned error: %v", err)
	}
	if resolved.Session.ID != "session.jsonl" {
		t.Fatalf("ID = %q, want session.jsonl", resolved.Session.ID)
	}
	if resolved.Path != wantPath {
		t.Fatalf("Path = %q, want %q", resolved.Path, wantPath)
	}
}

func TestResolveSessionByIDRejectsTraversal(t *testing.T) {
	root := t.TempDir()
	writeSessionFile(t, root, "--tmp--project--", "session.jsonl")

	_, err := resolveSessionByID(root, "../session.jsonl")
	if err == nil {
		t.Fatalf("resolveSessionByID accepted traversal id")
	}
}

func TestResolveSessionByIDRejectsUnknown(t *testing.T) {
	root := t.TempDir()
	writeSessionFile(t, root, "--tmp--project--", "session.jsonl")

	_, err := resolveSessionByID(root, "missing.jsonl")
	if err == nil {
		t.Fatalf("resolveSessionByID accepted unknown id")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run:

```bash
go test ./... -run TestResolveSessionByID
```

Expected: FAIL with undefined `resolveSessionByID`.

- [ ] **Step 3: Implement safe lookup**

Create `session_lookup.go`:

```go
package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
)

type ResolvedSession struct {
	Session Session
	Path    string
}

var errSessionNotFound = errors.New("session not found")
var errInvalidSessionID = errors.New("invalid session id")

func resolveSessionByID(sessionsDir, id string) (ResolvedSession, error) {
	if id == "" || filepath.Base(id) != id || filepath.Ext(id) != ".jsonl" {
		return ResolvedSession{}, errInvalidSessionID
	}

	entries, err := os.ReadDir(sessionsDir)
	if err != nil {
		return ResolvedSession{}, err
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		subDir := filepath.Join(sessionsDir, e.Name())
		subs, err := os.ReadDir(subDir)
		if err != nil {
			continue
		}
		for _, f := range subs {
			if f.IsDir() || f.Name() != id || !strings.HasSuffix(f.Name(), ".jsonl") {
				continue
			}
			path := filepath.Join(subDir, f.Name())
			sess, err := parseSession(path, e.Name(), f.Name())
			if err != nil {
				return ResolvedSession{}, err
			}
			return ResolvedSession{Session: sess, Path: path}, nil
		}
	}
	return ResolvedSession{}, errSessionNotFound
}
```

- [ ] **Step 4: Run tests to verify pass**

Run:

```bash
go test ./... -run TestResolveSessionByID
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
gofmt -w session_lookup.go session_lookup_test.go
go test ./...
git add session_lookup.go session_lookup_test.go
git commit -m "feat: resolve sessions safely for chat"
```

---

### Task 3: Chat Request Validation

**Files:**
- Create: `chat_request.go`
- Create: `chat_request_test.go`

- [ ] **Step 1: Write the failing tests**

Create `chat_request_test.go`:

```go
package main

import (
	"bytes"
	"mime/multipart"
	"net/http"
	"strings"
	"testing"
)

func multipartRequest(t *testing.T, message string, files map[string]struct{ name, contentType, body string }) *http.Request {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	if message != "" {
		if err := mw.WriteField("message", message); err != nil {
			t.Fatal(err)
		}
	}
	for field, file := range files {
		part, err := mw.CreateFormFile(field, file.name)
		if err != nil {
			t.Fatal(err)
		}
		part.Write([]byte(file.body))
	}
	if err := mw.Close(); err != nil {
		t.Fatal(err)
	}
	req, err := http.NewRequest(http.MethodPost, "/api/chat?id=session.jsonl", &buf)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())
	return req
}

func TestParseChatRequestAcceptsTextOnly(t *testing.T) {
	req := multipartRequest(t, "hello", nil)
	chat, err := parseChatRequest(req, 1024, 4096)
	if err != nil {
		t.Fatalf("parseChatRequest error: %v", err)
	}
	if chat.Message != "hello" {
		t.Fatalf("Message = %q, want hello", chat.Message)
	}
	if len(chat.Images) != 0 {
		t.Fatalf("Images length = %d, want 0", len(chat.Images))
	}
}

func TestParseChatRequestAcceptsImage(t *testing.T) {
	req := multipartRequest(t, "describe", map[string]struct{ name, contentType, body string }{
		"images": {"a.png", "image/png", "\x89PNG\r\n\x1a\nimage"},
	})
	chat, err := parseChatRequest(req, 1024, 4096)
	if err != nil {
		t.Fatalf("parseChatRequest error: %v", err)
	}
	if len(chat.Images) != 1 {
		t.Fatalf("Images length = %d, want 1", len(chat.Images))
	}
	if chat.Images[0].MimeType != "image/png" {
		t.Fatalf("MimeType = %q, want image/png", chat.Images[0].MimeType)
	}
}

func TestParseChatRequestRejectsEmpty(t *testing.T) {
	req := multipartRequest(t, "", nil)
	_, err := parseChatRequest(req, 1024, 4096)
	if err != errEmptyChatRequest {
		t.Fatalf("err = %v, want errEmptyChatRequest", err)
	}
}

func TestParseChatRequestRejectsNonImage(t *testing.T) {
	req := multipartRequest(t, "see file", map[string]struct{ name, contentType, body string }{
		"images": {"a.txt", "text/plain", "plain text"},
	})
	_, err := parseChatRequest(req, 1024, 4096)
	if err != errUnsupportedImageType {
		t.Fatalf("err = %v, want errUnsupportedImageType", err)
	}
}

func TestParseChatRequestRejectsOversizedImage(t *testing.T) {
	req := multipartRequest(t, "big", map[string]struct{ name, contentType, body string }{
		"images": {"a.png", "image/png", "\x89PNG\r\n\x1a\n" + strings.Repeat("x", 20)},
	})
	_, err := parseChatRequest(req, 8, 4096)
	if err != errImageTooLarge {
		t.Fatalf("err = %v, want errImageTooLarge", err)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run:

```bash
go test ./... -run TestParseChatRequest
```

Expected: FAIL with undefined `parseChatRequest` and error variables.

- [ ] **Step 3: Implement request parsing**

Create `chat_request.go`:

```go
package main

import (
	"encoding/base64"
	"errors"
	"io"
	"net/http"
	"strings"
)

const defaultMaxImageBytes int64 = 10 << 20
const defaultMaxChatRequestBytes int64 = 32 << 20

var errEmptyChatRequest = errors.New("message or image required")
var errUnsupportedImageType = errors.New("only image attachments are supported")
var errImageTooLarge = errors.New("image attachment too large")

type ChatImage struct {
	Type     string `json:"type"`
	Data     string `json:"data"`
	MimeType string `json:"mimeType"`
}

type ChatRequest struct {
	Message string
	Images  []ChatImage
}

func parseChatRequest(r *http.Request, maxImageBytes, maxRequestBytes int64) (ChatRequest, error) {
	r.Body = http.MaxBytesReader(nil, r.Body, maxRequestBytes)
	if err := r.ParseMultipartForm(maxRequestBytes); err != nil {
		return ChatRequest{}, err
	}

	chat := ChatRequest{Message: strings.TrimSpace(r.FormValue("message"))}
	files := r.MultipartForm.File["images"]
	for _, fh := range files {
		file, err := fh.Open()
		if err != nil {
			return ChatRequest{}, err
		}
		data, readErr := io.ReadAll(io.LimitReader(file, maxImageBytes+1))
		closeErr := file.Close()
		if readErr != nil {
			return ChatRequest{}, readErr
		}
		if closeErr != nil {
			return ChatRequest{}, closeErr
		}
		if int64(len(data)) > maxImageBytes {
			return ChatRequest{}, errImageTooLarge
		}
		mimeType := http.DetectContentType(data)
		if !strings.HasPrefix(mimeType, "image/") {
			return ChatRequest{}, errUnsupportedImageType
		}
		chat.Images = append(chat.Images, ChatImage{
			Type:     "image",
			Data:     base64.StdEncoding.EncodeToString(data),
			MimeType: mimeType,
		})
	}

	if chat.Message == "" && len(chat.Images) == 0 {
		return ChatRequest{}, errEmptyChatRequest
	}
	return chat, nil
}
```

- [ ] **Step 4: Run tests to verify pass**

Run:

```bash
go test ./... -run TestParseChatRequest
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
gofmt -w chat_request.go chat_request_test.go
go test ./...
git add chat_request.go chat_request_test.go
git commit -m "feat: validate browser chat requests"
```

---

### Task 4: RPC JSONL Client Primitives

**Files:**
- Create: `rpc_client.go`
- Create: `rpc_client_test.go`

- [ ] **Step 1: Write the failing tests**

Create `rpc_client_test.go`:

```go
package main

import (
	"bytes"
	"encoding/json"
	"io"
	"testing"
)

func TestSplitJSONLLinesHandlesCRLF(t *testing.T) {
	lines, err := readJSONLLines(bytes.NewBufferString("{\"a\":1}\r\n{\"b\":2}\n"))
	if err != nil {
		t.Fatalf("readJSONLLines error: %v", err)
	}
	if len(lines) != 2 {
		t.Fatalf("len(lines) = %d, want 2", len(lines))
	}
	if lines[0] != `{"a":1}` || lines[1] != `{"b":2}` {
		t.Fatalf("lines = %#v", lines)
	}
}

func TestBuildPromptCommandUsesSteerWhenStreaming(t *testing.T) {
	cmd := buildPromptCommand("req-1", ChatRequest{Message: "hello"}, true)
	if cmd["id"] != "req-1" {
		t.Fatalf("id = %v", cmd["id"])
	}
	if cmd["type"] != "prompt" {
		t.Fatalf("type = %v", cmd["type"])
	}
	if cmd["streamingBehavior"] != "steer" {
		t.Fatalf("streamingBehavior = %v, want steer", cmd["streamingBehavior"])
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

func (errReader) Read([]byte) (int, error) { return 0, io.ErrUnexpectedEOF }
```

- [ ] **Step 2: Run tests to verify they fail**

Run:

```bash
go test ./... -run 'TestSplitJSONL|TestBuildPromptCommand|TestWriteRPCCommand|TestReadJSONLLines'
```

Expected: FAIL with undefined RPC helper functions.

- [ ] **Step 3: Implement RPC helpers**

Create `rpc_client.go`:

```go
package main

import (
	"bufio"
	"encoding/json"
	"io"
	"strings"
)

type rpcResponse struct {
	ID      string          `json:"id"`
	Type    string          `json:"type"`
	Command string          `json:"command"`
	Success bool            `json:"success"`
	Data    json.RawMessage `json:"data"`
	Error   string          `json:"error"`
}

func readJSONLLines(r io.Reader) ([]string, error) {
	scanner := bufio.NewScanner(r)
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 10*1024*1024)
	var lines []string
	for scanner.Scan() {
		line := scanner.Text()
		line = strings.TrimSuffix(line, "\r")
		if strings.TrimSpace(line) != "" {
			lines = append(lines, line)
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return lines, nil
}

func writeRPCCommand(w io.Writer, cmd map[string]any) error {
	data, err := json.Marshal(cmd)
	if err != nil {
		return err
	}
	_, err = w.Write(append(data, '\n'))
	return err
}

func buildSwitchSessionCommand(id, sessionPath string) map[string]any {
	return map[string]any{"id": id, "type": "switch_session", "sessionPath": sessionPath}
}

func buildPromptCommand(id string, chat ChatRequest, streaming bool) map[string]any {
	cmd := map[string]any{
		"id":      id,
		"type":    "prompt",
		"message": chat.Message,
	}
	if len(chat.Images) > 0 {
		cmd["images"] = chat.Images
	}
	if streaming {
		cmd["streamingBehavior"] = "steer"
	}
	return cmd
}
```

- [ ] **Step 4: Run tests to verify pass**

Run:

```bash
go test ./... -run 'TestSplitJSONL|TestBuildPromptCommand|TestWriteRPCCommand|TestReadJSONLLines'
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
gofmt -w rpc_client.go rpc_client_test.go
go test ./...
git add rpc_client.go rpc_client_test.go
git commit -m "feat: add pi rpc protocol helpers"
```

---

### Task 5: Worker Manager and Fakeable Worker Interface

**Files:**
- Create: `worker_manager.go`
- Create: `worker_manager_test.go`

- [ ] **Step 1: Write the failing tests**

Create `worker_manager_test.go`:

```go
package main

import (
	"context"
	"sync"
	"testing"
)

type fakeChatWorker struct {
	mu        sync.Mutex
	streaming bool
	prompts   []map[string]any
}

func (f *fakeChatWorker) Prompt(ctx context.Context, chat ChatRequest) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	cmd := buildPromptCommand("test", chat, f.streaming)
	f.prompts = append(f.prompts, cmd)
	f.streaming = true
	return nil
}

func (f *fakeChatWorker) Status() WorkerStatus {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.streaming {
		return WorkerStatus{State: WorkerStateRunning}
	}
	return WorkerStatus{State: WorkerStateIdle}
}

func (f *fakeChatWorker) Close() error { return nil }

func TestWorkerManagerCreatesOneWorkerPerSession(t *testing.T) {
	created := 0
	manager := NewWorkerManager(func(sessionPath string) (ChatWorker, error) {
		created++
		return &fakeChatWorker{}, nil
	})

	ctx := context.Background()
	if err := manager.Send(ctx, "a.jsonl", "/tmp/a.jsonl", ChatRequest{Message: "a"}); err != nil {
		t.Fatal(err)
	}
	if err := manager.Send(ctx, "b.jsonl", "/tmp/b.jsonl", ChatRequest{Message: "b"}); err != nil {
		t.Fatal(err)
	}
	if err := manager.Send(ctx, "a.jsonl", "/tmp/a.jsonl", ChatRequest{Message: "again"}); err != nil {
		t.Fatal(err)
	}

	if created != 2 {
		t.Fatalf("created workers = %d, want 2", created)
	}
}

func TestWorkerManagerReportsMissingWorkerIdle(t *testing.T) {
	manager := NewWorkerManager(func(sessionPath string) (ChatWorker, error) {
		return &fakeChatWorker{}, nil
	})
	status := manager.Status("missing.jsonl")
	if status.State != WorkerStateIdle {
		t.Fatalf("status = %q, want idle", status.State)
	}
}

func TestBusyWorkerUsesSteeringCommand(t *testing.T) {
	worker := &fakeChatWorker{streaming: true}
	manager := NewWorkerManager(func(sessionPath string) (ChatWorker, error) {
		return worker, nil
	})

	if err := manager.Send(context.Background(), "a.jsonl", "/tmp/a.jsonl", ChatRequest{Message: "steer"}); err != nil {
		t.Fatal(err)
	}

	worker.mu.Lock()
	defer worker.mu.Unlock()
	if len(worker.prompts) != 1 {
		t.Fatalf("prompts = %d, want 1", len(worker.prompts))
	}
	if worker.prompts[0]["streamingBehavior"] != "steer" {
		t.Fatalf("streamingBehavior = %v, want steer", worker.prompts[0]["streamingBehavior"])
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run:

```bash
go test ./... -run 'TestWorkerManager|TestBusyWorker'
```

Expected: FAIL with undefined `WorkerManager`, `ChatWorker`, and status types.

- [ ] **Step 3: Implement manager and real worker skeleton**

Create `worker_manager.go`:

```go
package main

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"sync"
	"sync/atomic"
)

type WorkerState string

const (
	WorkerStateIdle    WorkerState = "idle"
	WorkerStateRunning WorkerState = "running"
	WorkerStateError   WorkerState = "error"
)

type WorkerStatus struct {
	State WorkerState `json:"state"`
	Error string      `json:"error,omitempty"`
}

type ChatWorker interface {
	Prompt(ctx context.Context, chat ChatRequest) error
	Status() WorkerStatus
	Close() error
}

type WorkerFactory func(sessionPath string) (ChatWorker, error)

type WorkerManager struct {
	mu      sync.Mutex
	workers map[string]ChatWorker
	factory WorkerFactory
}

func NewWorkerManager(factory WorkerFactory) *WorkerManager {
	return &WorkerManager{workers: make(map[string]ChatWorker), factory: factory}
}

func (m *WorkerManager) Send(ctx context.Context, sessionID, sessionPath string, chat ChatRequest) error {
	worker, err := m.workerFor(sessionID, sessionPath)
	if err != nil {
		return err
	}
	return worker.Prompt(ctx, chat)
}

func (m *WorkerManager) Status(sessionID string) WorkerStatus {
	m.mu.Lock()
	worker := m.workers[sessionID]
	m.mu.Unlock()
	if worker == nil {
		return WorkerStatus{State: WorkerStateIdle}
	}
	return worker.Status()
}

func (m *WorkerManager) Close() error {
	m.mu.Lock()
	workers := make([]ChatWorker, 0, len(m.workers))
	for _, worker := range m.workers {
		workers = append(workers, worker)
	}
	m.workers = make(map[string]ChatWorker)
	m.mu.Unlock()
	var result error
	for _, worker := range workers {
		if err := worker.Close(); err != nil {
			result = errors.Join(result, err)
		}
	}
	return result
}

func (m *WorkerManager) workerFor(sessionID, sessionPath string) (ChatWorker, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if worker := m.workers[sessionID]; worker != nil {
		return worker, nil
	}
	worker, err := m.factory(sessionPath)
	if err != nil {
		return nil, err
	}
	m.workers[sessionID] = worker
	return worker, nil
}

type piRPCWorker struct {
	mu          sync.Mutex
	sessionPath string
	cmd         *exec.Cmd
	stdin       ioWriteCloser
	status      WorkerStatus
	seq         atomic.Uint64
}

type ioWriteCloser interface {
	Write([]byte) (int, error)
	Close() error
}

func newPiRPCWorker(sessionPath string) (ChatWorker, error) {
	if _, err := exec.LookPath("pi"); err != nil {
		return nil, fmt.Errorf("pi executable not found: %w", err)
	}
	cmd := exec.Command("pi", "--mode", "rpc")
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	worker := &piRPCWorker{sessionPath: sessionPath, cmd: cmd, stdin: stdin, status: WorkerStatus{State: WorkerStateIdle}}
	go worker.consume(stdout)
	if err := worker.switchSession(context.Background()); err != nil {
		worker.Close()
		return nil, err
	}
	return worker, nil
}
```

Add missing imports to `worker_manager.go`: `encoding/json`, `io`, and `time` when implementing the real worker methods in the next task. Keep this task focused on manager tests; real worker methods can return deterministic errors until Task 6 covers them if compilation requires it.

- [ ] **Step 4: Add minimal real worker methods for compilation**

Append to `worker_manager.go`:

```go
func (w *piRPCWorker) Prompt(ctx context.Context, chat ChatRequest) error {
	w.mu.Lock()
	streaming := w.status.State == WorkerStateRunning
	w.status = WorkerStatus{State: WorkerStateRunning}
	w.mu.Unlock()
	id := w.nextID()
	return writeRPCCommand(w.stdin, buildPromptCommand(id, chat, streaming))
}

func (w *piRPCWorker) Status() WorkerStatus {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.status
}

func (w *piRPCWorker) Close() error {
	if w.stdin != nil {
		_ = w.stdin.Close()
	}
	if w.cmd != nil && w.cmd.Process != nil {
		_ = w.cmd.Process.Kill()
	}
	return nil
}

func (w *piRPCWorker) nextID() string {
	return fmt.Sprintf("req-%d", w.seq.Add(1))
}

func (w *piRPCWorker) switchSession(ctx context.Context) error {
	return writeRPCCommand(w.stdin, buildSwitchSessionCommand(w.nextID(), w.sessionPath))
}

func (w *piRPCWorker) consume(r io.Reader) {
	lines, err := readJSONLLines(r)
	w.mu.Lock()
	defer w.mu.Unlock()
	if err != nil {
		w.status = WorkerStatus{State: WorkerStateError, Error: err.Error()}
		return
	}
	for _, line := range lines {
		var event map[string]any
		if json.Unmarshal([]byte(line), &event) == nil && event["type"] == "agent_end" {
			w.status = WorkerStatus{State: WorkerStateIdle}
		}
	}
}
```

Ensure `worker_manager.go` imports include:

```go
import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"sync"
	"sync/atomic"
)
```

- [ ] **Step 5: Run tests to verify pass**

Run:

```bash
go test ./... -run 'TestWorkerManager|TestBusyWorker'
```

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
gofmt -w worker_manager.go worker_manager_test.go
go test ./...
git add worker_manager.go worker_manager_test.go
git commit -m "feat: manage parallel pi chat workers"
```

---

### Task 6: Chat HTTP Handlers

**Files:**
- Create: `chat_handler.go`
- Create: `chat_handler_test.go`
- Modify: `main.go:603-617`, `main.go:554-560`

- [ ] **Step 1: Write the failing tests**

Create `chat_handler_test.go`:

```go
package main

import (
	"bytes"
	"context"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

type fakeSender struct {
	sessionID   string
	sessionPath string
	chat        ChatRequest
}

func (f *fakeSender) Send(ctx context.Context, sessionID, sessionPath string, chat ChatRequest) error {
	f.sessionID = sessionID
	f.sessionPath = sessionPath
	f.chat = chat
	return nil
}

func (f *fakeSender) Status(sessionID string) WorkerStatus { return WorkerStatus{State: WorkerStateIdle} }

func TestHandleChatSendsResolvedSession(t *testing.T) {
	root := t.TempDir()
	wantPath := writeSessionFile(t, root, "--tmp--project--", "session.jsonl")
	fake := &fakeSender{}
	s := &server{sessionsDir: root, chatSender: fake}

	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	mw.WriteField("message", "hello")
	mw.Close()
	req := httptest.NewRequest(http.MethodPost, "/api/chat?id=session.jsonl", &body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	w := httptest.NewRecorder()

	s.handleChat(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", w.Code, w.Body.String())
	}
	if fake.sessionID != "session.jsonl" {
		t.Fatalf("sessionID = %q", fake.sessionID)
	}
	if fake.sessionPath != wantPath {
		t.Fatalf("sessionPath = %q, want %q", fake.sessionPath, wantPath)
	}
	if fake.chat.Message != "hello" {
		t.Fatalf("message = %q", fake.chat.Message)
	}
}

func TestHandleChatRejectsUnknownSession(t *testing.T) {
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, "--tmp--project--"), 0755)
	s := &server{sessionsDir: root, chatSender: &fakeSender{}}

	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	mw.WriteField("message", "hello")
	mw.Close()
	req := httptest.NewRequest(http.MethodPost, "/api/chat?id=missing.jsonl", &body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	w := httptest.NewRecorder()

	s.handleChat(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", w.Code)
	}
}

func TestHandleWorkerStatusDefaultsIdle(t *testing.T) {
	s := &server{sessionsDir: t.TempDir(), chatSender: &fakeSender{}}
	req := httptest.NewRequest(http.MethodGet, "/api/worker-status?id=session.jsonl", nil)
	w := httptest.NewRecorder()

	s.handleWorkerStatus(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d", w.Code)
	}
	if got := w.Body.String(); got != "{\"state\":\"idle\"}\n" {
		t.Fatalf("body = %q", got)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run:

```bash
go test ./... -run 'TestHandleChat|TestHandleWorkerStatus'
```

Expected: FAIL with missing handler and `chatSender` fields.

- [ ] **Step 3: Implement handlers and interfaces**

Create `chat_handler.go`:

```go
package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
)

type ChatSender interface {
	Send(ctx context.Context, sessionID, sessionPath string, chat ChatRequest) error
	Status(sessionID string) WorkerStatus
}

func (s *server) handleChat(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	id := r.URL.Query().Get("id")
	resolved, err := resolveSessionByID(s.sessionsDir, id)
	if err != nil {
		if errors.Is(err, errInvalidSessionID) {
			writeJSONError(w, http.StatusBadRequest, "invalid session id")
			return
		}
		if errors.Is(err, errSessionNotFound) {
			writeJSONError(w, http.StatusNotFound, "session not found")
			return
		}
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	chat, err := parseChatRequest(r, defaultMaxImageBytes, defaultMaxChatRequestBytes)
	if err != nil {
		switch {
		case errors.Is(err, errEmptyChatRequest):
			writeJSONError(w, http.StatusBadRequest, err.Error())
		case errors.Is(err, errImageTooLarge):
			writeJSONError(w, http.StatusRequestEntityTooLarge, err.Error())
		case errors.Is(err, errUnsupportedImageType):
			writeJSONError(w, http.StatusUnsupportedMediaType, err.Error())
		default:
			writeJSONError(w, http.StatusBadRequest, err.Error())
		}
		return
	}
	if err := s.chatSender.Send(r.Context(), resolved.Session.ID, resolved.Path, chat); err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"ok": true, "status": "accepted"})
}

func (s *server) handleWorkerStatus(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(s.chatSender.Status(id))
}

func writeJSONError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]any{"error": message})
}
```

Modify `server` in `main.go`:

```go
type server struct {
	sessionsDir string
	clients     []*sseClient
	clientsMu   sync.RWMutex
	fileMod     map[string]time.Time
	fileModMu   sync.RWMutex
	chatSender  ChatSender
}
```

Modify `newServer` in `main.go`:

```go
func newServer(sessionsDir string) *server {
	s := &server{
		sessionsDir: sessionsDir,
		clients:     make([]*sseClient, 0),
		fileMod:     make(map[string]time.Time),
		chatSender:  NewWorkerManager(newPiRPCWorker),
	}
	go s.watchFiles()
	return s
}
```

Ensure `main()` registered `/api/chat` and `/api/worker-status` as in Task 1.

- [ ] **Step 4: Run tests to verify pass**

Run:

```bash
go test ./... -run 'TestHandleChat|TestHandleWorkerStatus'
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
gofmt -w chat_handler.go chat_handler_test.go main.go
go test ./...
git add chat_handler.go chat_handler_test.go main.go
git commit -m "feat: add browser chat endpoints"
```

---

### Task 7: Browser Composer UI

**Files:**
- Modify: `templates/template.html`
- Modify: `templates/template.css`
- Modify: `templates/template.js`
- Modify: `main.go:1036-1082`

- [ ] **Step 1: Write failing tests for composer injection**

Create `export_html_test.go`:

```go
package main

import (
	"strings"
	"testing"
)

func TestGenerateExportHtmlIncludesChatComposerWhenButtonsShown(t *testing.T) {
	session := Session{ID: "s.jsonl", Filename: "s.jsonl", Entries: []map[string]any{{"id": "aaaaaaaa"}}}
	html := generateExportHtml(session, true)
	if !strings.Contains(html, `id="pi-chat-composer"`) {
		t.Fatalf("chat composer missing from local session page")
	}
	if !strings.Contains(html, `data-session-id="s.jsonl"`) {
		t.Fatalf("session id missing from composer")
	}
}

func TestGenerateExportHtmlOmitsChatComposerForShare(t *testing.T) {
	session := Session{ID: "s.jsonl", Filename: "s.jsonl", Entries: []map[string]any{{"id": "aaaaaaaa"}}}
	html := generateExportHtml(session, false)
	if strings.Contains(html, `id="pi-chat-composer"`) {
		t.Fatalf("chat composer should not be included in share export")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run:

```bash
go test ./... -run TestGenerateExportHtml
```

Expected: FAIL because composer markup is missing.

- [ ] **Step 3: Add template insertion point**

Modify `templates/template.html`, immediately before `</body>`:

```html
{{CHAT_COMPOSER}}
</body>
```

- [ ] **Step 4: Add composer generation and safe leaf ID handling**

Modify `generateExportHtml` in `main.go`:

```go
func generateExportHtml(session Session, showButtons bool) string {
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

	themeVars := generateThemeVars()
	bodyBg := "#18181e"
	cardBg := "#1e1e24"
	infoBg := "#3c3728"

	css := templateCss
	css = strings.Replace(css, "{{THEME_VARS}}", themeVars, 1)
	css = strings.Replace(css, "{{BODY_BG}}", bodyBg, 1)
	css = strings.Replace(css, "{{CONTAINER_BG}}", cardBg, 1)
	css = strings.Replace(css, "{{INFO_BG}}", infoBg, 1)

	html := templateHtml
	html = strings.Replace(html, "{{CSS}}", css, 1)
	html = strings.Replace(html, "{{JS}}", templateJs, 1)
	html = strings.Replace(html, "{{SESSION_DATA}}", dataBase64, 1)
	html = strings.Replace(html, "{{MARKED_JS}}", markedJs, 1)
	html = strings.Replace(html, "{{HIGHLIGHT_JS}}", hljsJs, 1)

	if showButtons {
		btns := `<div style="position:fixed;top:10px;right:10px;z-index:101;display:flex;flex-direction:column;gap:6px;">
<a href="/" title="Back to sessions" style="padding:4px 10px;font-size:11px;font-family:inherit;background:var(--container-bg);color:var(--muted);border:1px solid var(--dim);border-radius:3px;text-decoration:none;cursor:pointer;text-align:center;">← Sessions</a>
<button id="share-btn" title="Share session as GitHub Gist" style="padding:4px 10px;font-size:11px;font-family:inherit;background:var(--container-bg);color:var(--muted);border:1px solid var(--dim);border-radius:3px;cursor:pointer;">↗ Share</button>
</div>`
		html = strings.Replace(html, "<body>", "<body>"+btns, 1)
		html = strings.Replace(html, "{{CHAT_COMPOSER}}", chatComposerHtml(session.ID), 1)
		html = strings.Replace(html, "</body>", liveReloadJs+"</body>", 1)
	} else {
		html = strings.Replace(html, "{{CHAT_COMPOSER}}", "", 1)
	}

	return html
}

func chatComposerHtml(sessionID string) string {
	return `<form id="pi-chat-composer" class="pi-chat-composer" data-session-id="` + template.HTMLEscapeString(sessionID) + `">
  <input id="pi-chat-images" name="images" type="file" accept="image/*" multiple hidden>
  <button type="button" id="pi-chat-attach" class="pi-chat-icon-button" title="Attach images">◉</button>
  <div class="pi-chat-main">
    <textarea id="pi-chat-message" name="message" rows="2" placeholder="Continue this pi session…"></textarea>
    <div id="pi-chat-attachments" class="pi-chat-attachments"></div>
    <div id="pi-chat-status" class="pi-chat-status">idle</div>
  </div>
  <button type="submit" id="pi-chat-send" class="pi-chat-send">Send</button>
</form>`
}
```

- [ ] **Step 5: Add CSS matching existing style**

Append to `templates/template.css` before the final media queries:

```css
    .pi-chat-composer {
      position: sticky;
      bottom: 0;
      z-index: 80;
      display: flex;
      align-items: flex-end;
      gap: 6px;
      padding: 8px 10px;
      background: var(--container-bg);
      border-top: 1px solid var(--dim);
    }

    .pi-chat-main {
      flex: 1;
      min-width: 0;
    }

    #pi-chat-message {
      width: 100%;
      min-height: 42px;
      max-height: 132px;
      resize: vertical;
      font-family: inherit;
      font-size: 12px;
      line-height: var(--line-height);
      background: var(--body-bg);
      color: var(--text);
      border: 1px solid var(--dim);
      border-radius: 3px;
      padding: 6px 8px;
      outline: none;
    }

    #pi-chat-message:focus {
      border-color: var(--border);
    }

    .pi-chat-icon-button,
    .pi-chat-send {
      height: 30px;
      font-family: inherit;
      font-size: 11px;
      background: var(--container-bg);
      color: var(--muted);
      border: 1px solid var(--dim);
      border-radius: 3px;
      cursor: pointer;
    }

    .pi-chat-icon-button {
      width: 30px;
      padding: 0;
    }

    .pi-chat-send {
      padding: 4px 10px;
    }

    .pi-chat-icon-button:hover,
    .pi-chat-send:hover {
      color: var(--text);
      border-color: var(--borderMuted);
    }

    .pi-chat-attachments {
      display: flex;
      flex-wrap: wrap;
      gap: 4px;
      margin-top: 4px;
    }

    .pi-chat-attachment {
      color: var(--muted);
      border: 1px solid var(--dim);
      border-radius: 3px;
      padding: 1px 5px;
      font-size: 11px;
    }

    .pi-chat-status {
      margin-top: 4px;
      color: var(--dim);
      font-size: 11px;
    }

    .pi-chat-status.error {
      color: var(--error);
    }

    .pi-chat-status.running {
      color: var(--success);
    }
```

- [ ] **Step 6: Add frontend behavior**

Append to `templates/template.js` before the closing IIFE:

```javascript
      function setupPiChatComposer() {
        const form = document.getElementById('pi-chat-composer');
        if (!form) return;
        const sessionId = form.dataset.sessionId;
        const textarea = document.getElementById('pi-chat-message');
        const fileInput = document.getElementById('pi-chat-images');
        const attachButton = document.getElementById('pi-chat-attach');
        const attachmentList = document.getElementById('pi-chat-attachments');
        const status = document.getElementById('pi-chat-status');
        const sendButton = document.getElementById('pi-chat-send');

        function setStatus(text, cls) {
          status.textContent = text;
          status.className = 'pi-chat-status' + (cls ? ' ' + cls : '');
        }

        function renderAttachments() {
          attachmentList.innerHTML = '';
          for (const file of fileInput.files) {
            const item = document.createElement('span');
            item.className = 'pi-chat-attachment';
            item.textContent = '◉ ' + file.name;
            attachmentList.appendChild(item);
          }
        }

        attachButton.addEventListener('click', () => fileInput.click());
        fileInput.addEventListener('change', renderAttachments);

        textarea.addEventListener('keydown', (event) => {
          if (event.key === 'Enter' && !event.shiftKey) {
            event.preventDefault();
            form.requestSubmit();
          }
        });

        form.addEventListener('submit', async (event) => {
          event.preventDefault();
          const message = textarea.value.trim();
          if (!message && fileInput.files.length === 0) {
            setStatus('message or image required', 'error');
            return;
          }
          const body = new FormData();
          body.set('message', message);
          for (const file of fileInput.files) {
            body.append('images', file);
          }
          sendButton.disabled = true;
          setStatus('sending', 'running');
          try {
            const response = await fetch('/api/chat?id=' + encodeURIComponent(sessionId), { method: 'POST', body });
            const data = await response.json();
            if (!response.ok) {
              throw new Error(data.error || 'chat request failed');
            }
            textarea.value = '';
            fileInput.value = '';
            renderAttachments();
            setStatus('accepted', 'running');
          } catch (error) {
            setStatus(error.message || String(error), 'error');
          } finally {
            sendButton.disabled = false;
          }
        });

        async function refreshWorkerStatus() {
          try {
            const response = await fetch('/api/worker-status?id=' + encodeURIComponent(sessionId));
            if (!response.ok) return;
            const data = await response.json();
            if (data.state === 'running') setStatus('running', 'running');
            if (data.state === 'idle') setStatus('idle', '');
            if (data.state === 'error') setStatus(data.error || 'worker error', 'error');
          } catch {
            setStatus('status unavailable', 'error');
          }
        }

        setInterval(refreshWorkerStatus, 3000);
        refreshWorkerStatus();
      }

      setupPiChatComposer();
```

- [ ] **Step 7: Run tests to verify pass**

Run:

```bash
go test ./... -run TestGenerateExportHtml
```

Expected: PASS.

- [ ] **Step 8: Commit**

```bash
gofmt -w main.go export_html_test.go
go test ./...
git add templates/template.html templates/template.css templates/template.js main.go export_html_test.go
git commit -m "feat: add browser chat composer"
```

---

### Task 8: SSE Client Cleanup Bug and Worker Status Broadcast

**Files:**
- Modify: `main.go:621-655`, `main.go:872-910`

- [ ] **Step 1: Write failing test for SSE client removal**

Create `sse_test.go`:

```go
package main

import "testing"

func TestAddRemoveClientRemovesStoredClient(t *testing.T) {
	s := &server{clients: make([]*sseClient, 0)}
	client := s.addClient("a.jsonl")
	if len(s.clients) != 1 {
		t.Fatalf("clients = %d, want 1", len(s.clients))
	}
	s.removeClient(client)
	if len(s.clients) != 0 {
		t.Fatalf("clients = %d, want 0", len(s.clients))
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run:

```bash
go test ./... -run TestAddRemoveClientRemovesStoredClient
```

Expected: FAIL because `addClient` currently returns only a channel and `handleEvents` constructs a different client pointer.

- [ ] **Step 3: Fix client ownership**

Change `addClient` signature in `main.go`:

```go
func (s *server) addClient(sessID string) *sseClient {
	c := &sseClient{ch: make(chan string, 4), sessID: sessID}
	s.clientsMu.Lock()
	s.clients = append(s.clients, c)
	s.clientsMu.Unlock()
	return c
}
```

Change `handleEvents` client setup:

```go
client := s.addClient(sessID)
defer s.removeClient(client)

fmt.Fprintf(w, ":ok\n\n")
flusher.Flush()

for {
	select {
	case msg, open := <-client.ch:
		if !open {
			return
		}
		fmt.Fprintf(w, "data: %s\n\n", msg)
		flusher.Flush()
	case <-r.Context().Done():
		return
	}
}
```

- [ ] **Step 4: Run test to verify pass**

Run:

```bash
go test ./... -run TestAddRemoveClientRemovesStoredClient
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
gofmt -w main.go sse_test.go
go test ./...
git add main.go sse_test.go
git commit -m "fix: remove disconnected sse clients"
```

---

### Task 9: README Documentation

**Files:**
- Modify: `README.md`

- [ ] **Step 1: Update usage documentation**

Add under `## Usage` after custom port examples:

```markdown
### Network binding

By default, the viewer tries to bind to your Tailscale IP. If Tailscale is not available, it falls back to `127.0.0.1`.

```bash
# Prefer Tailscale IP, fallback localhost
pi-sessions-viewer

# Bind manually
pi-sessions-viewer --host 127.0.0.1
pi-sessions-viewer --host 100.x.y.z
```

Warning: v1 has no authentication. Anyone who can reach the bound address can view sessions and send instructions to pi.
```

Add under Pi integration or a new `## Browser chat` section:

```markdown
## Browser chat

Session pages include a compact composer at the bottom. Type instructions and press Enter to continue the same pi session from the browser. Use Shift+Enter for a newline.

The image icon attaches images. v1 supports image attachments only; arbitrary files are not uploaded.

Each active session gets its own headless `pi --mode rpc` worker. Multiple sessions can run in parallel, including sessions from different projects. Be careful: parallel agents may edit files concurrently.
```

- [ ] **Step 2: Verify docs render plainly**

Run:

```bash
rg -n "Browser chat|Network binding|no authentication|parallel" README.md
```

Expected: matching lines for all new warnings/features.

- [ ] **Step 3: Commit**

```bash
git add README.md
git commit -m "docs: document browser chat and tailscale binding"
```

---

### Task 10: Final Verification

**Files:**
- All modified files

- [ ] **Step 1: Format everything**

Run:

```bash
gofmt -w *.go
```

Expected: no output.

- [ ] **Step 2: Run tests**

Run:

```bash
go test ./...
```

Expected: PASS.

- [ ] **Step 3: Run vet**

Run:

```bash
go vet ./...
```

Expected: no output.

- [ ] **Step 4: Check formatting list**

Run:

```bash
gofmt -l .
```

Expected: no output.

- [ ] **Step 5: Manual smoke test**

Run:

```bash
go build -o pi-sessions-viewer .
./pi-sessions-viewer -p 31483
```

Expected startup prints either a Tailscale URL or localhost fallback. Open a session page and verify:

- composer appears at the bottom,
- image icon opens a file picker,
- empty send shows an error,
- worker status starts as idle,
- existing session browsing and sharing still render.

Do not send a real prompt during automated verification unless the user explicitly wants model/API usage.

- [ ] **Step 6: Review git diff**

Run:

```bash
git diff --stat HEAD
```

Expected: only planned files changed.

- [ ] **Step 7: Commit final build-related changes if any**

```bash
git add .
git commit -m "chore: verify web chat resume feature"
```

Only run this commit if there are verification-only changes not included in previous commits.

---

## Self-Review

Spec coverage:

- Browser composer: Task 7.
- Text and image chat endpoint: Tasks 3 and 6.
- Headless pi RPC workers: Tasks 4 and 5.
- Parallel workers: Task 5.
- Same-session steering: Tasks 4 and 5.
- Tailscale-preferred binding: Task 1.
- No-auth warning and docs: Task 9.
- Existing SSE/live reload preserved and fixed: Task 8.

Placeholder scan: no implementation steps contain placeholder markers or unspecified error handling.

Type consistency: `ChatRequest`, `ChatImage`, `ChatSender`, `ChatWorker`, `WorkerManager`, `WorkerStatus`, and `WorkerState` are defined before dependent tasks use them.
