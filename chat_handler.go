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
	resolved, err := resolveSessionByID(s.sessionsDir, r.URL.Query().Get("id"))
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
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(s.chatSender.Status(r.URL.Query().Get("id")))
}

func writeJSONError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]any{"error": message})
}
