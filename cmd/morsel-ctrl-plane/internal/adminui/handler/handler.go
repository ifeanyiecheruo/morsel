// Package handler implements the HTTP handlers for the Morsel admin UI.
// The admin UI is a standalone service — all data access goes through the
// Morsel REST API; the handler has no direct database or platform access.
package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// Handler serves all admin UI pages and handles form submissions.
// It calls the Morsel REST API for all data; it has no direct access to the
// database, platform, or Kubernetes.
type Handler struct {
	apiURL             string
	httpClient         *http.Client
	sessionKey         []byte
	githubClientID     string
	githubClientSecret string
}

// New constructs an admin Handler. githubClientID and githubClientSecret are the
// GitHub OAuth App credentials; leave empty to disable GitHub login.
func New(apiURL string, httpClient *http.Client, sessionKey []byte, githubClientID, githubClientSecret string) *Handler {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return &Handler{
		apiURL:             apiURL,
		httpClient:         httpClient,
		sessionKey:         sessionKey,
		githubClientID:     githubClientID,
		githubClientSecret: githubClientSecret,
	}
}

// apiSessionCtxKey is the context key for the active session injected by RequireSession.
type apiSessionCtxKey struct{}

// session holds the tokens for an authenticated admin session.
type session struct {
	AccessToken  string `json:"at"`
	RefreshToken string `json:"rt"`
}

func sessionFromContext(ctx context.Context) *session {
	v, _ := ctx.Value(apiSessionCtxKey{}).(*session)
	return v
}

func withSession(ctx context.Context, s *session) context.Context {
	return context.WithValue(ctx, apiSessionCtxKey{}, s)
}

// apiGet performs a GET request to the Morsel API with the session's access token.
func (h *Handler) apiGet(ctx context.Context, path string) (*http.Response, error) {
	return h.apiDo(ctx, http.MethodGet, path, nil)
}

// apiPost performs a POST request to the Morsel API with the session's access token.
func (h *Handler) apiPost(ctx context.Context, path string, body any) (*http.Response, error) {
	buf, err := jsonBody(body)
	if err != nil {
		return nil, err
	}
	return h.apiDo(ctx, http.MethodPost, path, buf)
}

// apiDelete performs a DELETE request to the Morsel API.
func (h *Handler) apiDelete(ctx context.Context, path string) (*http.Response, error) {
	return h.apiDo(ctx, http.MethodDelete, path, nil)
}

// apiDo issues an HTTP request to the REST API. The session access token is
// read from the request context (injected by RequireSession).
func (h *Handler) apiDo(ctx context.Context, method, path string, body io.Reader) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, method, h.apiURL+path, body)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if s := sessionFromContext(ctx); s != nil {
		req.Header.Set("Authorization", "Bearer "+s.AccessToken)
	}
	return h.httpClient.Do(req)
}

func jsonBody(v any) (io.Reader, error) {
	if v == nil {
		return http.NoBody, nil
	}
	b, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	return bytes.NewReader(b), nil
}
