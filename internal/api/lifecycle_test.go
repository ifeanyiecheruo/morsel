package api_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ifeanyiecheruo/morsel/internal/platform/local"
	"github.com/ifeanyiecheruo/morsel/internal/tokens"
)

// ── test environment ──────────────────────────────────────────────────────────

type testEnv struct {
	mux      http.Handler
	opToken  string
	devToken string
}

// newEnv creates a mux backed by an in-memory database and pre-issues operator
// and developer tokens signed with the same key as the mux.
func newEnv(t *testing.T) *testEnv {
	t.Helper()
	plat, _, mux := testSetup(t)
	return &testEnv{
		mux:      mux,
		opToken:  mustIssueOperator(t, plat),
		devToken: mustIssueDeveloper(t, plat, "myorg/myrepo"),
	}
}

func mustIssueOperator(t *testing.T, plat *local.LocalPlatform) string {
	t.Helper()
	key := mustSigningKey(t, plat)
	tok, err := tokens.IssueToken(key, tokens.CreateOperatorClaims("test-operator"))
	if err != nil {
		t.Fatalf("issue operator token: %v", err)
	}
	return tok
}

func mustIssueDeveloper(t *testing.T, plat *local.LocalPlatform, slug string) string {
	t.Helper()
	key := mustSigningKey(t, plat)
	tok, err := tokens.IssueToken(key, tokens.CreateDeployClaims(slug))
	if err != nil {
		t.Fatalf("issue developer token for %q: %v", slug, err)
	}
	return tok
}

func mustSigningKey(t *testing.T, plat *local.LocalPlatform) []byte {
	t.Helper()
	keys, err := plat.Secrets().GetSigningKeys(context.Background())
	if err != nil || len(keys) == 0 {
		t.Fatalf("get signing key: %v", err)
	}
	return keys[0]
}

// authed adds a Bearer Authorization header to the request.
func authed(req *http.Request, token string) *http.Request {
	req.Header.Set("Authorization", "Bearer "+token)
	return req
}

// jsonReq builds a request with a JSON body.
func jsonReq(method, target, body string) *http.Request {
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	return req
}

// deployBody returns a minimal UpsertApp request body.
func deployBody(name, appType, image string) string {
	if name == "" {
		return fmt.Sprintf(`{"type":%q,"image":%q}`, appType, image)
	}
	return fmt.Sprintf(`{"name":%q,"type":%q,"image":%q}`, name, appType, image)
}

// do executes a request against the mux and returns the recorder.
func (e *testEnv) do(req *http.Request) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	e.mux.ServeHTTP(rec, req)
	return rec
}

// doJSON executes a JSON request with a bearer token.
func (e *testEnv) doJSON(method, target, body, token string) *httptest.ResponseRecorder {
	return e.do(authed(jsonReq(method, target, body), token))
}

// get executes a GET request with a bearer token.
func (e *testEnv) get(target, token string) *httptest.ResponseRecorder {
	return e.do(authed(httptest.NewRequest(http.MethodGet, target, nil), token))
}

// ── UpsertApp ─────────────────────────────────────────────────────────────────

func TestUpsertAppReturns202WithLocation(t *testing.T) {
	env := newEnv(t)
	rec := env.doJSON(http.MethodPost, "/api/repos/myorg/myrepo/apps", deployBody("api", "http", "ghcr.io/org/api:v1"), env.devToken)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want 202; body: %s", rec.Code, rec.Body)
	}
	if loc := rec.Header().Get("Location"); loc == "" {
		t.Error("Location header is empty")
	}
	var body struct {
		OperationID string `json:"operation_id"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.OperationID == "" {
		t.Error("operation_id is empty")
	}
}

func TestUpsertAppRequiresAuth(t *testing.T) {
	env := newEnv(t)
	rec := env.do(jsonReq(http.MethodPost, "/api/repos/myorg/myrepo/apps", deployBody("api", "http", "img:v1")))
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401; body: %s", rec.Code, rec.Body)
	}
}

func TestUpsertAppRejectsCrossRepoToken(t *testing.T) {
	env := newEnv(t)
	rec := env.doJSON(http.MethodPost, "/api/repos/otherorg/otherrepo/apps", deployBody("api", "http", "img:v1"), env.devToken)
	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403", rec.Code)
	}
}

// ── GetApp ────────────────────────────────────────────────────────────────────

func TestGetAppReturnsApp(t *testing.T) {
	env := newEnv(t)

	rec := env.doJSON(http.MethodPost, "/api/repos/myorg/myrepo/apps", deployBody("api", "http", "ghcr.io/org/api:v1"), env.devToken)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("upsert status = %d; body: %s", rec.Code, rec.Body)
	}

	rec = env.get("/api/repos/myorg/myrepo/apps/api", env.devToken)
	if rec.Code != http.StatusOK {
		t.Fatalf("get status = %d; body: %s", rec.Code, rec.Body)
	}
	var body struct {
		Type string `json:"type"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Type != "http" {
		t.Errorf("type = %q, want http", body.Type)
	}
}

func TestGetAppNotFoundReturns404(t *testing.T) {
	env := newEnv(t)
	rec := env.get("/api/repos/myorg/myrepo/apps/missing", env.devToken)
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rec.Code)
	}
}

// ── ListApps ──────────────────────────────────────────────────────────────────

func TestListAppsReturnsDeployedApp(t *testing.T) {
	env := newEnv(t)

	rec := env.doJSON(http.MethodPost, "/api/repos/myorg/myrepo/apps", deployBody("worker", "worker", "ghcr.io/org/worker:v1"), env.devToken)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("upsert status = %d; body: %s", rec.Code, rec.Body)
	}

	rec = env.get("/api/repos/myorg/myrepo/apps", env.devToken)
	if rec.Code != http.StatusOK {
		t.Fatalf("list status = %d; body: %s", rec.Code, rec.Body)
	}
	var apps []struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&apps); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(apps) != 1 {
		t.Fatalf("len(apps) = %d, want 1", len(apps))
	}
	if apps[0].Name != "worker" {
		t.Errorf("apps[0].Name = %q, want worker", apps[0].Name)
	}
}

// ── DeleteApp ─────────────────────────────────────────────────────────────────

func TestDeleteAppReturns202(t *testing.T) {
	env := newEnv(t)

	env.doJSON(http.MethodPost, "/api/repos/myorg/myrepo/apps", deployBody("api", "http", "img:v1"), env.devToken)

	rec := env.do(authed(httptest.NewRequest(http.MethodDelete, "/api/repos/myorg/myrepo/apps/api", nil), env.devToken))
	if rec.Code != http.StatusAccepted {
		t.Fatalf("delete status = %d; body: %s", rec.Code, rec.Body)
	}
	var body struct {
		OperationID string `json:"operation_id"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.OperationID == "" {
		t.Error("operation_id is empty")
	}
}

func TestDeleteAppNotFoundReturns404(t *testing.T) {
	env := newEnv(t)
	rec := env.do(authed(httptest.NewRequest(http.MethodDelete, "/api/repos/myorg/myrepo/apps/missing", nil), env.devToken))
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rec.Code)
	}
}

func TestDeletedAppNoLongerInList(t *testing.T) {
	env := newEnv(t)

	env.doJSON(http.MethodPost, "/api/repos/myorg/myrepo/apps", deployBody("api", "http", "img:v1"), env.devToken)
	env.do(authed(httptest.NewRequest(http.MethodDelete, "/api/repos/myorg/myrepo/apps/api", nil), env.devToken))

	rec := env.get("/api/repos/myorg/myrepo/apps", env.devToken)
	var apps []interface{}
	if err := json.NewDecoder(rec.Body).Decode(&apps); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(apps) != 0 {
		t.Errorf("len(apps) = %d after delete, want 0", len(apps))
	}
}

// ── GetAppStatus ──────────────────────────────────────────────────────────────

func TestGetAppStatusReturnsUnknown(t *testing.T) {
	env := newEnv(t)

	env.doJSON(http.MethodPost, "/api/repos/myorg/myrepo/apps", deployBody("api", "http", "img:v1"), env.devToken)

	rec := env.get("/api/repos/myorg/myrepo/apps/api/status", env.devToken)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; body: %s", rec.Code, rec.Body)
	}
	var body struct {
		Status string `json:"status"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Status != "unknown" {
		t.Errorf("status = %q, want unknown", body.Status)
	}
}

// ── GetOperation ──────────────────────────────────────────────────────────────

func TestGetOperationReturnsComplete(t *testing.T) {
	env := newEnv(t)

	rec := env.doJSON(http.MethodPost, "/api/repos/myorg/myrepo/apps", deployBody("api", "http", "img:v1"), env.devToken)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("upsert status = %d; body: %s", rec.Code, rec.Body)
	}
	loc := rec.Header().Get("Location")
	if loc == "" {
		t.Fatal("no Location header")
	}

	rec = env.get(loc, env.devToken)
	if rec.Code != http.StatusOK {
		t.Fatalf("get operation status = %d; body: %s", rec.Code, rec.Body)
	}
	var body struct {
		Status string `json:"status"`
		Type   string `json:"type"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Status != "complete" {
		t.Errorf("status = %q, want complete", body.Status)
	}
	if body.Type != "deploy" {
		t.Errorf("type = %q, want deploy", body.Type)
	}
}

func TestGetOperationNotFoundReturns404(t *testing.T) {
	env := newEnv(t)
	rec := env.get("/api/repos/myorg/myrepo/apps/api/operations/op_notexist", env.devToken)
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rec.Code)
	}
}

// ── GetAppHistory ─────────────────────────────────────────────────────────────

func TestGetAppHistoryContainsDeploy(t *testing.T) {
	env := newEnv(t)

	env.doJSON(http.MethodPost, "/api/repos/myorg/myrepo/apps", deployBody("api", "http", "img:v1"), env.devToken)

	rec := env.get("/api/repos/myorg/myrepo/apps/api/history", env.devToken)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; body: %s", rec.Code, rec.Body)
	}
	var ops []struct {
		Type   string `json:"type"`
		Status string `json:"status"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&ops); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(ops) == 0 {
		t.Fatal("expected at least one operation in history")
	}
	if ops[0].Type != "deploy" {
		t.Errorf("ops[0].type = %q, want deploy", ops[0].Type)
	}
	if ops[0].Status != "complete" {
		t.Errorf("ops[0].status = %q, want complete", ops[0].Status)
	}
}

// ── GetRepo ───────────────────────────────────────────────────────────────────

func TestGetRepoReturnsRepoAfterDeploy(t *testing.T) {
	env := newEnv(t)

	env.doJSON(http.MethodPost, "/api/repos/myorg/myrepo/apps", deployBody("api", "http", "img:v1"), env.devToken)

	rec := env.get("/api/repos/myorg/myrepo", env.devToken)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; body: %s", rec.Code, rec.Body)
	}
	var body struct {
		Slug     string `json:"slug"`
		Tier     string `json:"tier"`
		AppCount int    `json:"app_count"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Slug != "myorg/myrepo" {
		t.Errorf("slug = %q, want myorg/myrepo", body.Slug)
	}
	if body.AppCount != 1 {
		t.Errorf("app_count = %d, want 1", body.AppCount)
	}
}

func TestGetRepoNotFoundReturns404(t *testing.T) {
	env := newEnv(t)
	rec := env.get("/api/repos/myorg/myrepo", env.devToken)
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rec.Code)
	}
}

// ── ListRepos ─────────────────────────────────────────────────────────────────

func TestListReposOperatorAllReturnsAllRepos(t *testing.T) {
	env := newEnv(t)

	env.doJSON(http.MethodPost, "/api/repos/myorg/myrepo/apps", deployBody("api", "http", "img:v1"), env.devToken)

	rec := env.get("/api/repos?all=true", env.opToken)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; body: %s", rec.Code, rec.Body)
	}
	var repos []struct {
		Slug string `json:"slug"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&repos); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(repos) != 1 {
		t.Fatalf("len(repos) = %d, want 1", len(repos))
	}
	if repos[0].Slug != "myorg/myrepo" {
		t.Errorf("repos[0].Slug = %q, want myorg/myrepo", repos[0].Slug)
	}
}

func TestListReposDeveloperReturnsOwnRepo(t *testing.T) {
	env := newEnv(t)

	env.doJSON(http.MethodPost, "/api/repos/myorg/myrepo/apps", deployBody("api", "http", "img:v1"), env.devToken)

	rec := env.get("/api/repos", env.devToken)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; body: %s", rec.Code, rec.Body)
	}
	var repos []struct {
		Slug string `json:"slug"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&repos); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(repos) != 1 {
		t.Fatalf("len(repos) = %d, want 1", len(repos))
	}
	if repos[0].Slug != "myorg/myrepo" {
		t.Errorf("repos[0].Slug = %q, want myorg/myrepo", repos[0].Slug)
	}
}

// ── SyncRepo ──────────────────────────────────────────────────────────────────

func TestSyncRepoUpsertsDeclaredApps(t *testing.T) {
	env := newEnv(t)

	syncBody := `{"apps":[{"name":"api","type":"http","image":"img:v1"},{"name":"worker","type":"worker","image":"img:v2"}]}`
	rec := env.doJSON(http.MethodPost, "/api/repos/myorg/myrepo/sync", syncBody, env.devToken)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("sync status = %d; body: %s", rec.Code, rec.Body)
	}

	rec = env.get("/api/repos/myorg/myrepo/apps", env.devToken)
	var apps []struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&apps); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(apps) != 2 {
		t.Errorf("len(apps) = %d, want 2", len(apps))
	}
}

func TestSyncRepoMarksMissingAppsForDeletion(t *testing.T) {
	env := newEnv(t)

	env.doJSON(http.MethodPost, "/api/repos/myorg/myrepo/apps", deployBody("api", "http", "img:v1"), env.devToken)
	env.doJSON(http.MethodPost, "/api/repos/myorg/myrepo/apps", deployBody("worker", "worker", "img:v2"), env.devToken)

	// Sync with only "api" — "worker" should be marked for deletion.
	syncBody := `{"apps":[{"name":"api","type":"http","image":"img:v1"}]}`
	rec := env.doJSON(http.MethodPost, "/api/repos/myorg/myrepo/sync", syncBody, env.devToken)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("sync status = %d; body: %s", rec.Code, rec.Body)
	}

	rec = env.get("/api/repos/myorg/myrepo/apps", env.devToken)
	var apps []struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&apps); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(apps) != 1 {
		t.Errorf("len(apps) = %d after sync, want 1", len(apps))
	}
	if apps[0].Name != "api" {
		t.Errorf("remaining app = %q, want api", apps[0].Name)
	}
}

// ── Namespace naming ──────────────────────────────────────────────────────────

func TestUpsertAppSetsNamespace(t *testing.T) {
	env := newEnv(t)

	env.doJSON(http.MethodPost, "/api/repos/myorg/myrepo/apps", deployBody("api", "http", "img:v1"), env.devToken)

	rec := env.get("/api/repos/myorg/myrepo/apps/api", env.devToken)
	var body struct {
		Namespace string `json:"namespace"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	// org=myorg, repo=myrepo, name=api → myorg-myrepo--api
	if body.Namespace != "myorg-myrepo--api" {
		t.Errorf("namespace = %q, want myorg-myrepo--api", body.Namespace)
	}
}
