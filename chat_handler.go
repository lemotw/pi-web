package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"pi-web/internal/chat"
	"pi-web/internal/sessions"
	"pi-web/internal/workers"
)

type ChatSender interface {
	Send(ctx context.Context, sessionID, sessionPath string, chat chat.Request) error
	SetModel(ctx context.Context, sessionID, sessionPath, provider, modelID string) error
	SetThinkingLevel(ctx context.Context, sessionID, sessionPath, level string) error
	GetState(ctx context.Context, sessionID string) (workers.WorkerStatus, error)
	Status(sessionID string) workers.WorkerStatus
}

func (s *server) handleChat(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	resolved, err := sessions.ResolveByID(s.sessionsDir, r.URL.Query().Get("id"))
	if err != nil {
		if errors.Is(err, sessions.ErrInvalidSessionID) {
			writeJSONError(w, http.StatusBadRequest, "invalid session id")
			return
		}
		if errors.Is(err, sessions.ErrSessionNotFound) {
			writeJSONError(w, http.StatusNotFound, "session not found")
			return
		}
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if !resolved.Session.ChatAvailable {
		writeJSONError(w, http.StatusConflict, resolved.Session.ChatDisabledReason)
		return
	}
	chatReq, err := chat.ParseRequest(r, chat.DefaultMaxImageBytes, chat.DefaultMaxRequestBytes)
	if err != nil {
		switch {
		case errors.Is(err, chat.ErrEmptyRequest):
			writeJSONError(w, http.StatusBadRequest, err.Error())
		case errors.Is(err, chat.ErrImageTooLarge):
			writeJSONError(w, http.StatusRequestEntityTooLarge, err.Error())
		case errors.Is(err, chat.ErrUnsupportedImageType):
			writeJSONError(w, http.StatusUnsupportedMediaType, err.Error())
		case errors.As(err, new(*http.MaxBytesError)):
			writeJSONError(w, http.StatusRequestEntityTooLarge, err.Error())
		default:
			writeJSONError(w, http.StatusBadRequest, err.Error())
		}
		return
	}
	if err := s.chatSender.Send(r.Context(), resolved.Session.ID, resolved.Path, chatReq); err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"ok": true, "status": "accepted"})
}

const recentSessionActivityWindow = 3 * time.Second
const sessionStatusTTL = 10 * time.Second

type sessionStatusFile struct {
	State     string `json:"state"`
	UpdatedAt string `json:"updatedAt"`
}

func (s *server) readSessionStatus(sessionID string) *workers.WorkerStatus {
	if sessionID == "" {
		return nil
	}
	dir := filepath.Join(s.sessionsDir, "..", "session-status")
	path := filepath.Join(dir, sessionID)
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var status sessionStatusFile
	if err := json.Unmarshal(data, &status); err != nil {
		return nil
	}
	if status.State != "running" {
		return nil
	}
	updatedAt, err := time.Parse(time.RFC3339, status.UpdatedAt)
	if err != nil {
		return nil
	}
	if time.Since(updatedAt) > sessionStatusTTL {
		return nil
	}
	return &workers.WorkerStatus{State: workers.WorkerStateRunning}
}

func (s *server) computeWorkerStatus(ctx context.Context, sessionID string) *workers.WorkerStatus {
	if status := s.readSessionStatus(sessionID); status != nil {
		return status
	}
	status := s.chatSender.Status(sessionID)
	if status.State != workers.WorkerStateRunning {
		if state, err := s.chatSender.GetState(ctx, sessionID); err == nil {
			status.ThinkingLevel = state.ThinkingLevel
		}
	}
	if status.State == workers.WorkerStateIdle && s.hasRecentSessionActivity(sessionID) {
		status.State = workers.WorkerStateRunning
	}
	return &status
}

func (s *server) handleWorkerStatus(w http.ResponseWriter, r *http.Request) {
	sessionID := r.URL.Query().Get("id")
	status := s.computeWorkerStatus(r.Context(), sessionID)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(status)
}

func (s *server) hasRecentSessionActivity(sessionID string) bool {
	if sessionID == "" {
		return false
	}
	var now time.Time
	if s.now != nil {
		now = s.now()
	} else {
		now = time.Now()
	}
	s.fileModMu.RLock()
	mod, ok := s.fileMod[sessionID]
	s.fileModMu.RUnlock()
	if !ok {
		return false
	}
	return !mod.IsZero() && now.Sub(mod) <= recentSessionActivityWindow
}

func (s *server) handleSetModel(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	resolved, err := sessions.ResolveByID(s.sessionsDir, r.URL.Query().Get("id"))
	if err != nil {
		if errors.Is(err, sessions.ErrInvalidSessionID) {
			writeJSONError(w, http.StatusBadRequest, "invalid session id")
			return
		}
		if errors.Is(err, sessions.ErrSessionNotFound) {
			writeJSONError(w, http.StatusNotFound, "session not found")
			return
		}
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	var body struct {
		Provider string `json:"provider"`
		ModelID  string `json:"modelId"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid json body")
		return
	}
	if body.Provider == "" || body.ModelID == "" {
		writeJSONError(w, http.StatusBadRequest, "provider and modelId required")
		return
	}
	if err := s.chatSender.SetModel(r.Context(), resolved.Session.ID, resolved.Path, body.Provider, body.ModelID); err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"ok": true})
}

func (s *server) handleSetThinkingLevel(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	resolved, err := sessions.ResolveByID(s.sessionsDir, r.URL.Query().Get("id"))
	if err != nil {
		if errors.Is(err, sessions.ErrInvalidSessionID) {
			writeJSONError(w, http.StatusBadRequest, "invalid session id")
			return
		}
		if errors.Is(err, sessions.ErrSessionNotFound) {
			writeJSONError(w, http.StatusNotFound, "session not found")
			return
		}
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	var body struct {
		Level string `json:"level"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid json body")
		return
	}
	if body.Level == "" {
		writeJSONError(w, http.StatusBadRequest, "level required")
		return
	}
	if err := s.chatSender.SetThinkingLevel(r.Context(), resolved.Session.ID, resolved.Path, body.Level); err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	status := s.chatSender.Status(resolved.Session.ID)
	if state, err := s.chatSender.GetState(r.Context(), resolved.Session.ID); err == nil {
		status.ThinkingLevel = state.ThinkingLevel
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"ok": true, "thinkingLevel": status.ThinkingLevel})
}

func writeJSONError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]any{"error": message})
}
