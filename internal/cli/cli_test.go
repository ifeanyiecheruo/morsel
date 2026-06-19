package cli

import (
	"errors"
	"testing"
)

// --- Test doubles ---

// mockCliHandler implements Handler using optional function hooks.
// Hooks that are nil return zero values with no error (except OperatorLogin,
// which returns a safe empty profile). Tests set only the hooks they care about.
type mockCliHandler struct {
	onServiceBootstrap        func(platformName, kubeconfig string) error
	onOperatorLogin           func() (*Profile, error)
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

func (h *mockCliHandler) ServiceBootstrap(platformName, kubeconfig string) error {
	if h.onServiceBootstrap != nil {
		return h.onServiceBootstrap(platformName, kubeconfig)
	}
	return nil
}

func (h *mockCliHandler) OperatorLogin() (*Profile, error) {
	if h.onOperatorLogin != nil {
		return h.onOperatorLogin()
	}
	return &Profile{}, nil
}

func (h *mockCliHandler) Lint(staged, fix bool) error {
	if h.onLint != nil {
		return h.onLint(staged, fix)
	}
	return nil
}

func (h *mockCliHandler) ServiceStatus(prof *Profile) error {
	if h.onServiceStatus != nil {
		return h.onServiceStatus(prof)
	}
	return nil
}

func (h *mockCliHandler) ServiceDelete(prof *Profile) error {
	if h.onServiceDelete != nil {
		return h.onServiceDelete(prof)
	}
	return nil
}

func (h *mockCliHandler) ServiceUpgradeRetry(prof *Profile) error {
	if h.onServiceUpgradeRetry != nil {
		return h.onServiceUpgradeRetry(prof)
	}
	return nil
}

func (h *mockCliHandler) OperatorLogout(prof *Profile) error {
	if h.onOperatorLogout != nil {
		return h.onOperatorLogout(prof)
	}
	return nil
}

func (h *mockCliHandler) OperatorPrincipalAdd(prof *Profile, principal string) error {
	if h.onOperatorPrincipalAdd != nil {
		return h.onOperatorPrincipalAdd(prof, principal)
	}
	return nil
}

func (h *mockCliHandler) OperatorPrincipalRemove(prof *Profile, principal string) error {
	if h.onOperatorPrincipalRemove != nil {
		return h.onOperatorPrincipalRemove(prof, principal)
	}
	return nil
}

func (h *mockCliHandler) OperatorPrincipalList(prof *Profile) error {
	if h.onOperatorPrincipalList != nil {
		return h.onOperatorPrincipalList(prof)
	}
	return nil
}

func (h *mockCliHandler) TierList(prof *Profile) error {
	if h.onTierList != nil {
		return h.onTierList(prof)
	}
	return nil
}

func (h *mockCliHandler) TierCreate(prof *Profile, flags TierFlags) error {
	if h.onTierCreate != nil {
		return h.onTierCreate(prof, flags)
	}
	return nil
}

func (h *mockCliHandler) TierEdit(prof *Profile, flags TierFlags) error {
	if h.onTierEdit != nil {
		return h.onTierEdit(prof, flags)
	}
	return nil
}

func (h *mockCliHandler) TierSetDefault(prof *Profile, name string) error {
	if h.onTierSetDefault != nil {
		return h.onTierSetDefault(prof, name)
	}
	return nil
}

func (h *mockCliHandler) TierDelete(prof *Profile, name string) error {
	if h.onTierDelete != nil {
		return h.onTierDelete(prof, name)
	}
	return nil
}

func (h *mockCliHandler) AppExemptAdd(prof *Profile, repo, app string) error {
	if h.onAppExemptAdd != nil {
		return h.onAppExemptAdd(prof, repo, app)
	}
	return nil
}

func (h *mockCliHandler) AppExemptRemove(prof *Profile, repo, app string) error {
	if h.onAppExemptRemove != nil {
		return h.onAppExemptRemove(prof, repo, app)
	}
	return nil
}

func (h *mockCliHandler) RepoExemptAdd(prof *Profile, repo string) error {
	if h.onRepoExemptAdd != nil {
		return h.onRepoExemptAdd(prof, repo)
	}
	return nil
}

func (h *mockCliHandler) RepoExemptRemove(prof *Profile, repo string) error {
	if h.onRepoExemptRemove != nil {
		return h.onRepoExemptRemove(prof, repo)
	}
	return nil
}

func (h *mockCliHandler) AppDeploy(prof *Profile) error {
	if h.onAppDeploy != nil {
		return h.onAppDeploy(prof)
	}
	return nil
}

// fakeProfile is a pre-built profile for injecting into auth-required command tests.
var fakeProfile = &Profile{Platform: "local", APIURL: "http://localhost:8080"}

// --- Service bootstrap ---

func TestServiceBootstrapPassesFlagsToHandler(t *testing.T) {
	var gotPlatform, gotKubeconfig string
	mock := &mockCliHandler{
		onServiceBootstrap: func(platformName, kubeconfig string) error {
			gotPlatform = platformName
			gotKubeconfig = kubeconfig
			return nil
		},
	}
	if err := run(mock, []string{"service", "bootstrap", "--platform", "local", "--kubeconfig", "/tmp/kube"}, nil); err != nil {
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
	err := run(&mockCliHandler{}, []string{"service", "bootstrap"}, nil)
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
	}
	err := run(mock, []string{"service", "status"}, nil)
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
		onServiceStatus: func(prof *Profile) error {
			gotProf = prof
			return nil
		},
	}
	if err := run(mock, []string{"service", "status"}, fakeProfile); err != nil {
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
		onServiceDelete: func(_ *Profile) error {
			called = true
			return nil
		},
	}
	err := run(mock, []string{"service", "delete"}, fakeProfile)
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
		onServiceDelete: func(_ *Profile) error {
			called = true
			return nil
		},
	}
	if err := run(mock, []string{"service", "delete", "--confirm"}, fakeProfile); err != nil {
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
	if err := run(mock, []string{"lint", "--staged"}, nil); err != nil {
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
	if err := run(mock, []string{"lint", "--fix"}, nil); err != nil {
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
		onTierCreate: func(_ *Profile, flags TierFlags) error {
			gotFlags = flags
			return nil
		},
	}
	err := run(mock,
		[]string{"operator", "tier", "create", "--name", "small", "--cpu", "0.5", "--max-apps", "3"},
		fakeProfile,
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
		onAppExemptAdd: func(_ *Profile, repo, app string) error {
			gotRepo = repo
			gotApp = app
			return nil
		},
	}
	err := run(mock,
		[]string{"operator", "app", "exempt", "add", "--repo", "org/myrepo", "--app", "api"},
		fakeProfile,
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
	err := run(mock, []string{"lint"}, nil)
	if !errors.Is(err, sentinel) {
		t.Errorf("expected sentinel error, got: %v", err)
	}
}
