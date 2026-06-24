package local_test

import (
	"context"
	"errors"
	"log/slog"
	"testing"

	"github.com/ifeanyiecheruo/morsel/internal/ctxlog"
	"github.com/ifeanyiecheruo/morsel/internal/db"
	dbqueries "github.com/ifeanyiecheruo/morsel/internal/db/queries"
	"github.com/ifeanyiecheruo/morsel/internal/platform"
	"github.com/ifeanyiecheruo/morsel/internal/platform/local"
	"github.com/ifeanyiecheruo/morsel/internal/store"
)

var ctx = ctxlog.With(context.Background(), slog.Default())

func TestAmbientTokenReturnsEmpty(t *testing.T) {
	plat := local.New(nil)
	token, err := plat.Secrets().AmbientToken(ctx)
	if err != nil {
		t.Fatalf("AmbientToken: unexpected error: %v", err)
	}
	if token != "" {
		t.Errorf("token = %q, want empty string", token)
	}
}

func TestValidateDeployTokenRejectsInvalidToken(t *testing.T) {
	plat := platWithTempHome(t)
	_, err := plat.Secrets().ValidateDeployToken(ctx, "not-a-valid-token")
	if err == nil {
		t.Error("ValidateDeployToken: expected error for invalid token, got nil")
	}
}

func TestDeployTokenRoundTrip(t *testing.T) {
	plat := platWithTempHome(t)

	token, err := plat.Secrets().DeployToken(ctx)
	if err != nil {
		t.Fatalf("DeployToken: %v", err)
	}
	slug, err := plat.Secrets().ValidateDeployToken(ctx, token)
	if err != nil {
		t.Fatalf("ValidateDeployToken: %v", err)
	}
	if slug == "" {
		t.Error("ValidateDeployToken: returned empty slug")
	}
}

// platWithTempHome creates a LocalPlatform (no store) whose secrets file points
// at a temporary directory so tests never touch ~/.morsel/local/secrets.json.
func platWithTempHome(t *testing.T) *local.LocalPlatform {
	t.Helper()
	tmp := t.TempDir()
	t.Setenv("USERPROFILE", tmp) // Windows
	t.Setenv("HOME", tmp)        // Linux / macOS
	return local.New(nil)
}

// platWithStore creates a LocalPlatform backed by an in-memory SQLite store for
// tests that exercise principal validation or SeedDefaults.
func platWithStore(t *testing.T) (*local.LocalPlatform, *store.Store) {
	t.Helper()
	tmp := t.TempDir()
	t.Setenv("USERPROFILE", tmp) // Windows
	t.Setenv("HOME", tmp)        // Linux / macOS

	database, err := db.Open(ctx, ":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })

	if err := db.Migrate(ctx, database); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	s := store.New(dbqueries.New(database))
	return local.New(s), s
}

func TestDNSCreateRecordIsNoop(t *testing.T) {
	plat := local.New(nil)
	if err := plat.DNS().CreateRecord(ctx, "zone", "name", "A", "1.2.3.4", 60); err != nil {
		t.Errorf("CreateRecord: unexpected error: %v", err)
	}
}

func TestDNSDeleteRecordIsNoop(t *testing.T) {
	plat := local.New(nil)
	if err := plat.DNS().DeleteRecord(ctx, "zone", "name", "A"); err != nil {
		t.Errorf("DeleteRecord: unexpected error: %v", err)
	}
}

func TestDNSRecordExistsReturnsFalse(t *testing.T) {
	plat := local.New(nil)
	exists, err := plat.DNS().RecordExists(ctx, "zone", "name", "A")
	if err != nil {
		t.Fatalf("RecordExists: unexpected error: %v", err)
	}
	if exists {
		t.Error("RecordExists = true, want false")
	}
}

func TestPricesFetchedAtIsSet(t *testing.T) {
	plat := local.New(nil)
	prices, err := plat.Pricing().Prices(ctx)
	if err != nil {
		t.Fatalf("Prices: unexpected error: %v", err)
	}
	if prices.FetchedAt.IsZero() {
		t.Error("Prices.FetchedAt is zero, want non-zero")
	}
}

func TestSeedDefaultsWritesWhenAbsent(t *testing.T) {
	plat, _ := platWithStore(t)
	if err := plat.SeedDefaults(ctx); err != nil {
		t.Fatalf("SeedDefaults: %v", err)
	}
	subject, err := plat.Secrets().ValidateOperatorCredential(ctx, "operator@example.com", "")
	if err != nil {
		t.Fatalf("ValidateOperatorCredential after SeedDefaults: %v", err)
	}
	if subject != "operator@example.com" {
		t.Errorf("subject = %q, want operator@example.com", subject)
	}
}

func TestSeedDefaultsIsNoOpWhenAlreadySet(t *testing.T) {
	plat, s := platWithStore(t)
	seedPrincipals(t, s, "custom@example.com")

	if err := plat.SeedDefaults(ctx); err != nil {
		t.Fatalf("SeedDefaults: %v", err)
	}
	// The pre-existing principal must still authenticate.
	subject, err := plat.Secrets().ValidateOperatorCredential(ctx, "custom@example.com", "")
	if err != nil {
		t.Fatalf("ValidateOperatorCredential: %v", err)
	}
	if subject != "custom@example.com" {
		t.Errorf("subject = %q, want custom@example.com", subject)
	}
	// The default principal must NOT have been injected.
	if _, err := plat.Secrets().ValidateOperatorCredential(ctx, "operator@example.com", ""); !errors.Is(err, platform.ErrPrincipalNotAuthorized) {
		t.Errorf("expected ErrPrincipalNotAuthorized for default principal after SeedDefaults no-op, got %v", err)
	}
}

func TestBootstrapPromptsReturnsExpectedKeys(t *testing.T) {
	plat := local.New(nil)
	prompts := plat.Bootstrap().Prompts()
	if len(prompts) == 0 {
		t.Fatal("Bootstrap.Prompts: returned no prompts")
	}
	keys := make(map[string]bool, len(prompts))
	for _, p := range prompts {
		keys[p.Key] = true
	}
	for _, want := range []string{"github_org", "domain", "k8s_namespace"} {
		if !keys[want] {
			t.Errorf("Bootstrap.Prompts: missing prompt key %q", want)
		}
	}
}

func TestBootstrapPlanReturnsResources(t *testing.T) {
	plat := local.New(nil)
	plan := plat.Bootstrap().Plan(map[string]string{})
	if plan.Summary == "" {
		t.Error("Bootstrap.Plan: empty summary")
	}
	if len(plan.Resources) == 0 {
		t.Error("Bootstrap.Plan: no resources listed")
	}
}

func TestBootstrapProvisionWritesConfigAndKey(t *testing.T) {
	plat := platWithTempHome(t)
	answers := map[string]string{
		"github_org": "my-org",
		"domain":     "morsel.localhost",
	}
	if err := plat.Bootstrap().Provision(ctx, answers); err != nil {
		t.Fatalf("Bootstrap.Provision: unexpected error: %v", err)
	}
	// Deploy signing key must have been generated.
	_, err := plat.Deploy().Credentials(ctx)
	if err == nil {
		t.Fatal("Deploy.Credentials: expected error for missing deploy key, got nil")
	} else {
		if !errors.Is(err, platform.ErrPrincipalNotAuthorized) {
			t.Fatalf("Deploy.Credentials: expected ErrPrincipalNotAuthorized for missing deploy key, got %v", err)
		}
	}

	token, err := plat.Secrets().DeployToken(ctx)
	if err != nil {
		t.Fatalf("DeployToken after Provision: %v", err)
	}
	if _, err := plat.Secrets().ValidateDeployToken(ctx, token); err != nil {
		t.Errorf("ValidateDeployToken after Provision: %v", err)
	}
}

func TestBootstrapProvisionIsIdempotent(t *testing.T) {
	plat := platWithTempHome(t)
	answers := map[string]string{"github_org": "my-org", "domain": "morsel.localhost"}
	if err := plat.Bootstrap().Provision(ctx, answers); err != nil {
		t.Fatalf("first Provision: %v", err)
	}
	if err := plat.Bootstrap().Provision(ctx, answers); err != nil {
		t.Fatalf("second Provision (idempotent): %v", err)
	}
}

func TestDeployCredentialsNotImplemented(t *testing.T) {
	plat := local.New(nil)
	if _, err := plat.Deploy().Credentials(ctx); !errors.Is(err, platform.ErrNotImplemented) {
		t.Errorf("Deploy.Credentials: err = %v, want ErrNotImplemented", err)
	}
}

func TestBlobsGetNotImplemented(t *testing.T) {
	plat := local.New(nil)
	if _, err := plat.Blobs().Get(ctx, "bucket", "key"); !errors.Is(err, platform.ErrNotImplemented) {
		t.Errorf("Blobs.Get: err = %v, want ErrNotImplemented", err)
	}
}
