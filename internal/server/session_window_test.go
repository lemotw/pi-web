package server

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"pi-web/internal/sessions"
)

func TestSessionWindow(t *testing.T) {
	root := t.TempDir()
	writeSessionWithNMessages(t, root, "proj", "window.jsonl", 250)
	s := &Server{sessionsDir: root, cache: sessions.NewCache()}
	for _, tt := range []struct {
		name, query, cookie string
		from, count, size   int
		end                 *int
	}{
		{"default", "", "", 151, 100, 100, nil},
		{"fifty", "&limit=50", "", 201, 50, 50, nil},
		{"two hundred", "&limit=200", "", 51, 200, 200, nil},
		{"cookie", "", "50", 201, 50, 50, nil},
		{"query overrides cookie", "&limit=200", "50", 51, 200, 200, nil},
		{"bad limit bounded", "&limit=999999999999999999999", "", 151, 100, 100, nil},
		{"negative limit bounded", "&limit=-1", "", 151, 100, 100, nil},
		{"bad cookie bounded", "", "bad", 151, 100, 100, nil},
		{"earlier", "&limit=50&before=201", "", 151, 50, 50, intPointer(201)},
		{"first partial page", "&limit=50&before=10", "", 0, 10, 50, intPointer(10)},
		{"zero", "&before=0", "", 0, 0, 100, intPointer(0)},
		{"end clamped", "&before=99999", "", 151, 100, 100, intPointer(251)},
		{"negative before", "&before=-1", "", 151, 100, 100, nil},
		{"target", "&limit=50&targetId=id000080", "", 32, 50, 50, intPointer(82)},
		{"leaf", "&limit=50&leafId=id000080", "", 32, 50, 50, intPointer(82)},
		{"missing target", "&targetId=missing", "", 151, 100, 100, nil},
	} {
		t.Run(tt.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, "/api/session?id=window.jsonl&paginate=1"+tt.query, nil)
			if tt.cookie != "" {
				r.AddCookie(&http.Cookie{Name: sessionWindowCookie, Value: tt.cookie})
			}
			w := httptest.NewRecorder()
			s.handleApiSession(w, r)
			if w.Code != http.StatusOK {
				t.Fatalf("status %d: %s", w.Code, w.Body.String())
			}
			var data struct {
				Entries []map[string]any `json:"entries"`
				From    int              `json:"from"`
				Total   int              `json:"total"`
				Size    int              `json:"windowSize"`
				End     *int             `json:"windowEnd"`
			}
			if err := json.Unmarshal(w.Body.Bytes(), &data); err != nil {
				t.Fatal(err)
			}
			if data.From != tt.from || len(data.Entries) != tt.count || data.Total != 251 || data.Size != tt.size {
				t.Fatalf("got from=%d count=%d total=%d size=%d", data.From, len(data.Entries), data.Total, data.Size)
			}
			if (data.End == nil) != (tt.end == nil) || (data.End != nil && *data.End != *tt.end) {
				t.Fatalf("unexpected windowEnd: %v", data.End)
			}

			// A hard navigation must embed exactly the same bounded records.
			encoded := s.sessionBootstrap("window.jsonl", r)
			raw, err := base64.StdEncoding.DecodeString(encoded)
			if err != nil {
				t.Fatal(err)
			}
			var boot struct {
				Data json.RawMessage `json:"data"`
			}
			if err := json.Unmarshal(raw, &boot); err != nil {
				t.Fatal(err)
			}
			var apiMap, bootMap map[string]any
			json.Unmarshal(w.Body.Bytes(), &apiMap)
			json.Unmarshal(boot.Data, &bootMap)
			if len(bootMap["entries"].([]any)) != tt.count || bootMap["from"] != apiMap["from"] {
				t.Fatal("bootstrap differs from API")
			}
		})
	}
}

func intPointer(n int) *int { return &n }

func TestSessionWindowEmptyAndGrowing(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/api/session?paginate=1&limit=50&before=20", nil)
	session := sessions.Session{Entries: make([]map[string]any, 80)}
	data := sessionWindowResponse(session, r)
	if data["from"] != 0 || len(data["entries"].([]map[string]any)) != 20 {
		t.Fatal("wrong historical page")
	}
	session.Entries = append(session.Entries, make([]map[string]any, 50)...)
	data = sessionWindowResponse(session, r)
	if data["from"] != 0 || len(data["entries"].([]map[string]any)) != 20 || data["total"] != 130 {
		t.Fatal("historical page moved on append")
	}
	data = sessionWindowResponse(sessions.Session{}, r)
	if data["from"] != 0 || len(data["entries"].([]map[string]any)) != 0 {
		t.Fatal("empty session not bounded")
	}
}
