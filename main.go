package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"pi-web/internal/auth"
	"pi-web/internal/render"
	"pi-web/internal/rpc"
	"pi-web/internal/sessions"
	"pi-web/internal/workers"
)

const defaultPort = "31483"
const tokenEnvVar = "PI_WEB_TOKEN"

// globalSessID is the sentinel SSE topic for events that are not tied to a
// specific session — e.g. a new session file appearing on disk. The index
// page subscribes to this so it can refresh when new sessions show up.
const globalSessID = "__all__"

// indexScriptPath is the URL path at which the index page's Vite module is
// served. It defaults to a stable path and is overwritten at startup if a
// hashed asset is found in the Vite manifest.
var indexScriptPath = "/static/assets/index.js"

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
	authMiddleware := auth.New(os.Getenv(tokenEnvVar))
	srv := newServer(sessionsDir, authMiddleware)

	mux := http.NewServeMux()
	mux.HandleFunc("/", authMiddleware.Wrap(srv.handleIndex))
	mux.HandleFunc("/session", authMiddleware.Wrap(srv.handleSession))
	mux.HandleFunc("/api/session", authMiddleware.Wrap(srv.handleApiSession))
	mux.HandleFunc("/api/chat", authMiddleware.Wrap(srv.handleChat))
	mux.HandleFunc("/api/set-model", authMiddleware.Wrap(srv.handleSetModel))
	mux.HandleFunc("/api/set-thinking-level", authMiddleware.Wrap(srv.handleSetThinkingLevel))
	mux.HandleFunc("/api/models", authMiddleware.Wrap(handleAvailableModels))
	mux.HandleFunc("/api/worker-status", authMiddleware.Wrap(srv.handleWorkerStatus))
	mux.HandleFunc("/share", authMiddleware.Wrap(srv.handleShare))
	mux.HandleFunc("/events", authMiddleware.Wrap(srv.handleEvents))
	mux.HandleFunc("/api/new-session", authMiddleware.Wrap(srv.handleNewSession))
	mux.HandleFunc("/api/recent-locations", authMiddleware.Wrap(srv.handleRecentLocations))
	mux.HandleFunc("/static/alpine.js", serveStaticJS(alpineJs))
	if scriptPath, js, err := loadIndexScript("web/dist"); err == nil {
		indexScriptPath = scriptPath
		mux.HandleFunc(scriptPath, serveIndexJS(js, scriptPath != "/static/assets/index.js"))
	} else {
		fmt.Fprintf(os.Stderr, "WARNING: failed to load Vite index script: %v (index page JS will be unavailable)\n", err)
	}

	addr := net.JoinHostPort(bindHost, *port)
	url := "http://" + addr
	fmt.Printf("Pi Sessions Viewer -> %s\n", url)
	if !usedTailscale && *hostOverride == "" {
		fmt.Println("Tailscale IP not detected; using localhost.")
	}
	fmt.Printf("Serving from: %s\n", sessionsDir)
	if authMiddleware.Enabled() {
		fmt.Println("Auth: enabled (set PI_WEB_TOKEN to require token)")
	} else {
		fmt.Printf("Auth: disabled — set %s to require a token for access.\n", tokenEnvVar)
	}

	pidfilePath, err := writePidfile(bindHost, *port, usedTailscale)
	if err != nil {
		fmt.Fprintf(os.Stderr, "WARNING: failed to write pidfile: %v\n", err)
	} else {
		defer os.Remove(pidfilePath)
	}

	if *open {
		go func() {
			time.Sleep(300 * time.Millisecond)
			openBrowser(url)
		}()
	}

	warmModelsCache()

	if err := http.ListenAndServe(addr, mux); err != nil {
		fmt.Fprintf(os.Stderr, "server error: %v\n", err)
		os.Exit(1)
	}
}

func openBrowser(url string) {
	var cmd string
	var args []string
	switch runtime.GOOS {
	case "darwin":
		cmd = "open"
		args = []string{url}
	case "windows":
		cmd = "cmd"
		args = []string{"/c", "start", url}
	default:
		cmd = "xdg-open"
		args = []string{url}
	}
	exec.Command(cmd, args...).Start()
}

func writePidfile(host, port string, usedTailscale bool) (string, error) {
	home := os.Getenv("HOME")
	if home == "" {
		return "", fmt.Errorf("HOME not set")
	}
	agentDir := filepath.Join(home, ".pi", "agent")
	if err := os.MkdirAll(agentDir, 0755); err != nil {
		return "", err
	}
	path := filepath.Join(agentDir, "pi-web-state.json")
	data, err := json.Marshal(map[string]any{
		"pid":       os.Getpid(),
		"port":      port,
		"host":      host,
		"tailscale": usedTailscale,
		"startedAt": time.Now().UTC().Format(time.RFC3339),
	})
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		return "", err
	}
	return path, nil
}

func loadIndexScript(distDir string) (scriptPath string, js string, err error) {
	data, err := os.ReadFile(filepath.Join(distDir, ".vite/manifest.json"))
	if err != nil {
		return "", "", fmt.Errorf("read manifest: %w", err)
	}
	var manifest render.Manifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return "", "", fmt.Errorf("parse manifest: %w", err)
	}
	entry, ok := manifest["src/index/index.js"]
	if !ok {
		return "", "", fmt.Errorf("manifest missing src/index/index.js entry")
	}
	if entry.File == "" {
		return "", "", fmt.Errorf("manifest entry file is empty")
	}
	if strings.HasPrefix(entry.File, "/") {
		return "", "", fmt.Errorf("manifest entry file is absolute: %s", entry.File)
	}
	if strings.Contains(entry.File, "..") {
		return "", "", fmt.Errorf("manifest entry file contains path traversal: %s", entry.File)
	}
	scriptPath, ok = manifest.ScriptPath("src/index/index.js")
	if !ok {
		return "", "", fmt.Errorf("manifest script path not found")
	}
	content, err := os.ReadFile(filepath.Join(distDir, entry.File))
	if err != nil {
		return "", "", fmt.Errorf("read index js: %w", err)
	}
	return scriptPath, string(content), nil
}

func serveIndexJS(js string, immutable bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
		if immutable {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		} else {
			w.Header().Set("Cache-Control", "no-cache")
		}
		_, _ = w.Write([]byte(js))
	}
}

// ── Server with live-reload SSE ────────────────────────────────────────────

type sseClient struct {
	ch     chan string
	sessID string
}

type statusClient struct {
	ch     chan struct{}
	sessID string
}

type server struct {
	sessionsDir     string
	clients         []*sseClient
	clientsMu       sync.RWMutex
	statusClients   []*statusClient
	statusClientsMu sync.RWMutex
	fileMod         map[string]time.Time
	fileModMu       sync.RWMutex
	chatSender      ChatSender
	cache           *sessions.Cache
	auth            *auth.Middleware
	shareRunner     shareCmdRunner
	now             func() time.Time
}

func newServer(sessionsDir string, auth *auth.Middleware) *server {
	s := &server{
		sessionsDir:   sessionsDir,
		clients:       make([]*sseClient, 0),
		statusClients: make([]*statusClient, 0),
		fileMod:       make(map[string]time.Time),
		chatSender:    workers.NewManager(rpc.NewPiWorker),
		cache:         sessions.NewCache(),
		auth:          auth,
		now:           time.Now,
	}
	go s.watchFiles()
	return s
}

func (s *server) loadSessions() ([]sessions.Session, error) {
	if s.cache != nil {
		return s.cache.LoadAll(s.sessionsDir)
	}
	return sessions.LoadAll(s.sessionsDir)
}

func (s *server) addClient(sessID string) *sseClient {
	c := &sseClient{ch: make(chan string, 4), sessID: sessID}
	s.clientsMu.Lock()
	s.clients = append(s.clients, c)
	s.clientsMu.Unlock()
	return c
}

func (s *server) removeClient(target *sseClient) {
	s.clientsMu.Lock()
	filtered := s.clients[:0]
	for _, c := range s.clients {
		if c != target {
			filtered = append(filtered, c)
		}
	}
	s.clients = filtered
	s.clientsMu.Unlock()
	close(target.ch)
}

func (s *server) broadcast(sessID, msg string) {
	s.clientsMu.RLock()
	defer s.clientsMu.RUnlock()
	for _, c := range s.clients {
		if c.sessID == sessID {
			select {
			case c.ch <- msg:
			default:
			}
		}
	}
}

func (s *server) addStatusClient(sessID string) *statusClient {
	c := &statusClient{ch: make(chan struct{}, 1), sessID: sessID}
	s.statusClientsMu.Lock()
	s.statusClients = append(s.statusClients, c)
	s.statusClientsMu.Unlock()
	return c
}

func (s *server) removeStatusClient(target *statusClient) {
	s.statusClientsMu.Lock()
	filtered := s.statusClients[:0]
	for _, c := range s.statusClients {
		if c != target {
			filtered = append(filtered, c)
		}
	}
	s.statusClients = filtered
	s.statusClientsMu.Unlock()
	close(target.ch)
}

func (s *server) broadcastStatusChange(sessID string) {
	s.statusClientsMu.RLock()
	defer s.statusClientsMu.RUnlock()
	for _, c := range s.statusClients {
		if c.sessID == sessID {
			select {
			case c.ch <- struct{}{}:
			default:
			}
		}
	}
}

// watchFiles is implemented in file_watcher.go.

// ── HTTP Handlers ──────────────────────────────────────────────────────────

func (s *server) handleIndex(w http.ResponseWriter, r *http.Request) {
	sessions, err := s.loadSessions()
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := indexTmpl.Execute(w, sessions); err != nil {
		fmt.Fprintf(os.Stderr, "template error: %v\n", err)
	}
}

func (s *server) handleSession(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	if id == "" {
		http.Error(w, "missing id", 400)
		return
	}

	sessions, err := s.loadSessions()
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}

	for _, sess := range sessions {
		if sess.ID == id {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.Write([]byte(generateExportHtml(sess, true)))
			return
		}
	}
	http.Error(w, "session not found", 404)
}

func (s *server) handleApiSession(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	if id == "" {
		writeJSONError(w, http.StatusBadRequest, "missing id")
		return
	}

	sessions, err := s.loadSessions()
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}

	for _, sess := range sessions {
		if sess.ID == id {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]any{
				"header":  sess.Header,
				"entries": sess.Entries,
			})
			return
		}
	}
	writeJSONError(w, http.StatusNotFound, "not found")
}

func (s *server) handleEvents(w http.ResponseWriter, r *http.Request) {
	singleID := r.URL.Query().Get("id")
	multiIDs := r.URL.Query().Get("ids")

	if singleID == "" && multiIDs == "" {
		http.Error(w, "missing id or ids", 400)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	// Single-session mode (existing)
	if singleID != "" {
		client := s.addClient(singleID)
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
	}

	// Multi-session mode (batch status)
	ids := strings.Split(multiIDs, ",")
	if len(ids) == 0 {
		return
	}

	clients := make([]*statusClient, len(ids))
	for i, id := range ids {
		clients[i] = s.addStatusClient(id)
	}
	defer func() {
		for _, c := range clients {
			s.removeStatusClient(c)
		}
	}()

	lastSent := make(map[string]string, len(ids))
	s.sendStatusMapIfChanged(w, flusher, r.Context(), ids, lastSent)

	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
			fmt.Fprintf(w, ":hb\n\n")
			flusher.Flush()
		default:
			changed := false
			for _, c := range clients {
				select {
				case <-c.ch:
					changed = true
				default:
				}
			}
			if changed {
				s.sendStatusMapIfChanged(w, flusher, r.Context(), ids, lastSent)
			} else {
				time.Sleep(50 * time.Millisecond)
			}
		}
	}
}

func (s *server) sendStatusMapIfChanged(w http.ResponseWriter, flusher http.Flusher, ctx context.Context, ids []string, lastSent map[string]string) {
	result := make(map[string]*workers.WorkerStatus, len(ids))
	changed := false
	for _, id := range ids {
		status := s.computeWorkerStatus(ctx, id)
		result[id] = status
		stateStr := string(status.State)
		if lastSent[id] != stateStr {
			lastSent[id] = stateStr
			changed = true
		}
	}
	if !changed {
		return
	}
	data, _ := json.Marshal(result)
	fmt.Fprintf(w, "data: %s\n\n", string(data))
	flusher.Flush()
}

func (s *server) handleNewSession(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var body struct {
		Path string `json:"path"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid json body")
		return
	}
	if body.Path == "" {
		writeJSONError(w, http.StatusBadRequest, "path is required")
		return
	}

	id, err := sessions.CreateSessionFile(s.sessionsDir, body.Path)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"ok": true, "id": id})
}

func (s *server) handleRecentLocations(w http.ResponseWriter, r *http.Request) {
	locations, err := sessions.ListRecentLocations(s.sessionsDir)
	if err != nil {
		locations = []string{}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"locations": locations})
}

func handleAvailableModels(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	data, err := defaultModelsCache.get(ctx)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			writeJSONError(w, http.StatusGatewayTimeout, "timed out waiting for model list")
			return
		}
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}

	var payload struct {
		Models []map[string]any `json:"models"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		writeJSONError(w, http.StatusInternalServerError, "invalid model list payload: "+err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"models": payload.Models})
}
