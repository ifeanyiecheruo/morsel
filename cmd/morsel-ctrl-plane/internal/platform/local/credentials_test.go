package local_test

import (
	"errors"
	"testing"

	"golang.org/x/crypto/bcrypt"

	"github.com/ifeanyiecheruo/morsel/cmd/morsel-ctrl-plane/internal/platform"
	"github.com/ifeanyiecheruo/morsel/cmd/morsel-ctrl-plane/internal/store"
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

func TestValidateOperatorCredentialChecksPasswordWhenHashStored(t *testing.T) {
	plat, s := platWithStore(t)

	const correctPassword = "hunter2"
	if err := s.AddPrincipalWithPasswordHash(ctx, "alice@example.com", bcryptHash(t, correctPassword)); err != nil {
		t.Fatalf("AddPrincipalWithPasswordHash: %v", err)
	}

	if _, err := plat.Tokens().ValidateOperatorCredential(ctx, "alice@example.com", correctPassword); err != nil {
		t.Errorf("ValidateOperatorCredential with correct password: %v", err)
	}
	if _, err := plat.Tokens().ValidateOperatorCredential(ctx, "alice@example.com", "wrongpassword"); !isPrincipalNotAuthorized(err) {
		t.Errorf("ValidateOperatorCredential with wrong password: want ErrPrincipalNotAuthorized, got %v", err)
	}
}

func bcryptHash(t *testing.T, password string) string {
	t.Helper()
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.MinCost)
	if err != nil {
		t.Fatalf("bcrypt.GenerateFromPassword: %v", err)
	}
	return string(hash)
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
