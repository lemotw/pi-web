package main

import (
	"bufio"
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
)

const defaultPort = "31483"
const tokenEnvVar = "PI_WEB_TOKEN"

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
	auth := newAuth(os.Getenv(tokenEnvVar))
	srv := newServer(sessionsDir, auth)

	mux := http.NewServeMux()
	mux.HandleFunc("/", auth.wrap(srv.handleIndex))
	mux.HandleFunc("/session", auth.wrap(srv.handleSession))
	mux.HandleFunc("/api/session", auth.wrap(srv.handleApiSession))
	mux.HandleFunc("/api/chat", auth.wrap(srv.handleChat))
	mux.HandleFunc("/api/set-model", auth.wrap(srv.handleSetModel))
	mux.HandleFunc("/api/set-thinking-level", auth.wrap(srv.handleSetThinkingLevel))
	mux.HandleFunc("/api/models", auth.wrap(handleAvailableModels))
	mux.HandleFunc("/api/worker-status", auth.wrap(srv.handleWorkerStatus))
	mux.HandleFunc("/share", auth.wrap(srv.handleShare))
	mux.HandleFunc("/events", auth.wrap(srv.handleEvents))
	mux.HandleFunc("/api/new-session", auth.wrap(srv.handleNewSession))
	mux.HandleFunc("/api/recent-locations", auth.wrap(srv.handleRecentLocations))

	addr := net.JoinHostPort(bindHost, *port)
	url := "http://" + addr
	fmt.Printf("Pi Sessions Viewer -> %s\n", url)
	if !usedTailscale && *hostOverride == "" {
		fmt.Println("Tailscale IP not detected; using localhost.")
	}
	fmt.Printf("Serving from: %s\n", sessionsDir)
	if auth.enabled() {
		fmt.Println("Auth: enabled (set PI_WEB_TOKEN to require token)")
	} else {
		fmt.Printf("Auth: disabled — set %s to require a token for access.\n", tokenEnvVar)
	}

	if *open {
		go func() {
			time.Sleep(300 * time.Millisecond)
			openBrowser(url)
		}()
	}

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

// ── Server with live-reload SSE ────────────────────────────────────────────

type sseClient struct {
	ch     chan string
	sessID string
}

type server struct {
	sessionsDir string
	clients     []*sseClient
	clientsMu   sync.RWMutex
	fileMod     map[string]time.Time
	fileModMu   sync.RWMutex
	chatSender  ChatSender
	cache       *sessionCache
	auth        *authMiddleware
	shareRunner shareCmdRunner
}

func newServer(sessionsDir string, auth *authMiddleware) *server {
	s := &server{
		sessionsDir: sessionsDir,
		clients:     make([]*sseClient, 0),
		fileMod:     make(map[string]time.Time),
		chatSender:  NewWorkerManager(newPiRPCWorker),
		cache:       newSessionCache(),
		auth:        auth,
	}
	go s.watchFiles()
	return s
}

func (s *server) loadSessions() ([]Session, error) {
	if s.cache != nil {
		return s.cache.loadAll(s.sessionsDir)
	}
	return loadAllSessions(s.sessionsDir)
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

func (s *server) watchFiles() {
	ticker := time.NewTicker(1500 * time.Millisecond)
	defer ticker.Stop()

	for range ticker.C {
		entries, err := os.ReadDir(s.sessionsDir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			subDir := filepath.Join(s.sessionsDir, e.Name())
			subs, err := os.ReadDir(subDir)
			if err != nil {
				continue
			}
			for _, f := range subs {
				if f.IsDir() || !strings.HasSuffix(f.Name(), ".jsonl") {
					continue
				}
				sessID := f.Name()
				path := filepath.Join(subDir, f.Name())
				info, err := os.Stat(path)
				if err != nil {
					continue
				}

				s.fileModMu.Lock()
				lastMod, known := s.fileMod[sessID]
				s.fileMod[sessID] = info.ModTime()
				s.fileModMu.Unlock()

				if known && info.ModTime().After(lastMod) {
					s.broadcast(sessID, "reload")
				}
			}
		}
	}
}

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
	sessID := r.URL.Query().Get("id")
	if sessID == "" {
		http.Error(w, "missing id", 400)
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

	id, err := createSessionFile(s.sessionsDir, body.Path)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"ok": true, "id": id})
}

func (s *server) handleRecentLocations(w http.ResponseWriter, r *http.Request) {
	locations, err := listRecentLocations(s.sessionsDir)
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
	if _, err := exec.LookPath("pi"); err != nil {
		writeJSONError(w, http.StatusServiceUnavailable, "pi executable not found")
		return
	}

	cmd := exec.Command("pi", "--mode", "rpc")
	stdin, err := cmd.StdinPipe()
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	var stderrBuf strings.Builder
	cmd.Stderr = &stderrBuf

	if err := cmd.Start(); err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer func() {
		_ = stdin.Close()
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	}()

	type response struct {
		Type    string `json:"type"`
		ID      string `json:"id"`
		Command string `json:"command"`
		Success bool   `json:"success"`
		Data    struct {
			Models []map[string]any `json:"models"`
		} `json:"data"`
		Error string `json:"error"`
	}

	type scanResult struct {
		models []map[string]any
		err    error
	}

	reqID := fmt.Sprintf("models-%d", time.Now().UnixNano())
	resultCh := make(chan scanResult, 1)
	go func() {
		defer close(resultCh)
		scanner := bufio.NewScanner(stdout)
		scanner.Buffer(make([]byte, 0, 64*1024), 10*1024*1024)
		for scanner.Scan() {
			line := strings.TrimSuffix(scanner.Text(), "\r")
			if strings.TrimSpace(line) == "" {
				continue
			}
			var res response
			if err := json.Unmarshal([]byte(line), &res); err != nil {
				continue
			}
			if res.Type == "response" && res.ID == reqID && res.Command == "get_available_models" {
				if !res.Success {
					resultCh <- scanResult{err: errors.New(res.Error)}
					return
				}
				resultCh <- scanResult{models: res.Data.Models}
				return
			}
		}
		if err := scanner.Err(); err != nil {
			resultCh <- scanResult{err: err}
		} else {
			resultCh <- scanResult{err: fmt.Errorf("pi closed stdout without response (stderr: %q)", stderrBuf.String())}
		}
	}()

	if _, err := fmt.Fprintf(stdin, `{"id":"%s","type":"get_available_models"}`+"\n", reqID); err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}

	select {
	case res := <-resultCh:
		if res.err != nil {
			writeJSONError(w, http.StatusInternalServerError, res.err.Error())
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"models": res.models})
	case <-time.After(10 * time.Second):
		writeJSONError(w, http.StatusGatewayTimeout, fmt.Sprintf("timed out waiting for model list (stderr: %q)", stderrBuf.String()))
	}
}
