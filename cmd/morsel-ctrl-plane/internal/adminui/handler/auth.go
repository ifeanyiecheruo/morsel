package handler

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/ifeanyiecheruo/morsel/cmd/morsel-ctrl-plane/internal/adminui/pages"
	"github.com/ifeanyiecheruo/morsel/cmd/morsel-ctrl-plane/internal/tokens"
	"github.com/ifeanyiecheruo/morsel/internal/ctxlog"
)

const (
	sessionCookie    = "morsel_admin"
	oauthStateCookie = "morsel_oauth_state"
)

// ── Session cookie ────────────────────────────────────────────────────────────

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

// ── Session JWT helpers ───────────────────────────────────────────────────────

// accessTokenExpiry parses the access token claims WITHOUT verifying the signature
// and returns the expiry time. Returns zero time on parse failure.
func accessTokenExpiry(ctx context.Context, accessToken string) time.Time {
	var claims tokens.Claims
	p := jwt.NewParser()
	if _, _, err := p.ParseUnverified(accessToken, &claims); err != nil {
		ctxlog.From(ctx).Warn("parse access token expiry", "err", err)
	}
	if claims.ExpiresAt != nil {
		return claims.ExpiresAt.Time
	}
	return time.Time{}
}

// sessionIsAdmin reports whether the active session belongs to an admin principal.
func sessionIsAdmin(ctx context.Context) bool {
	s := sessionFromContext(ctx)
	if s == nil {
		return false
	}
	var claims tokens.Claims
	p := jwt.NewParser()
	if _, _, err := p.ParseUnverified(s.AccessToken, &claims); err != nil {
		ctxlog.From(ctx).Warn("parse session token role", "err", err)
	}
	return claims.Role == tokens.RoleAdmin
}

// ── Session middleware ────────────────────────────────────────────────────────

type tokenRefreshReq struct {
	RefreshToken string `json:"refresh_token"`
}

type tokenPairResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
}

// refreshSession exchanges a refresh token for a new token pair.
func (h *Handler) refreshSession(ctx context.Context, refreshToken string) (*session, error) {
	body, err := json.Marshal(tokenRefreshReq{RefreshToken: refreshToken})
	if err != nil {
		ctxlog.From(ctx).Error("marshal token refresh request", "err", err)
	}
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
		exp := accessTokenExpiry(r.Context(), s.AccessToken)
		if !exp.IsZero() && time.Until(exp) < 30*time.Second {
			refreshed, err := h.refreshSession(r.Context(), s.RefreshToken)
			if err != nil {
				clearSessionCookie(w)
				http.Redirect(w, r, "/login", http.StatusSeeOther)
				return
			}
			s = refreshed
			if err := h.setSessionCookie(w, s); err != nil {
				ctxlog.From(r.Context()).Warn("set session cookie", "err", err)
			}
		}

		next.ServeHTTP(w, r.WithContext(withSession(r.Context(), s)))
	})
}

// ── GitHub OAuth ──────────────────────────────────────────────────────────────

// callbackURL reconstructs the OAuth callback URL from the incoming request.
func callbackURL(r *http.Request) string {
	scheme := "https"
	if r.TLS == nil && r.Header.Get("X-Forwarded-Proto") != "https" {
		scheme = "http"
	}
	return scheme + "://" + r.Host + "/oidc/callback"
}

// ServeLogin handles GET /login — redirects immediately to GitHub OAuth.
func (h *Handler) ServeLogin(w http.ResponseWriter, r *http.Request) {
	if h.githubClientID == "" {
		ctx := r.Context()
		if err := pages.LoginPage("GitHub OAuth is not configured for this deployment.").Render(ctx, w); err != nil {
			ctxlog.From(ctx).Warn("render login page", "err", err)
		}
		return
	}

	// Generate a cryptographically random CSRF state token.
	stateBytes := make([]byte, 16)
	if _, err := rand.Read(stateBytes); err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	state := hex.EncodeToString(stateBytes)

	// Store state in a short-lived HttpOnly cookie for CSRF verification on callback.
	http.SetCookie(w, &http.Cookie{
		Name:     oauthStateCookie,
		Value:    state,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   600, // 10 minutes
	})

	authURL := "https://github.com/login/oauth/authorize?" + url.Values{
		"client_id":    {h.githubClientID},
		"redirect_uri": {callbackURL(r)},
		"scope":        {"read:user"},
		"state":        {state},
	}.Encode()

	http.Redirect(w, r, authURL, http.StatusFound)
}

// HandleGitHubCallback handles GET /oidc/callback.
func (h *Handler) HandleGitHubCallback(w http.ResponseWriter, r *http.Request) {
	log := ctxlog.From(r.Context())

	// Validate CSRF state.
	stateCookie, err := r.Cookie(oauthStateCookie)
	if err != nil || stateCookie.Value != r.URL.Query().Get("state") {
		log.Warn("github oauth callback: state mismatch")
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}
	// Clear the state cookie — single use.
	http.SetCookie(w, &http.Cookie{Name: oauthStateCookie, Value: "", Path: "/", MaxAge: -1})

	code := r.URL.Query().Get("code")
	if code == "" {
		log.Warn("github oauth callback: missing code")
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	// Exchange the code for a GitHub access token.
	ghToken, err := h.exchangeGitHubCode(r.Context(), code, callbackURL(r))
	if err != nil {
		log.Error("github oauth callback: code exchange failed", "err", err)
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	// Exchange the GitHub token for a Morsel session.
	s, err := h.githubTokenToSession(r.Context(), ghToken)
	if err != nil {
		log.Warn("github oauth callback: morsel token exchange failed", "err", err)
		if renderErr := pages.LoginPage("Your GitHub account does not have access to this platform.").Render(r.Context(), w); renderErr != nil {
			log.Warn("render login page", "err", renderErr)
		}
		return
	}

	if err := h.setSessionCookie(w, s); err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/apps", http.StatusSeeOther)
}

type githubAccessTokenResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	Error       string `json:"error"`
}

// exchangeGitHubCode exchanges a GitHub OAuth code for a GitHub access token.
func (h *Handler) exchangeGitHubCode(ctx context.Context, code, redirectURI string) (string, error) {
	body, err := json.Marshal(map[string]string{
		"client_id":     h.githubClientID,
		"client_secret": h.githubClientSecret,
		"code":          code,
		"redirect_uri":  redirectURI,
	})
	if err != nil {
		return "", fmt.Errorf("marshal github token request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://github.com/login/oauth/access_token", bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("build github token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := h.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("exchange github code: %w", err)
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			ctxlog.From(ctx).Warn("close response body", "err", closeErr)
		}
	}()

	var result githubAccessTokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("decode github token response: %w", err)
	}
	if result.Error != "" {
		return "", fmt.Errorf("github token error: %s", result.Error)
	}
	if result.AccessToken == "" {
		return "", errors.New("github returned empty access token")
	}
	return result.AccessToken, nil
}

type githubAuthRequest struct {
	Token string `json:"token"`
}

// githubTokenToSession calls the Morsel API to exchange a GitHub token for
// Morsel tokens, then returns a session containing those tokens.
func (h *Handler) githubTokenToSession(ctx context.Context, githubToken string) (*session, error) {
	body, err := json.Marshal(githubAuthRequest{Token: githubToken})
	if err != nil {
		return nil, fmt.Errorf("marshal morsel auth request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, h.apiURL+"/token/oidc", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("build morsel auth request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := h.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("morsel github auth: %w", err)
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			ctxlog.From(ctx).Warn("close response body", "err", closeErr)
		}
	}()

	if resp.StatusCode == http.StatusForbidden {
		return nil, errors.New("principal has viewer access only")
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("morsel github auth returned %d", resp.StatusCode)
	}

	var pair tokenPairResponse
	if err := json.NewDecoder(resp.Body).Decode(&pair); err != nil {
		return nil, fmt.Errorf("decode morsel token response: %w", err)
	}
	return &session{AccessToken: pair.AccessToken, RefreshToken: pair.RefreshToken}, nil
}

// HandleLogout handles POST /logout.
func (h *Handler) HandleLogout(w http.ResponseWriter, r *http.Request) {
	clearSessionCookie(w)
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}
