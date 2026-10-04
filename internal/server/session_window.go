package server

import (
	"net/http"
	"strconv"

	"pi-web/internal/sessions"
)

const sessionWindowCookie = "pi_session_window"

// A window counts all JSONL records, including tools and metadata. Keeping the
// same bound for bootstrap and live reload avoids silently loading full history.
func sessionWindowResponse(session sessions.Session, r *http.Request) map[string]any {
	q := r.URL.Query()
	size := 100
	rawSize := q.Get("limit")
	if rawSize == "" {
		if cookie, err := r.Cookie(sessionWindowCookie); err == nil {
			rawSize = cookie.Value
		}
	}
	if n, err := strconv.Atoi(rawSize); err == nil && (n == 50 || n == 100 || n == 200) {
		size = n
	}

	total := len(session.Entries)
	end := total
	var windowEnd *int
	if n, err := strconv.Atoi(q.Get("before")); err == nil && n >= 0 {
		end = min(n, total)
		windowEnd = &end
	}
	// Deep links load a bounded window ending at the requested entry instead
	// of downloading every ancestor. Missing targets fall back to the tail.
	target := q.Get("targetId")
	if target == "" {
		target = q.Get("leafId")
	}
	if target != "" {
		for i, entry := range session.Entries {
			if entry["id"] == target {
				end = i + 1
				windowEnd = &end
				break
			}
		}
	}
	from := max(0, end-size)
	data := sessionResponseMap(session, session.Entries[from:end], total, from)
	data["windowSize"] = size
	data["windowEnd"] = windowEnd
	return data
}
