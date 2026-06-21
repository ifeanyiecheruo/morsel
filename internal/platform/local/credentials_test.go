package local_test

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/ifeanyiecheruo/morsel/internal/platform"
	"github.com/ifeanyiecheruo/morsel/internal/platform/local"
)

func TestValidateOperatorTokenAcceptsKnownPrincipal(t *testing.T) {
	plat := platWithTempHome(t)
	seedPrincipals(t, plat, "alice@example.com")

	subject, err := plat.Credentials().ValidateOperatorToken(ctx, "alice@example.com", "")
	if err != nil {
		t.Fatalf("ValidateOperatorToken: unexpected error: %v", err)
	}
	if subject != "alice@example.com" {
		t.Errorf("subject = %q, want alice@example.com", subject)
	}
}

func TestValidateOperatorTokenRejectsUnknownPrincipal(t *testing.T) {
	plat := platWithTempHome(t)
	seedPrincipals(t, plat, "alice@example.com")

	_, err := plat.Credentials().ValidateOperatorToken(ctx, "eve@example.com", "")
	if !isPrincipalNotAuthorized(err) {
		t.Errorf("err = %v, want ErrPrincipalNotAuthorized", err)
	}
}

func TestValidateOperatorTokenRejectsEmptyPrincipalsList(t *testing.T) {
	plat := platWithTempHome(t)

	_, err := plat.Credentials().ValidateOperatorToken(ctx, "alice@example.com", "")
	if !isPrincipalNotAuthorized(err) {
		t.Errorf("err = %v, want ErrPrincipalNotAuthorized", err)
	}
}

func TestValidateOperatorTokenRejectsEmptyUsername(t *testing.T) {
	plat := platWithTempHome(t)
	seedPrincipals(t, plat, "alice@example.com")

	_, err := plat.Credentials().ValidateOperatorToken(ctx, "", "")
	if !isPrincipalNotAuthorized(err) {
		t.Errorf("err = %v, want ErrPrincipalNotAuthorized", err)
	}
}

func seedPrincipals(t *testing.T, plat *local.LocalPlatform, emails ...string) {
	t.Helper()
	raw, err := json.Marshal(emails)
	if err != nil {
		t.Fatalf("marshal principals: %v", err)
	}
	if err := plat.Secrets().Set(ctx, "operator-principals", raw); err != nil {
		t.Fatalf("seed principals: %v", err)
	}
}

func isPrincipalNotAuthorized(err error) bool {
	return errors.Is(err, platform.ErrPrincipalNotAuthorized)
}
