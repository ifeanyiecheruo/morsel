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

func TestDeployTokenNotImplemented(t *testing.T) {
	plat := local.New()
	_, err := plat.Credentials().DeployToken(ctx)
	if !errors.Is(err, platform.ErrNotImplemented) {
		t.Errorf("DeployToken: err = %v, want ErrNotImplemented", err)
	}
}

func TestValidateDeployTokenNotImplemented(t *testing.T) {
	plat := local.New()
	_, err := plat.Credentials().ValidateDeployToken(ctx, "some-token")
	if !errors.Is(err, platform.ErrNotImplemented) {
		t.Errorf("ValidateDeployToken: err = %v, want ErrNotImplemented", err)
	}
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
