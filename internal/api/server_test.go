package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ifeanyiecheruo/morsel/internal/api"
	"github.com/ifeanyiecheruo/morsel/internal/db"
	dbqueries "github.com/ifeanyiecheruo/morsel/internal/db/queries"
	"github.com/ifeanyiecheruo/morsel/platform/local"
)

// jsonPost returns a POST request with Content-Type: application/json.
func jsonPost(target, body string) *http.Request {
	req := httptest.NewRequest(http.MethodPost, target, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	return req
}

// testKey is a fixed 32-byte key used in tests — never used outside tests.
var testKey = make([]byte, 32)

func newTestMux(t *testing.T) http.Handler {
	t.Helper()
	return newTestMuxWithPlatform(t, local.New())
}

func newTestMuxWithPlatform(t *testing.T, plat api.AppPlatform) http.Handler {
	t.Helper()
	database, err := db.Open(context.Background(), ":memory:")
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	if err := db.Migrate(context.Background(), database); err != nil {
		t.Fatalf("migrate test database: %v", err)
	}
	return api.NewMux(context.Background(), plat, testKey, dbqueries.New(database))
}

func TestHealthzReturnsOK(t *testing.T) {
	mux := newTestMux(t)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/healthz", nil))

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
	var body map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body["status"] != "ok" {
		t.Errorf("status = %q, want ok", body["status"])
	}
}

func TestUnregisteredRouteReturnsStructured404(t *testing.T) {
	mux := newTestMux(t)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/no/such/path", nil))

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
	var body struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Error.Code != "not_found" {
		t.Errorf("code = %q, want not_found", body.Error.Code)
	}
}

func TestHealthzIgnoresWrongMethod(t *testing.T) {
	mux := newTestMux(t)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/healthz", nil))

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want 405", rec.Code)
	}
}

func TestTokenOIDCIssuesBothTokens(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("USERPROFILE", tmp)
	t.Setenv("HOME", tmp)
	plat := local.New()
	principals, _ := json.Marshal([]string{"alice@example.com"})
	if err := plat.Secrets().Set(context.Background(), "operator-principals", principals); err != nil {
		t.Fatalf("seed principals: %v", err)
	}

	mux := newTestMuxWithPlatform(t, plat)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, jsonPost("/api/token/oidc", `{"username":"alice@example.com","password":""}`))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rec.Code, rec.Body)
	}
	var body struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    int    `json:"expires_in"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.AccessToken == "" {
		t.Error("access_token is empty")
	}
	if body.RefreshToken == "" {
		t.Error("refresh_token is empty")
	}
	if body.ExpiresIn != 900 {
		t.Errorf("expires_in = %d, want 900", body.ExpiresIn)
	}
}

func TestTokenOIDCRejectsUnknownPrincipal(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("USERPROFILE", tmp)
	t.Setenv("HOME", tmp)
	plat := local.New()
	principals, _ := json.Marshal([]string{"alice@example.com"})
	if err := plat.Secrets().Set(context.Background(), "operator-principals", principals); err != nil {
		t.Fatalf("seed principals: %v", err)
	}

	mux := newTestMuxWithPlatform(t, plat)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, jsonPost("/api/token/oidc", `{"username":"eve@example.com","password":""}`))

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rec.Code)
	}
}
