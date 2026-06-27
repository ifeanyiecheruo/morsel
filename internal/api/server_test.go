package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ifeanyiecheruo/morsel/internal/api"
	"github.com/ifeanyiecheruo/morsel/internal/api/handler"
	"github.com/ifeanyiecheruo/morsel/internal/db"
	dbqueries "github.com/ifeanyiecheruo/morsel/internal/db/queries"
	"github.com/ifeanyiecheruo/morsel/internal/kube"
	"github.com/ifeanyiecheruo/morsel/internal/platform/local"
	"github.com/ifeanyiecheruo/morsel/internal/store"
)

// fakeDeployer satisfies handler.Deployer and immediately succeeds every
// operation, allowing tests to verify API contracts without a real cluster.
type fakeDeployer struct{}

func (fakeDeployer) Apply(_ context.Context, _ kube.AppManifest) error        { return nil }
func (fakeDeployer) Delete(_ context.Context, _ string) error                 { return nil }
func (fakeDeployer) WatchDeploymentRollout(_ context.Context, _ string) error { return nil }
func (fakeDeployer) RollbackDeployment(_ context.Context, _, _ string) error  { return nil }
func (fakeDeployer) AppStatus(_ context.Context, _, _ string) string          { return "running" }
func (fakeDeployer) GetTLSCertExpiry(_ context.Context, _, _ string) (*time.Time, error) {
	return nil, nil
}

var _ handler.Deployer = fakeDeployer{}

// jsonPost returns a POST request with Content-Type: application/json.
func jsonPost(target, body string) *http.Request {
	req := httptest.NewRequest(http.MethodPost, target, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	return req
}

func newTestMux(t *testing.T) http.Handler {
	t.Helper()
	_, _, mux := testSetup(t)
	return mux
}

// testSetup creates a platform backed by an in-memory DB and the API mux.
func testSetup(t *testing.T) (*local.LocalPlatform, *store.Store, http.Handler) {
	t.Helper()
	tmp := t.TempDir()
	t.Setenv("USERPROFILE", tmp)
	t.Setenv("HOME", tmp)

	database, err := db.Open(context.Background(), ":memory:")
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	if err := db.Migrate(context.Background(), database); err != nil {
		t.Fatalf("migrate test database: %v", err)
	}

	s := store.New(dbqueries.New(database))
	plat := local.New(s)
	if err := plat.Secrets().Migrate(context.Background()); err != nil {
		t.Fatalf("secret migration: %v", err)
	}
	mux := api.NewMux(context.Background(), plat, s, &fakeDeployer{})
	return plat, s, mux
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
	_, s, mux := testSetup(t)
	if err := s.AddPrincipal(context.Background(), "alice@example.com"); err != nil {
		t.Fatalf("seed principal: %v", err)
	}

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
	_, s, mux := testSetup(t)
	if err := s.AddPrincipal(context.Background(), "alice@example.com"); err != nil {
		t.Fatalf("seed principal: %v", err)
	}

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, jsonPost("/api/token/oidc", `{"username":"eve@example.com","password":""}`))

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rec.Code)
	}
}
