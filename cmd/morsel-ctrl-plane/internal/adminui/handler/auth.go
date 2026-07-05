package handler

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/ifeanyiecheruo/morsel/cmd/morsel-ctrl-plane/internal/adminui/pages"
	"github.com/ifeanyiecheruo/morsel/cmd/morsel-ctrl-plane/internal/tokens"
	"github.com/ifeanyiecheruo/morsel/internal/ctxlog"
)

const sessionCookie = "morsel_admin"

// encodeSession base64url-encodes session JSON and appends an HMAC-SHA256 tag:
// "<b64(json)>.<b64(hmac)>". The HMAC tag prevents cookie forgery while keeping
// the session data readable for debugging (tokens are already JWTs).
func (h *Handler) encodeSession(s *session) (string, error) {
	payload, err := json.Marshal(s)
	if err != nil {
		return "", err
	}
	b64 := base64.RawURLEncoding.EncodeToString(payload)
	mac := hmac.New(sha256.New, h.sessionKey)
	_, _ = io.WriteString(mac, b64)
	sig := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	return b64 + "." + sig, nil
}

// decodeSession parses and verifies a session cookie value produced by encodeSession.
func (h *Handler) decodeSession(cookieVal string) (*session, error) {
	dot := strings.LastIndex(cookieVal, ".")
	if dot < 0 {
		return nil, errors.New("malformed session cookie")
	}
	b64, sig := cookieVal[:dot], cookieVal[dot+1:]

	mac := hmac.New(sha256.New, h.sessionKey)
	_, _ = io.WriteString(mac, b64)
	expected := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	if !hmac.Equal([]byte(sig), []byte(expected)) {
		return nil, errors.New("session cookie signature mismatch")
	}

	payload, err := base64.RawURLEncoding.DecodeString(b64)
	if err != nil {
		return nil, err
	}
	var s session
	if err := json.Unmarshal(payload, &s); err != nil {
		return nil, err
	}
	return &s, nil
}

// setSessionCookie writes the session to an HttpOnly cookie on the response.
func (h *Handler) setSessionCookie(w http.ResponseWriter, s *session) error {
	val, err := h.encodeSession(s)
	if err != nil {
		return err
	}
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    val,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int((8 * time.Hour).Seconds()),
	})
	return nil
}

// clearSessionCookie removes the session cookie from the response.
func clearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		MaxAge:   -1,
	})
}

// accessTokenExpiry parses the access token claims WITHOUT verifying the signature
// and returns the expiry time. Returns zero time on parse failure.
func accessTokenExpiry(accessToken string) time.Time {
	var claims tokens.Claims
	p := jwt.NewParser()
	_, _, _ = p.ParseUnverified(accessToken, &claims)
	if claims.ExpiresAt != nil {
		return claims.ExpiresAt.Time
	}
	return time.Time{}
}

// sessionIsAdmin reports whether the active session belongs to an admin principal.
// Role is read from the access token claims without signature verification — the
// API will enforce the admin requirement on any actual admin operation.
func sessionIsAdmin(ctx context.Context) bool {
	s := sessionFromContext(ctx)
	if s == nil {
		return false
	}
	var claims tokens.Claims
	p := jwt.NewParser()
	_, _, _ = p.ParseUnverified(s.AccessToken, &claims)
	return claims.Role == tokens.RoleAdmin
}

// RequireSession is middleware that validates the session cookie.
// On success it injects the session into the request context.
// If the access token is expired it attempts a silent refresh.
// On any failure it redirects to /login.
func (h *Handler) RequireSession(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie(sessionCookie)
		if err != nil {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}

		s, err := h.decodeSession(cookie.Value)
		if err != nil {
			clearSessionCookie(w)
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}

		// Refresh the access token if it has expired or will expire in under 30 s.
		exp := accessTokenExpiry(s.AccessToken)
		if !exp.IsZero() && time.Until(exp) < 30*time.Second {
			refreshed, err := h.refreshSession(r.Context(), s.RefreshToken)
			if err != nil {
				clearSessionCookie(w)
				http.Redirect(w, r, "/login", http.StatusSeeOther)
				return
			}
			// Preserve the password-reset flag across token refreshes.
			refreshed.PasswordResetRequired = s.PasswordResetRequired
			s = refreshed
			_ = h.setSessionCookie(w, s)
		}

		// Force password change before accessing any other page.
		if s.PasswordResetRequired && r.URL.Path != "/password-reset" {
			http.Redirect(w, r, "/password-reset", http.StatusSeeOther)
			return
		}

		next.ServeHTTP(w, r.WithContext(withSession(r.Context(), s)))
	})
}

type tokenOIDCReq struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type tokenPairResponse struct {
	AccessToken           string `json:"access_token"`
	RefreshToken          string `json:"refresh_token"`
	PasswordResetRequired bool   `json:"password_reset_required,omitempty"`
}

type tokenRefreshReq struct {
	RefreshToken string `json:"refresh_token"`
}

// refreshSession exchanges a refresh token for a new token pair.
func (h *Handler) refreshSession(ctx context.Context, refreshToken string) (*session, error) {
	body, _ := json.Marshal(tokenRefreshReq{RefreshToken: refreshToken})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, h.apiURL+"/api/token/refresh", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("build refresh request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := h.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("refresh token: %w", err)
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			ctxlog.From(ctx).Warn("close response body", "err", closeErr)
		}
	}()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("refresh returned %d", resp.StatusCode)
	}
	var pair tokenPairResponse
	if err := json.NewDecoder(resp.Body).Decode(&pair); err != nil {
		return nil, fmt.Errorf("decode refresh response: %w", err)
	}
	return &session{AccessToken: pair.AccessToken, RefreshToken: pair.RefreshToken}, nil
}

// ServeLogin handles GET /login.
func (h *Handler) ServeLogin(w http.ResponseWriter, r *http.Request) {
	_ = pages.LoginPage("").Render(r.Context(), w)
}

// HandleLogin handles POST /login. It exchanges credentials for tokens via the
// REST API and stores the session in a signed HttpOnly cookie.
func (h *Handler) HandleLogin(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	username := strings.TrimSpace(r.FormValue("username"))
	password := r.FormValue("password")

	body, _ := json.Marshal(tokenOIDCReq{Username: username, Password: password})
	req, err := http.NewRequestWithContext(r.Context(), http.MethodPost, h.apiURL+"/api/token/oidc", bytes.NewReader(body))
	if err != nil {
		_ = pages.LoginPage("Login failed. Try again.").Render(r.Context(), w)
		return
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := h.httpClient.Do(req)
	if err != nil || resp.StatusCode != http.StatusOK {
		if resp != nil {
			_ = resp.Body.Close()
		}
		_ = pages.LoginPage("Invalid credentials.").Render(r.Context(), w)
		return
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			ctxlog.From(r.Context()).Warn("close response body", "err", closeErr)
		}
	}()

	var pair tokenPairResponse
	if err := json.NewDecoder(resp.Body).Decode(&pair); err != nil || pair.AccessToken == "" {
		_ = pages.LoginPage("Login failed. Try again.").Render(r.Context(), w)
		return
	}

	s := &session{
		AccessToken:           pair.AccessToken,
		RefreshToken:          pair.RefreshToken,
		PasswordResetRequired: pair.PasswordResetRequired,
	}
	if err := h.setSessionCookie(w, s); err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if pair.PasswordResetRequired {
		http.Redirect(w, r, "/password-reset", http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, "/apps", http.StatusSeeOther)
}

// HandleLogout handles POST /logout.
func (h *Handler) HandleLogout(w http.ResponseWriter, r *http.Request) {
	clearSessionCookie(w)
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}
