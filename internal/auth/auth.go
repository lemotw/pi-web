package auth

import (
	"crypto/subtle"
	"net/http"
	"strings"
)

const TokenCookieName = "pi_token"

// tokenPromptHTML is served when auth is enabled and a browser (Accept:
// text/html) request arrives without a valid token. It renders a clean,
// Linear-inspired auth form that POSTs the token so it never appears in
// the URL bar or browser history.
const tokenPromptHTML = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1, maximum-scale=1">
<title>pi-web — Authentication Required</title>
<script>
  (function() {
    var theme = 'dark';
    try { theme = localStorage.getItem('pi-web-theme') || 'dark'; } catch (e) {}
    document.documentElement.dataset.theme = theme === 'light' ? 'light' : 'dark';
  })();
</script>
<style>
  :root, [data-theme="dark"] {
    --body-bg: #111116;
    --text: #e6e7eb;
    --text-soft: #b7bbc4;
    --muted: #858a96;
    --dim: #292a33;
    --surface-2: #191920;
    --accent: #9cc7c0;
    --danger: #ef767a;
  }
  [data-theme="light"] {
    --body-bg: #f6f5f2;
    --text: #1f2328;
    --text-soft: #3f4650;
    --muted: #747b85;
    --dim: #d8d5cc;
    --surface-2: #f1f0ec;
    --accent: #496f69;
    --danger: #b23b42;
  }
  * { margin: 0; padding: 0; box-sizing: border-box; }
  html, body { height: 100%; }
  body {
    font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, Helvetica, Arial, sans-serif;
    font-size: 14px;
    line-height: 1.5;
    color: var(--text-soft);
    background: var(--body-bg);
    display: flex;
    align-items: center;
    justify-content: center;
    padding: 24px;
    -webkit-font-smoothing: antialiased;
    -moz-osx-font-smoothing: grayscale;
  }
  .container {
    width: 100%;
    max-width: 360px;
  }
  h1 {
    color: var(--text);
    font-size: 20px;
    font-weight: 650;
    letter-spacing: -0.3px;
    margin-bottom: 6px;
  }
  .subtitle {
    color: var(--muted);
    font-size: 13px;
    margin-bottom: 8px;
  }
  .hint {
    color: var(--muted);
    font-size: 12px;
    margin-bottom: 24px;
    line-height: 1.6;
  }
  .hint code {
    background: var(--surface-2);
    color: var(--text-soft);
    padding: 2px 6px;
    border-radius: 4px;
    font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, "Liberation Mono", monospace;
    font-size: 11px;
    white-space: nowrap;
  }
  .field {
    margin-bottom: 6px;
  }
  label {
    display: block;
    color: var(--text-soft);
    font-size: 12px;
    font-weight: 550;
    margin-bottom: 6px;
  }
  input {
    width: 100%;
    height: 42px;
    background: var(--surface-2);
    border: 1px solid var(--dim);
    border-radius: 8px;
    color: var(--text);
    padding: 0 12px;
    font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, "Liberation Mono", monospace;
    font-size: 13px;
    outline: none;
    transition: border-color 0.15s;
  }
  input:focus { border-color: var(--accent); }
  input::placeholder { color: var(--muted); }
  .error {
    visibility: hidden;
    color: var(--danger);
    font-size: 12px;
    line-height: 16px;
    min-height: 16px;
    margin-bottom: 8px;
  }
  button {
    width: 100%;
    height: 42px;
    background: var(--text);
    color: var(--body-bg);
    border: 0;
    border-radius: 8px;
    font-family: inherit;
    font-size: 14px;
    font-weight: 600;
    cursor: pointer;
    transition: opacity 0.15s;
  }
  button:hover { opacity: 0.88; }
</style>
</head>
<body>
<div class="container">
  <h1>Authentication Required</h1>
  <p class="subtitle">This instance requires an access token.</p>
  <p class="hint">Run <code>/pi-web token</code> in <code>pi</code> to retrieve it.</p>
  <form method="post">
    <div class="field">
      <label for="token">Access Token</label>
      <input type="password" id="token" name="token" placeholder="Enter access token" required autofocus autocomplete="off">
    </div>
    <div class="error" id="error">Invalid access token.</div>
    <button type="submit">Continue</button>
  </form>
</div>
<script>
  if (location.search.indexOf('error=1') !== -1) {
    document.getElementById('error').style.visibility = 'visible';
  }
</script>
</body>
</html>`

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
//
// Token sources (checked in order): for browser POSTs, form body first;
// otherwise query parameter, Authorization header, X-Pi-Token header, and
// cookie. When the token arrives via query or POST, a cookie is set and the
// browser is redirected to the same URL without the token, so the secret never
// appears in the address bar or browser history.
//
// When auth fails and the request appears to come from a browser (Accept
// header includes text/html), the middleware serves an HTML token prompt
// instead of a bare 401. API clients (no text/html in Accept) still receive
// a plain 401.
func (a *Middleware) Wrap(h http.HandlerFunc) http.HandlerFunc {
	if !a.Enabled() {
		return h
	}
	return func(w http.ResponseWriter, r *http.Request) {
		got := ""
		fromQuery := false
		fromPost := false

		// Browser login form submissions should prefer the submitted token over
		// any stale token that may still be present in the URL query string.
		if r.Method == http.MethodPost && strings.Contains(r.Header.Get("Accept"), "text/html") {
			// ParseForm is idempotent; safe to call even if already parsed.
			r.ParseForm()
			if t := r.PostFormValue("token"); t != "" {
				got = t
				fromPost = true
			}
		}

		if got == "" {
			got, fromQuery = ExtractToken(r)
		}

		if subtle.ConstantTimeCompare([]byte(got), []byte(a.token)) != 1 {
			if strings.Contains(r.Header.Get("Accept"), "text/html") {
				// Invalid login attempt — redirect with error flag so
				// the prompt shows "Invalid token".
				if fromPost {
					target := cleanURL(r)
					if strings.Contains(target, "?") {
						target += "&error=1"
					} else {
						target += "?error=1"
					}
					http.Redirect(w, r, target, http.StatusFound)
					return
				}
				w.Header().Set("Content-Type", "text/html; charset=utf-8")
				w.WriteHeader(http.StatusUnauthorized)
				w.Write([]byte(tokenPromptHTML))
				return
			}
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		// Token is valid. Set a cookie if it came from query or POST so
		// subsequent requests authenticate automatically.
		shouldSetCookie := fromQuery || fromPost
		if shouldSetCookie {
			http.SetCookie(w, &http.Cookie{
				Name:     TokenCookieName,
				Value:    got,
				Path:     "/",
				HttpOnly: true,
				SameSite: http.SameSiteLaxMode,
			})
		}

		// Redirect to a clean URL (no token in query or error flag) when
		// the token arrived via query or POST. This keeps the secret out
		// of the address bar and browser history.
		if fromQuery || fromPost {
			http.Redirect(w, r, cleanURL(r), http.StatusFound)
			return
		}

		h(w, r)
	}
}

// cleanURL returns r.URL.Path with query string intact except for "token" and
// "error" parameters, which are stripped.
func cleanURL(r *http.Request) string {
	q := r.URL.Query()
	q.Del("token")
	q.Del("error")
	if len(q) == 0 {
		return r.URL.Path
	}
	return r.URL.Path + "?" + q.Encode()
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
