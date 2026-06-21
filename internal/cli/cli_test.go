package cli

import (
	"context"
	"errors"
	"testing"

	"github.com/ifeanyiecheruo/morsel/internal/platform"
)

// --- Test doubles ---

// mockCliHandler implements Handler using optional function hooks.
// Hooks that are nil return zero values with no error (except OperatorLogin,
// which returns a safe empty profile). Tests set only the hooks they care about.
type mockCliHandler struct {
	onLoadProfile             func(name string, ensureValid bool) (*Profile, error)
	onServiceBootstrap        func(platformName, kubeconfig string) (*Profile, error)
	onOperatorLogin           func(apiURL, username, password string) (*Profile, error)
	onSaveProfile             func(name string, prof *Profile) error
	onDeleteProfile           func(name string) error
	onLint                    func(staged, fix bool) error
	onServiceStatus           func(prof *Profile) error
	onServiceDelete           func(prof *Profile) error
	onServiceUpgradeRetry     func(prof *Profile) error
	onOperatorLogout          func(prof *Profile) error
	onOperatorPrincipalAdd    func(prof *Profile, principal string) error
	onOperatorPrincipalRemove func(prof *Profile, principal string) error
	onOperatorPrincipalList   func(prof *Profile) error
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
}

func (h *mockCliHandler) LoadProfile(_ context.Context, name string, ensureValid bool) (*Profile, error) {
	if h.onLoadProfile != nil {
		return h.onLoadProfile(name, ensureValid)
	}
	return nil, nil
}

func (h *mockCliHandler) ServiceBootstrap(_ context.Context, platformName, kubeconfig string) (*Profile, error) {
	if h.onServiceBootstrap != nil {
		return h.onServiceBootstrap(platformName, kubeconfig)
	}
	return &Profile{}, nil
}

func (h *mockCliHandler) OperatorLogin(_ context.Context, apiURL, username, password string) (*Profile, error) {
	if h.onOperatorLogin != nil {
		return h.onOperatorLogin(apiURL, username, password)
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

func (h *mockCliHandler) ServiceDelete(_ context.Context, _ platform.Platform, prof *Profile) error {
	if h.onServiceDelete != nil {
		return h.onServiceDelete(prof)
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

// fakeProfile is a pre-built profile for injecting into auth-required command tests.
var fakeProfile = &Profile{Platform: "local", APIURL: "http://localhost:8080"}

// withProfile returns a LoadProfile hook that always returns fakeProfile.
func withProfile(prof *Profile) func(string, bool) (*Profile, error) {
	return func(_ string, _ bool) (*Profile, error) { return prof, nil }
}

// --- Operator login ---

func TestOperatorLoginUsesProfileAPIURL(t *testing.T) {
	var gotURL string
	mock := &mockCliHandler{
		onLoadProfile: withProfile(fakeProfile),
		onOperatorLogin: func(apiURL, _, _ string) (*Profile, error) {
			gotURL = apiURL
			return &Profile{}, nil
		},
	}
	if err := run(context.Background(), mock, []string{"operator", "login", "--username", "a@b.com", "--password", "x"}); err != nil {
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
		onLoadProfile: withProfile(&Profile{Platform: "local", APIURL: "http://localhost:8080"}),
		onOperatorLogin: func(apiURL, _, _ string) (*Profile, error) {
			gotURL = apiURL
			return &Profile{}, nil
		},
	}
	if err := run(context.Background(), mock, []string{"operator", "login", "--api-url", want, "--username", "a@b.com", "--password", "x"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotURL != want {
		t.Errorf("api-url: want %q, got %q", want, gotURL)
	}
}

func TestOperatorLoginPassesUsernameToHandler(t *testing.T) {
	var gotUsername string
	mock := &mockCliHandler{
		onLoadProfile: withProfile(fakeProfile),
		onOperatorLogin: func(_, username, _ string) (*Profile, error) {
			gotUsername = username
			return &Profile{}, nil
		},
	}
	if err := run(context.Background(), mock, []string{"operator", "login", "--username", "alice@example.com", "--password", "x"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotUsername != "alice@example.com" {
		t.Errorf("username: want %q, got %q", "alice@example.com", gotUsername)
	}
}

func TestOperatorLoginFailsWithoutAPIURLWhenNoProfile(t *testing.T) {
	// onLoadProfile is nil → LoadProfile returns (nil, nil) → c.profile is nil, no --api-url → error.
	mock := &mockCliHandler{}
	err := run(context.Background(), mock, []string{"operator", "login", "--username", "a@b.com", "--password", "x"})
	if err == nil {
		t.Fatal("expected error when no profile and no --api-url, got nil")
	}
}

func TestOperatorLoginFailsWithEmptyAPIURLInProfile(t *testing.T) {
	// Profile exists but has no APIURL — should get a clear message, not a raw HTTP scheme error.
	mock := &mockCliHandler{
		onLoadProfile: withProfile(&Profile{Platform: "local", APIURL: ""}),
	}
	err := run(context.Background(), mock, []string{"operator", "login", "--username", "a@b.com", "--password", "x"})
	if err == nil {
		t.Fatal("expected error when profile has empty APIURL, got nil")
	}
}

func TestOperatorLoginSucceedsWithAPIURLAndNoProfile(t *testing.T) {
	var gotURL string
	const want = "http://remote:8080"
	mock := &mockCliHandler{
		// no onLoadProfile → c.profile stays nil
		onOperatorLogin: func(apiURL, _, _ string) (*Profile, error) {
			gotURL = apiURL
			return &Profile{}, nil
		},
	}
	if err := run(context.Background(), mock, []string{"operator", "login", "--api-url", want, "--username", "a@b.com", "--password", "x"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotURL != want {
		t.Errorf("api-url: want %q, got %q", want, gotURL)
	}
}

// --- Service bootstrap ---

func TestServiceBootstrapPassesFlagsToHandler(t *testing.T) {
	var gotPlatform, gotKubeconfig string
	mock := &mockCliHandler{
		onServiceBootstrap: func(platformName, kubeconfig string) (*Profile, error) {
			gotPlatform = platformName
			gotKubeconfig = kubeconfig
			return &Profile{}, nil
		},
	}
	if err := run(context.Background(), mock, []string{"service", "bootstrap", "--platform", "local", "--kubeconfig", "/tmp/kube"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotPlatform != "local" {
		t.Errorf("platform: want %q, got %q", "local", gotPlatform)
	}
	if gotKubeconfig != "/tmp/kube" {
		t.Errorf("kubeconfig: want %q, got %q", "/tmp/kube", gotKubeconfig)
	}
}

func TestServiceBootstrapRequiresPlatformFlag(t *testing.T) {
	err := run(context.Background(), &mockCliHandler{}, []string{"service", "bootstrap"})
	if err == nil {
		t.Fatal("expected error for missing --platform, got nil")
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
	err := run(context.Background(), mock, []string{"service", "status"})
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
	if err := run(context.Background(), mock, []string{"service", "status"}); err != nil {
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
		onServiceDelete: func(_ *Profile) error {
			called = true
			return nil
		},
	}
	err := run(context.Background(), mock, []string{"service", "delete"})
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
		onServiceDelete: func(_ *Profile) error {
			called = true
			return nil
		},
	}
	if err := run(context.Background(), mock, []string{"service", "delete", "--confirm"}); err != nil {
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
	if err := run(context.Background(), mock, []string{"lint", "--staged"}); err != nil {
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
	if err := run(context.Background(), mock, []string{"lint", "--fix"}); err != nil {
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
	err := run(context.Background(), mock, []string{"lint"})
	if !errors.Is(err, sentinel) {
		t.Errorf("expected sentinel error, got: %v", err)
	}
}
