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
