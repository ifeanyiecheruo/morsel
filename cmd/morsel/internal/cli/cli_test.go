package cli

import (
	"context"
	"errors"
	"testing"

	"github.com/ifeanyiecheruo/morsel/cmd/morsel/internal/platform"
)

// --- Test doubles ---

// mockCliHandler implements Handler using optional function hooks.
// Hooks that are nil return zero values with no error (except OperatorLogin,
// which returns a safe empty profile). Tests set only the hooks they care about.
type mockCliHandler struct {
	onLoadProfile             func(name string, ensureValid bool) (*Profile, error)
	onServiceDeploy           func(kubeconfig string, b platform.ServiceDeployer, dockerfile []byte, yes bool) (*Profile, error)
	onServiceDeployPlatform   func(prof *Profile) (string, error)
	onOperatorLogin           func(apiURL string) (*Profile, error)
	onCompleteBootstrap       func(apiURL, bootstrapToken, idToken string) (*Profile, error)
	onSaveProfile             func(name string, prof *Profile) error
	onDeleteProfile           func(name string) error
	onLint                    func(staged, fix bool) error
	onServiceStatus           func(prof *Profile) error
	onServiceDelete           func(prof *Profile, kubecontext, namespace string) error
	onServiceUpgradeRetry     func(prof *Profile) error
	onOperatorLogout          func(prof *Profile) error
	onOperatorPrincipalAdd    func(prof *Profile, login string) error
	onOperatorPrincipalRemove func(prof *Profile, login string) error
	onOperatorPrincipalList   func(prof *Profile) error
	onOperatorPrincipalPatch  func(prof *Profile, login string, isOperator, isAdmin *bool) error
	onTierList                func(prof *Profile) error
	onTierCreate              func(prof *Profile, flags TierFlags) error
	onTierEdit                func(prof *Profile, flags TierFlags) error
	onTierSetDefault          func(prof *Profile, name string) error
	onTierDelete              func(prof *Profile, name string) error
	onAppExemptAdd            func(prof *Profile, repo, app string) error
	onAppExemptRemove         func(prof *Profile, repo, app string) error
	onRepoExemptAdd           func(prof *Profile, repo string) error
	onRepoExemptRemove        func(prof *Profile, repo string) error
	onAppDeploy               func(prof *Profile) error
	onAppList                 func(prof *Profile, org, repo string) error
	onAppGet                  func(prof *Profile, org, repo, name string) error
	onAppStatus               func(prof *Profile, org, repo, name string) error
	onAppDelete               func(prof *Profile, org, repo, name string) error
	onAppHistory              func(prof *Profile, org, repo, name string) error
	onAppSync                 func(prof *Profile, org, repo, name string) error
}

func (h *mockCliHandler) LoadProfile(_ context.Context, name string, ensureValid bool) (*Profile, error) {
	if h.onLoadProfile != nil {
		return h.onLoadProfile(name, ensureValid)
	}
	return nil, nil
}

func (h *mockCliHandler) ServiceDeploy(_ context.Context, kubeconfig string, b platform.ServiceDeployer, dockerfile []byte, yes bool) (*Profile, error) {
	if h.onServiceDeploy != nil {
		return h.onServiceDeploy(kubeconfig, b, dockerfile, yes)
	}
	return &Profile{}, nil
}

func (h *mockCliHandler) ServiceDeployPlatform(_ context.Context, prof *Profile) (string, error) {
	if h.onServiceDeployPlatform != nil {
		return h.onServiceDeployPlatform(prof)
	}
	return "local", nil
}

func (h *mockCliHandler) OperatorLogin(_ context.Context, apiURL string) (*Profile, error) {
	if h.onOperatorLogin != nil {
		return h.onOperatorLogin(apiURL)
	}
	return &Profile{}, nil
}

func (h *mockCliHandler) CompleteBootstrap(_ context.Context, apiURL, bootstrapToken, idToken string) (*Profile, error) {
	if h.onCompleteBootstrap != nil {
		return h.onCompleteBootstrap(apiURL, bootstrapToken, idToken)
	}
	return &Profile{}, nil
}

func (h *mockCliHandler) SaveProfile(_ context.Context, name string, prof *Profile) error {
	if h.onSaveProfile != nil {
		return h.onSaveProfile(name, prof)
	}
	return nil
}

func (h *mockCliHandler) DeleteProfile(_ context.Context, name string) error {
	if h.onDeleteProfile != nil {
		return h.onDeleteProfile(name)
	}
	return nil
}

func (h *mockCliHandler) Lint(_ context.Context, staged, fix bool) error {
	if h.onLint != nil {
		return h.onLint(staged, fix)
	}
	return nil
}

func (h *mockCliHandler) ServiceStatus(_ context.Context, prof *Profile) error {
	if h.onServiceStatus != nil {
		return h.onServiceStatus(prof)
	}
	return nil
}

func (h *mockCliHandler) ServiceDelete(_ context.Context, prof *Profile, kubecontext, namespace string) error {
	if h.onServiceDelete != nil {
		return h.onServiceDelete(prof, kubecontext, namespace)
	}
	return nil
}

func (h *mockCliHandler) ServiceUpgradeRetry(_ context.Context, prof *Profile) error {
	if h.onServiceUpgradeRetry != nil {
		return h.onServiceUpgradeRetry(prof)
	}
	return nil
}

func (h *mockCliHandler) OperatorLogout(_ context.Context, prof *Profile) error {
	if h.onOperatorLogout != nil {
		return h.onOperatorLogout(prof)
	}
	return nil
}

func (h *mockCliHandler) OperatorPrincipalAdd(_ context.Context, prof *Profile, principal string) error {
	if h.onOperatorPrincipalAdd != nil {
		return h.onOperatorPrincipalAdd(prof, principal)
	}
	return nil
}

func (h *mockCliHandler) OperatorPrincipalRemove(_ context.Context, prof *Profile, principal string) error {
	if h.onOperatorPrincipalRemove != nil {
		return h.onOperatorPrincipalRemove(prof, principal)
	}
	return nil
}

func (h *mockCliHandler) OperatorPrincipalList(_ context.Context, prof *Profile) error {
	if h.onOperatorPrincipalList != nil {
		return h.onOperatorPrincipalList(prof)
	}
	return nil
}

func (h *mockCliHandler) OperatorPrincipalPatch(_ context.Context, prof *Profile, login string, isOperator, isAdmin *bool) error {
	if h.onOperatorPrincipalPatch != nil {
		return h.onOperatorPrincipalPatch(prof, login, isOperator, isAdmin)
	}
	return nil
}

func (h *mockCliHandler) TierList(_ context.Context, prof *Profile) error {
	if h.onTierList != nil {
		return h.onTierList(prof)
	}
	return nil
}

func (h *mockCliHandler) TierCreate(_ context.Context, prof *Profile, flags TierFlags) error {
	if h.onTierCreate != nil {
		return h.onTierCreate(prof, flags)
	}
	return nil
}

func (h *mockCliHandler) TierEdit(_ context.Context, prof *Profile, flags TierFlags) error {
	if h.onTierEdit != nil {
		return h.onTierEdit(prof, flags)
	}
	return nil
}

func (h *mockCliHandler) TierSetDefault(_ context.Context, prof *Profile, name string) error {
	if h.onTierSetDefault != nil {
		return h.onTierSetDefault(prof, name)
	}
	return nil
}

func (h *mockCliHandler) TierDelete(_ context.Context, prof *Profile, name string) error {
	if h.onTierDelete != nil {
		return h.onTierDelete(prof, name)
	}
	return nil
}

func (h *mockCliHandler) AppExemptAdd(_ context.Context, prof *Profile, repo, app string) error {
	if h.onAppExemptAdd != nil {
		return h.onAppExemptAdd(prof, repo, app)
	}
	return nil
}

func (h *mockCliHandler) AppExemptRemove(_ context.Context, prof *Profile, repo, app string) error {
	if h.onAppExemptRemove != nil {
		return h.onAppExemptRemove(prof, repo, app)
	}
	return nil
}

func (h *mockCliHandler) RepoExemptAdd(_ context.Context, prof *Profile, repo string) error {
	if h.onRepoExemptAdd != nil {
		return h.onRepoExemptAdd(prof, repo)
	}
	return nil
}

func (h *mockCliHandler) RepoExemptRemove(_ context.Context, prof *Profile, repo string) error {
	if h.onRepoExemptRemove != nil {
		return h.onRepoExemptRemove(prof, repo)
	}
	return nil
}

func (h *mockCliHandler) AppDeploy(_ context.Context, prof *Profile) error {
	if h.onAppDeploy != nil {
		return h.onAppDeploy(prof)
	}
	return nil
}

func (h *mockCliHandler) AppList(_ context.Context, prof *Profile, org, repo string) error {
	if h.onAppList != nil {
		return h.onAppList(prof, org, repo)
	}
	return nil
}

func (h *mockCliHandler) AppGet(_ context.Context, prof *Profile, org, repo, name string) error {
	if h.onAppGet != nil {
		return h.onAppGet(prof, org, repo, name)
	}
	return nil
}

func (h *mockCliHandler) AppStatus(_ context.Context, prof *Profile, org, repo, name string) error {
	if h.onAppStatus != nil {
		return h.onAppStatus(prof, org, repo, name)
	}
	return nil
}

func (h *mockCliHandler) AppDelete(_ context.Context, prof *Profile, org, repo, name string) error {
	if h.onAppDelete != nil {
		return h.onAppDelete(prof, org, repo, name)
	}
	return nil
}

func (h *mockCliHandler) AppHistory(_ context.Context, prof *Profile, org, repo, name string) error {
	if h.onAppHistory != nil {
		return h.onAppHistory(prof, org, repo, name)
	}
	return nil
}

func (h *mockCliHandler) AppSync(_ context.Context, prof *Profile, org, repo, name string) error {
	if h.onAppSync != nil {
		return h.onAppSync(prof, org, repo, name)
	}
	return nil
}

// fakeProfile is a pre-built profile for injecting into auth-required command tests.
var fakeProfile = &Profile{APIURL: "http://localhost:8080"}

// withProfile returns a LoadProfile hook that always returns fakeProfile.
func withProfile(prof *Profile) func(string, bool) (*Profile, error) {
	return func(_ string, _ bool) (*Profile, error) { return prof, nil }
}

// --- Operator login ---

func TestOperatorLoginUsesProfileAPIURL(t *testing.T) {
	var gotURL string
	mock := &mockCliHandler{
		onLoadProfile: withProfile(fakeProfile),
		onOperatorLogin: func(apiURL string) (*Profile, error) {
			gotURL = apiURL
			return &Profile{}, nil
		},
	}
	if err := run(context.Background(), mock, []string{"operator", "login"}, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotURL != fakeProfile.APIURL {
		t.Errorf("api-url: want %q, got %q", fakeProfile.APIURL, gotURL)
	}
}

func TestOperatorLoginUsesAPIURLFlag(t *testing.T) {
	var gotURL string
	const want = "http://localhost:9090"
	mock := &mockCliHandler{
		onLoadProfile: withProfile(&Profile{APIURL: "http://localhost:8080"}),
		onOperatorLogin: func(apiURL string) (*Profile, error) {
			gotURL = apiURL
			return &Profile{}, nil
		},
	}
	if err := run(context.Background(), mock, []string{"operator", "login", "--api-url", want}, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotURL != want {
		t.Errorf("api-url: want %q, got %q", want, gotURL)
	}
}

func TestOperatorLoginFailsWithoutAPIURLWhenNoProfile(t *testing.T) {
	// onLoadProfile is nil → LoadProfile returns (nil, nil) → c.profile is nil, no --api-url → error.
	mock := &mockCliHandler{}
	err := run(context.Background(), mock, []string{"operator", "login"}, nil)
	if err == nil {
		t.Fatal("expected error when no profile and no --api-url, got nil")
	}
}

func TestOperatorLoginFailsWithEmptyAPIURLInProfile(t *testing.T) {
	mock := &mockCliHandler{
		onLoadProfile: withProfile(&Profile{APIURL: ""}),
	}
	err := run(context.Background(), mock, []string{"operator", "login"}, nil)
	if err == nil {
		t.Fatal("expected error when profile has empty APIURL, got nil")
	}
}

func TestOperatorLoginSucceedsWithAPIURLAndNoProfile(t *testing.T) {
	var gotURL string
	const want = "http://remote:8080"
	mock := &mockCliHandler{
		onOperatorLogin: func(apiURL string) (*Profile, error) {
			gotURL = apiURL
			return &Profile{}, nil
		},
	}
	if err := run(context.Background(), mock, []string{"operator", "login", "--api-url", want}, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotURL != want {
		t.Errorf("api-url: want %q, got %q", want, gotURL)
	}
}

// --- Service deploy ---

func TestServiceDeployPassesFlagsToHandler(t *testing.T) {
	var gotKubeconfig string
	mock := &mockCliHandler{
		onServiceDeploy: func(kubeconfig string, _ platform.ServiceDeployer, _ []byte, _ bool) (*Profile, error) {
			gotKubeconfig = kubeconfig
			return &Profile{}, nil
		},
	}
	if err := run(context.Background(), mock, []string{"service", "deploy", "--platform", "local", "--kubeconfig", "/tmp/kube"}, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotKubeconfig != "/tmp/kube" {
		t.Errorf("kubeconfig: want %q, got %q", "/tmp/kube", gotKubeconfig)
	}
}

func TestServiceDeployYesFlagPassedToHandler(t *testing.T) {
	var gotYes bool
	mock := &mockCliHandler{
		onServiceDeploy: func(_ string, _ platform.ServiceDeployer, _ []byte, yes bool) (*Profile, error) {
			gotYes = yes
			return &Profile{}, nil
		},
	}
	if err := run(context.Background(), mock, []string{"service", "deploy", "--platform", "local", "-y"}, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !gotYes {
		t.Error("yes: want true, got false")
	}
}

func TestServiceDeployRequiresPlatformFlagWhenNotLoggedIn(t *testing.T) {
	// onLoadProfile is nil → no profile → not logged in → --platform required
	err := run(context.Background(), &mockCliHandler{}, []string{"service", "deploy"}, nil)
	if err == nil {
		t.Fatal("expected error for missing --platform when not logged in, got nil")
	}
}

func TestServiceDeployRejectsMismatchedPlatformWhenLoggedIn(t *testing.T) {
	mock := &mockCliHandler{
		onLoadProfile:           withProfile(&Profile{APIURL: "http://localhost:8080"}),
		onServiceDeployPlatform: func(_ *Profile) (string, error) { return "local", nil },
		onServiceDeploy: func(_ string, _ platform.ServiceDeployer, _ []byte, _ bool) (*Profile, error) {
			return &Profile{}, nil
		},
	}
	err := run(context.Background(), mock, []string{"service", "deploy", "--platform", "gcp"}, nil)
	if err == nil {
		t.Fatal("expected error when --platform disagrees with instance platform, got nil")
	}
}

func TestServiceDeployAllowsMatchingPlatformWhenLoggedIn(t *testing.T) {
	mock := &mockCliHandler{
		onLoadProfile:           withProfile(&Profile{APIURL: "http://localhost:8080"}),
		onServiceDeployPlatform: func(_ *Profile) (string, error) { return "local", nil },
		onServiceDeploy: func(_ string, _ platform.ServiceDeployer, _ []byte, _ bool) (*Profile, error) {
			return &Profile{}, nil
		},
	}
	if err := run(context.Background(), mock, []string{"service", "deploy", "--platform", "local"}, nil); err != nil {
		t.Fatalf("expected no error when --platform matches instance platform, got: %v", err)
	}
}

func TestServiceDeployRejectsUnknownPlatform(t *testing.T) {
	err := run(context.Background(), &mockCliHandler{}, []string{"service", "deploy", "--platform", "gcp"}, nil)
	if err == nil {
		t.Fatal("expected error for unknown platform, got nil")
	}
}

// --- Auth gating ---

func TestAuthRequiredCommandRejectsWithoutProfile(t *testing.T) {
	// service status requires auth; without a profile it should fail before calling the handler.
	called := false
	mock := &mockCliHandler{
		onServiceStatus: func(_ *Profile) error {
			called = true
			return nil
		},
		// onLoadProfile is nil → LoadProfile returns (nil, nil) → no profile set
	}
	err := run(context.Background(), mock, []string{"service", "status"}, nil)
	if err == nil {
		t.Fatal("expected auth error, got nil")
	}
	if called {
		t.Error("handler should not be called when no profile is present")
	}
}

func TestAuthRequiredCommandCallsHandlerWithProfile(t *testing.T) {
	var gotProf *Profile
	mock := &mockCliHandler{
		onLoadProfile: withProfile(fakeProfile),
		onServiceStatus: func(prof *Profile) error {
			gotProf = prof
			return nil
		},
	}
	if err := run(context.Background(), mock, []string{"service", "status"}, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotProf != fakeProfile {
		t.Error("handler received wrong profile")
	}
}

// --- Service delete --confirm guard ---

func TestServiceDeleteRequiresConfirmFlag(t *testing.T) {
	called := false
	mock := &mockCliHandler{
		onLoadProfile: withProfile(fakeProfile),
		onServiceDelete: func(_ *Profile, _, _ string) error {
			called = true
			return nil
		},
	}
	err := run(context.Background(), mock, []string{"service", "delete"}, nil)
	if err == nil {
		t.Fatal("expected error without --confirm, got nil")
	}
	if called {
		t.Error("handler should not be called without --confirm")
	}
}

func TestServiceDeleteCallsHandlerWithConfirm(t *testing.T) {
	called := false
	mock := &mockCliHandler{
		onLoadProfile: withProfile(fakeProfile),
		onServiceDelete: func(_ *Profile, _, _ string) error {
			called = true
			return nil
		},
	}
	if err := run(context.Background(), mock, []string{"service", "delete", "--confirm"}, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !called {
		t.Error("handler was not called with --confirm")
	}
}

// --- Lint flag routing ---

func TestLintPassesStagedFlag(t *testing.T) {
	var gotStaged, gotFix bool
	mock := &mockCliHandler{
		onLint: func(staged, fix bool) error {
			gotStaged = staged
			gotFix = fix
			return nil
		},
	}
	if err := run(context.Background(), mock, []string{"lint", "--staged"}, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !gotStaged {
		t.Error("staged: want true, got false")
	}
	if gotFix {
		t.Error("fix: want false, got true")
	}
}

func TestLintPassesFixFlag(t *testing.T) {
	var gotStaged, gotFix bool
	mock := &mockCliHandler{
		onLint: func(staged, fix bool) error {
			gotStaged = staged
			gotFix = fix
			return nil
		},
	}
	if err := run(context.Background(), mock, []string{"lint", "--fix"}, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotStaged {
		t.Error("staged: want false, got true")
	}
	if !gotFix {
		t.Error("fix: want true, got false")
	}
}

// --- Tier create flag routing ---

func TestTierCreatePassesFlags(t *testing.T) {
	var gotFlags TierFlags
	mock := &mockCliHandler{
		onLoadProfile: withProfile(fakeProfile),
		onTierCreate: func(_ *Profile, flags TierFlags) error {
			gotFlags = flags
			return nil
		},
	}
	err := run(context.Background(), mock,
		[]string{"operator", "tier", "create", "--name", "small", "--cpu", "0.5", "--max-apps", "3"},
		nil,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotFlags.Name != "small" {
		t.Errorf("name: want %q, got %q", "small", gotFlags.Name)
	}
	if gotFlags.CPU != 0.5 {
		t.Errorf("cpu: want 0.5, got %v", gotFlags.CPU)
	}
	if gotFlags.MaxApps != 3 {
		t.Errorf("max-apps: want 3, got %d", gotFlags.MaxApps)
	}
}

// --- Deep nesting: operator app exempt add ---

func TestAppExemptAddPassesFlags(t *testing.T) {
	var gotRepo, gotApp string
	mock := &mockCliHandler{
		onLoadProfile: withProfile(fakeProfile),
		onAppExemptAdd: func(_ *Profile, repo, app string) error {
			gotRepo = repo
			gotApp = app
			return nil
		},
	}
	err := run(context.Background(), mock,
		[]string{"operator", "app", "exempt", "add", "--repo", "org/myrepo", "--app", "api"},
		nil,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotRepo != "org/myrepo" {
		t.Errorf("repo: want %q, got %q", "org/myrepo", gotRepo)
	}
	if gotApp != "api" {
		t.Errorf("app: want %q, got %q", "api", gotApp)
	}
}

// --- Handler error propagation ---

func TestHandlerErrorPropagates(t *testing.T) {
	sentinel := errors.New("handler sentinel")
	mock := &mockCliHandler{
		onLint: func(_, _ bool) error { return sentinel },
	}
	err := run(context.Background(), mock, []string{"lint"}, nil)
	if !errors.Is(err, sentinel) {
		t.Errorf("expected sentinel error, got: %v", err)
	}
}
