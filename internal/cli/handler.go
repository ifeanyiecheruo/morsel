package cli

import (
	"fmt"

	"github.com/ifeanyiecheruo/morsel/internal/apiclient"
)

// Handler defines the business logic behind each CLI command.
// The cli layer owns Cobra routing, flag parsing, and authentication gating;
// it then delegates to Handler for the actual work. Tests inject a mock.
type Handler interface {
	// No-auth commands
	ServiceBootstrap(platformName, kubeconfig string) error
	OperatorLogin() (*Profile, error)
	Lint(staged, fix bool) error

	// Auth-required commands — prof is pre-validated by the cli layer
	ServiceStatus(prof *Profile) error
	ServiceDelete(prof *Profile) error
	ServiceUpgradeRetry(prof *Profile) error
	OperatorLogout(prof *Profile) error
	OperatorPrincipalAdd(prof *Profile, principal string) error
	OperatorPrincipalRemove(prof *Profile, principal string) error
	OperatorPrincipalList(prof *Profile) error
	TierList(prof *Profile) error
	TierCreate(prof *Profile, flags TierFlags) error
	TierEdit(prof *Profile, flags TierFlags) error
	TierSetDefault(prof *Profile, name string) error
	TierDelete(prof *Profile, name string) error
	AppExemptAdd(prof *Profile, repo, app string) error
	AppExemptRemove(prof *Profile, repo, app string) error
	RepoExemptAdd(prof *Profile, repo string) error
	RepoExemptRemove(prof *Profile, repo string) error
	AppDeploy(prof *Profile) error
}

type cliHandler struct{}

// clientFor constructs an authenticated API client from the profile's stored credentials.
func (h *cliHandler) clientFor(prof *Profile) (*apiclient.Client, error) { //nolint:unused
	c, err := apiclient.New(prof.APIURL, prof.AccessToken)
	if err != nil {
		return nil, fmt.Errorf("build api client: %w", err)
	}
	return c, nil
}
