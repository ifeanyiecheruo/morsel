package local_test

import (
	"errors"
	"testing"

	"github.com/ifeanyiecheruo/morsel/internal/platform"
	"github.com/ifeanyiecheruo/morsel/internal/store"
)

func TestValidateOperatorCredentialAcceptsKnownPrincipal(t *testing.T) {
	plat, s := platWithStore(t)
	seedPrincipals(t, s, "alice@example.com")

	subject, err := plat.Tokens().ValidateOperatorCredential(ctx, "alice@example.com", "")
	if err != nil {
		t.Fatalf("ValidateOperatorCredential: unexpected error: %v", err)
	}
	if subject != "alice@example.com" {
		t.Errorf("subject = %q, want alice@example.com", subject)
	}
}

func TestValidateOperatorCredentialRejectsUnknownPrincipal(t *testing.T) {
	plat, s := platWithStore(t)
	seedPrincipals(t, s, "alice@example.com")

	_, err := plat.Tokens().ValidateOperatorCredential(ctx, "eve@example.com", "")
	if !isPrincipalNotAuthorized(err) {
		t.Errorf("err = %v, want ErrPrincipalNotAuthorized", err)
	}
}

func TestValidateOperatorCredentialRejectsEmptyPrincipalsList(t *testing.T) {
	plat, _ := platWithStore(t)

	_, err := plat.Tokens().ValidateOperatorCredential(ctx, "alice@example.com", "")
	if !isPrincipalNotAuthorized(err) {
		t.Errorf("err = %v, want ErrPrincipalNotAuthorized", err)
	}
}

func TestValidateOperatorCredentialRejectsEmptyUsername(t *testing.T) {
	plat, s := platWithStore(t)
	seedPrincipals(t, s, "alice@example.com")

	_, err := plat.Tokens().ValidateOperatorCredential(ctx, "", "")
	if !isPrincipalNotAuthorized(err) {
		t.Errorf("err = %v, want ErrPrincipalNotAuthorized", err)
	}
}

func seedPrincipals(t *testing.T, s *store.Store, emails ...string) {
	t.Helper()
	for _, email := range emails {
		if err := s.AddPrincipal(ctx, email); err != nil {
			t.Fatalf("seed principal %q: %v", email, err)
		}
	}
}

func isPrincipalNotAuthorized(err error) bool {
	return errors.Is(err, platform.ErrPrincipalNotAuthorized)
}
