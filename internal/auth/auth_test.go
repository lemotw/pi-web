package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func okHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("ok"))
}

func TestAuthDisabledPassesThrough(t *testing.T) {
	a := New("")
	if a.Enabled() {
		t.Fatal("expected Enabled()=false when token empty")
	}
	rec := httptest.NewRecorder()
	a.Wrap(okHandler)(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestAuthRejectsMissingToken(t *testing.T) {
	a := New("secret")
	rec := httptest.NewRecorder()
	a.Wrap(okHandler)(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}

func TestAuthRejectsWrongToken(t *testing.T) {
	a := New("secret")
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/?token=nope", nil)
	a.Wrap(okHandler)(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}

func TestAuthAcceptsQueryAndSetsCookie(t *testing.T) {
	a := New("secret")
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/?token=secret", nil)
	a.Wrap(okHandler)(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	cookies := rec.Result().Cookies()
	var found *http.Cookie
	for _, c := range cookies {
		if c.Name == TokenCookieName {
			found = c
			break
		}
	}
	if found == nil {
		t.Fatalf("expected %s cookie to be set", TokenCookieName)
	}
	if found.Value != "secret" {
		t.Fatalf("cookie value = %q", found.Value)
	}
	if !found.HttpOnly {
		t.Fatal("expected HttpOnly cookie")
	}
}

func TestAuthAcceptsCookie(t *testing.T) {
	a := New("secret")
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: TokenCookieName, Value: "secret"})
	a.Wrap(okHandler)(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	// Cookie was already present; we should not re-set it.
	for _, c := range rec.Result().Cookies() {
		if c.Name == TokenCookieName {
			t.Fatal("did not expect cookie to be re-set when request already had it")
		}
	}
}

func TestAuthAcceptsBearerHeader(t *testing.T) {
	a := New("secret")
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer secret")
	a.Wrap(okHandler)(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestAuthAcceptsXPiTokenHeader(t *testing.T) {
	a := New("secret")
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Pi-Token", "secret")
	a.Wrap(okHandler)(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestAuthEmptyTokenSubmittedWhenAuthEnabled(t *testing.T) {
	// Empty submitted value must not match an empty stored value
	// (which can't happen since Enabled() requires non-empty, but check
	// constant-time compare doesn't accept "").
	a := New("secret")
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/?token=", nil)
	a.Wrap(okHandler)(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}
