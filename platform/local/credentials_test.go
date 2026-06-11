package local_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ifeanyiecheruo/morsel/platform"
	"github.com/ifeanyiecheruo/morsel/platform/local"
)

func TestValidateOperatorTokenAcceptsKnownPrincipal(t *testing.T) {
	plat := platWithTempHome(t)
	seedPrincipals(t, plat, "alice@example.com")

	req := oidcRequest(`{"email":"alice@example.com"}`)
	subject, err := plat.Credentials().ValidateOperatorToken(context.Background(), req)
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

	req := oidcRequest(`{"email":"eve@example.com"}`)
	_, err := plat.Credentials().ValidateOperatorToken(context.Background(), req)
	if !isPrincipalNotAuthorized(err) {
		t.Errorf("err = %v, want ErrPrincipalNotAuthorized", err)
	}
}

func TestValidateOperatorTokenRejectsEmptyPrincipalsList(t *testing.T) {
	plat := platWithTempHome(t)

	req := oidcRequest(`{"email":"alice@example.com"}`)
	_, err := plat.Credentials().ValidateOperatorToken(context.Background(), req)
	if !isPrincipalNotAuthorized(err) {
		t.Errorf("err = %v, want ErrPrincipalNotAuthorized", err)
	}
}

func TestValidateOperatorTokenRejectsMalformedBody(t *testing.T) {
	plat := platWithTempHome(t)
	seedPrincipals(t, plat, "alice@example.com")

	req := oidcRequest(`not json`)
	_, err := plat.Credentials().ValidateOperatorToken(context.Background(), req)
	if !isPrincipalNotAuthorized(err) {
		t.Errorf("err = %v, want ErrPrincipalNotAuthorized", err)
	}
}

func TestValidateOperatorTokenRejectsMissingEmail(t *testing.T) {
	plat := platWithTempHome(t)
	seedPrincipals(t, plat, "alice@example.com")

	req := oidcRequest(`{}`)
	_, err := plat.Credentials().ValidateOperatorToken(context.Background(), req)
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
	if err := plat.Secrets().Set(context.Background(), "operator-principals", raw); err != nil {
		t.Fatalf("seed principals: %v", err)
	}
}

func oidcRequest(body string) *http.Request {
	return httptest.NewRequest(http.MethodPost, "/api/token/oidc", strings.NewReader(body))
}

func isPrincipalNotAuthorized(err error) bool {
	return errors.Is(err, platform.ErrPrincipalNotAuthorized)
}
