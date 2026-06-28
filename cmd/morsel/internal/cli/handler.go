package cli

import (
	"context"
	"fmt"
	"time"

	"github.com/ifeanyiecheruo/morsel/cmd/morsel/internal/client"
	"github.com/ifeanyiecheruo/morsel/cmd/morsel/internal/client/oas"
	"github.com/ifeanyiecheruo/morsel/internal/platform"
)

// Handler defines the business logic behind each CLI command.
// The cli layer owns Cobra routing, flag parsing, and authentication gating;
// it then delegates to Handler for the actual work. Tests inject a mock.
type Handler interface {
	// LoadProfile reads the named profile. If ensureValid is true the handler
	// must also verify that stored credentials have not expired.
	LoadProfile(ctx context.Context, name string, ensureValid bool) (*Profile, error)

	// No-auth commands
	ServiceBootstrap(ctx context.Context, kubeconfig string, b platform.Bootstrapper, dockerfile []byte, yes bool) (*Profile, error)

	OperatorLogin(ctx context.Context, apiURL, username, password string) (*Profile, error)
	SaveProfile(ctx context.Context, name string, prof *Profile) error
	DeleteProfile(ctx context.Context, name string) error
	Lint(ctx context.Context, staged, fix bool) error

	// Auth-required commands — prof is pre-validated by the cli layer
	ServiceStatus(ctx context.Context, prof *Profile) error
	ServiceDelete(ctx context.Context, prof *Profile, kubecontext, namespace string) error
	ServiceUpgradeRetry(ctx context.Context, prof *Profile) error
	OperatorLogout(ctx context.Context, prof *Profile) error
	OperatorPrincipalAdd(ctx context.Context, prof *Profile, principal string) error
	OperatorPrincipalRemove(ctx context.Context, prof *Profile, principal string) error
	OperatorPrincipalList(ctx context.Context, prof *Profile) error
	TierList(ctx context.Context, prof *Profile) error
	TierCreate(ctx context.Context, prof *Profile, flags TierFlags) error
	TierEdit(ctx context.Context, prof *Profile, flags TierFlags) error
	TierSetDefault(ctx context.Context, prof *Profile, name string) error
	TierDelete(ctx context.Context, prof *Profile, name string) error
	AppExemptAdd(ctx context.Context, prof *Profile, repo, app string) error
	AppExemptRemove(ctx context.Context, prof *Profile, repo, app string) error
	RepoExemptAdd(ctx context.Context, prof *Profile, repo string) error
	RepoExemptRemove(ctx context.Context, prof *Profile, repo string) error
	AppDeploy(ctx context.Context, prof *Profile) error
	AppList(ctx context.Context, prof *Profile, org, repo string) error
	AppGet(ctx context.Context, prof *Profile, org, repo, name string) error
	AppStatus(ctx context.Context, prof *Profile, org, repo, name string) error
	AppDelete(ctx context.Context, prof *Profile, org, repo, name string) error
	AppHistory(ctx context.Context, prof *Profile, org, repo, name string) error
	AppSync(ctx context.Context, prof *Profile, org, repo, name string) error
}

type cliHandler struct{}

func (h *cliHandler) LoadProfile(ctx context.Context, name string, ensureValid bool) (*Profile, error) {
	prof, err := readProfile(name)
	if err != nil {
		return nil, err
	}
	if !ensureValid || prof.AccessTokenExpiresAt == "" {
		return prof, nil
	}
	expiry, err := time.Parse(time.RFC3339, prof.AccessTokenExpiresAt)
	if err != nil || time.Now().Before(expiry) {
		return prof, nil
	}
	return silentRefresh(ctx, prof, name)
}

func (h *cliHandler) SaveProfile(_ context.Context, name string, prof *Profile) error {
	return writeProfile(name, prof)
}

func (h *cliHandler) DeleteProfile(_ context.Context, name string) error {
	return deleteProfile(name)
}

// silentRefresh exchanges the stored refresh token for a new token pair and
// persists the updated profile. Called when the access token has expired.
func silentRefresh(ctx context.Context, prof *Profile, name string) (*Profile, error) {
	if prof.RefreshToken == "" {
		return nil, fmt.Errorf("session expired — run 'morsel operator login' to re-authenticate")
	}
	client, err := client.New(prof.APIURL, "")
	if err != nil {
		return nil, fmt.Errorf("build api client for refresh: %w", err)
	}
	resp, err := client.Inner().TokenRefresh(ctx, &oas.TokenRefreshReq{RefreshToken: prof.RefreshToken})
	if err != nil {
		return nil, fmt.Errorf("refresh token: %w", err)
	}
	pair, ok := resp.(*oas.TokenPairResponse)
	if !ok {
		return nil, fmt.Errorf("session expired — run 'morsel operator login' to re-authenticate")
	}
	now := time.Now()
	prof.AccessToken = pair.AccessToken
	prof.AccessTokenExpiresAt = now.Add(time.Duration(pair.ExpiresIn) * time.Second).UTC().Format(time.RFC3339)
	prof.RefreshToken = pair.RefreshToken
	prof.RefreshTokenExpiresAt = now.Add(time.Duration(pair.RefreshExpiresIn) * time.Second).UTC().Format(time.RFC3339)
	if err := writeProfile(name, prof); err != nil {
		return nil, fmt.Errorf("persist refreshed profile: %w", err)
	}
	return prof, nil
}

// clientFor constructs an authenticated API client from the profile's stored credentials.
func (h *cliHandler) clientFor(prof *Profile) (*client.Client, error) {
	c, err := client.New(prof.APIURL, prof.AccessToken)
	if err != nil {
		return nil, fmt.Errorf("build api client: %w", err)
	}
	return c, nil
}
