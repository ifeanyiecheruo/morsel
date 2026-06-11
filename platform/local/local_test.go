package local_test

import (
	"context"
	"errors"
	"testing"

	"github.com/ifeanyiecheruo/morsel/platform"
	"github.com/ifeanyiecheruo/morsel/platform/local"
)

var ctx = context.Background()

func TestAmbientTokenReturnsEmpty(t *testing.T) {
	plat := local.New()
	token, err := plat.Credentials().AmbientToken(ctx)
	if err != nil {
		t.Fatalf("AmbientToken: unexpected error: %v", err)
	}
	if token != "" {
		t.Errorf("token = %q, want empty string", token)
	}
}

func TestValidateDeployTokenRejectsInvalidToken(t *testing.T) {
	plat := platWithTempHome(t)
	_, err := plat.Credentials().ValidateDeployToken(ctx, "not-a-valid-token")
	if err == nil {
		t.Error("ValidateDeployToken: expected error for invalid token, got nil")
	}
}

func TestDeployTokenRoundTrip(t *testing.T) {
	plat := platWithTempHome(t)

	// No manual key seeding needed — the manager generates on first use.
	token, err := plat.Credentials().DeployToken(ctx)
	if err != nil {
		t.Fatalf("DeployToken: %v", err)
	}
	slug, err := plat.Credentials().ValidateDeployToken(ctx, token)
	if err != nil {
		t.Fatalf("ValidateDeployToken: %v", err)
	}
	if slug == "" {
		t.Error("ValidateDeployToken: returned empty slug")
	}
}

// platWithTempHome creates a LocalPlatform whose secrets store points at a
// temporary directory so tests never touch ~/.morsel/local/secrets.json.
func platWithTempHome(t *testing.T) *local.LocalPlatform {
	t.Helper()
	tmp := t.TempDir()
	t.Setenv("USERPROFILE", tmp) // Windows
	t.Setenv("HOME", tmp)        // Linux / macOS
	return local.New()
}

func TestDNSCreateRecordIsNoop(t *testing.T) {
	plat := local.New()
	if err := plat.DNS().CreateRecord(ctx, "zone", "name", "A", "1.2.3.4", 60); err != nil {
		t.Errorf("CreateRecord: unexpected error: %v", err)
	}
}

func TestDNSDeleteRecordIsNoop(t *testing.T) {
	plat := local.New()
	if err := plat.DNS().DeleteRecord(ctx, "zone", "name", "A"); err != nil {
		t.Errorf("DeleteRecord: unexpected error: %v", err)
	}
}

func TestDNSRecordExistsReturnsFalse(t *testing.T) {
	plat := local.New()
	exists, err := plat.DNS().RecordExists(ctx, "zone", "name", "A")
	if err != nil {
		t.Fatalf("RecordExists: unexpected error: %v", err)
	}
	if exists {
		t.Error("RecordExists = true, want false")
	}
}

func TestPricesFetchedAtIsSet(t *testing.T) {
	plat := local.New()
	prices, err := plat.Pricing().Prices(ctx)
	if err != nil {
		t.Fatalf("Prices: unexpected error: %v", err)
	}
	if prices.FetchedAt.IsZero() {
		t.Error("Prices.FetchedAt is zero, want non-zero")
	}
}

func TestBootstrapProvisionNotImplemented(t *testing.T) {
	plat := local.New()
	if err := plat.Bootstrap().Provision(ctx, nil); !errors.Is(err, platform.ErrNotImplemented) {
		t.Errorf("Bootstrap.Provision: err = %v, want ErrNotImplemented", err)
	}
}

func TestDeployCredentialsNotImplemented(t *testing.T) {
	plat := local.New()
	if _, err := plat.Deploy().Credentials(ctx); !errors.Is(err, platform.ErrNotImplemented) {
		t.Errorf("Deploy.Credentials: err = %v, want ErrNotImplemented", err)
	}
}

func TestBlobsGetNotImplemented(t *testing.T) {
	plat := local.New()
	if _, err := plat.Blobs().Get(ctx, "bucket", "key"); !errors.Is(err, platform.ErrNotImplemented) {
		t.Errorf("Blobs.Get: err = %v, want ErrNotImplemented", err)
	}
}
