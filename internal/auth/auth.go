package auth

import (
	"crypto/subtle"
	"net/http"
	"strings"
)

const TokenCookieName = "pi_token"

type Middleware struct {
	token string
}

func New(token string) *Middleware {
	return &Middleware{token: strings.TrimSpace(token)}
}

func (a *Middleware) Enabled() bool {
	return a.token != ""
}

// Wrap returns a handler that enforces the token check when auth is enabled.
// When the token is supplied via the `token` query parameter, a cookie is set
// so subsequent requests from the same browser succeed without the parameter.
func (a *Middleware) Wrap(h http.HandlerFunc) http.HandlerFunc {
	if !a.Enabled() {
		return h
	}
	return func(w http.ResponseWriter, r *http.Request) {
		got, fromQuery := ExtractToken(r)
		if subtle.ConstantTimeCompare([]byte(got), []byte(a.token)) != 1 {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		if fromQuery {
			http.SetCookie(w, &http.Cookie{
				Name:     TokenCookieName,
				Value:    got,
				Path:     "/",
				HttpOnly: true,
				SameSite: http.SameSiteLaxMode,
			})
		}
		h(w, r)
	}
}

// ExtractToken returns the candidate token and whether it came from the query
// string (in which case a cookie should be set).
func ExtractToken(r *http.Request) (string, bool) {
	if t := r.URL.Query().Get("token"); t != "" {
		return t, true
	}
	if h := r.Header.Get("Authorization"); strings.HasPrefix(h, "Bearer ") {
		return strings.TrimPrefix(h, "Bearer "), false
	}
	if h := r.Header.Get("X-Pi-Token"); h != "" {
		return h, false
	}
	if c, err := r.Cookie(TokenCookieName); err == nil {
		return c.Value, false
	}
	return "", false
}
