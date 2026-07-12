// Package store provides typed access to the Morsel SQLite database.
package store

import (
	"context"
	"database/sql"
	"time"

	dbqueries "github.com/ifeanyiecheruo/morsel/cmd/morsel-ctrl-plane/internal/db/queries"
)

// Re-export domain types so callers do not import db/queries directly.
type RefreshToken = dbqueries.RefreshToken
type App = dbqueries.App
type Repo = dbqueries.Repo
type Operation = dbqueries.Operation
type Tier = dbqueries.Tier
type ScaleEvent = dbqueries.ScaleEvent
type PriceSnapshot = dbqueries.PriceSnapshot
type PlatformConfig = dbqueries.PlatformConfig
type Exemption = dbqueries.Exemption
type StaleSuppressed = dbqueries.StaleSuppressed
type Principal = dbqueries.Principal

// fallbackDefaultTier is the built-in baseline used when no default tier exists.
const fallbackDefaultTier = "small"

// Store wraps the sqlc-generated query layer with typed, domain-level methods.
type Store struct {
	q  *dbqueries.Queries
	db *sql.DB
}

// New constructs a Store backed by the given queries instance and raw DB handle.
// The raw DB handle is used only for health checks (Ping).
func New(q *dbqueries.Queries, db *sql.DB) *Store {
	return &Store{q: q, db: db}
}

// Ping checks that the underlying database is reachable.
func (s *Store) Ping(ctx context.Context) error {
	return s.db.PingContext(ctx)
}

// ── Principals ───────────────────────────────────────────────────────────────

// UpsertPrincipal inserts or updates a principal by GitHub ID, updating the login if it changed.
func (s *Store) UpsertPrincipal(ctx context.Context, githubID int64, githubLogin string) (Principal, error) {
	return s.q.UpsertPrincipal(ctx, dbqueries.UpsertPrincipalParams{
		GithubID:    githubID,
		GithubLogin: githubLogin,
	})
}

// GetPrincipalByID returns the principal with the given GitHub user ID.
func (s *Store) GetPrincipalByID(ctx context.Context, githubID int64) (Principal, error) {
	return s.q.GetPrincipalByID(ctx, githubID)
}

// GetPrincipalByLogin returns the principal with the given GitHub login.
func (s *Store) GetPrincipalByLogin(ctx context.Context, githubLogin string) (Principal, error) {
	return s.q.GetPrincipalByLogin(ctx, githubLogin)
}

// ListPrincipals returns all principals ordered by GitHub login.
func (s *Store) ListPrincipals(ctx context.Context) ([]Principal, error) {
	return s.q.ListPrincipals(ctx)
}

// DeletePrincipal removes the principal with the given GitHub user ID.
func (s *Store) DeletePrincipal(ctx context.Context, githubID int64) error {
	return s.q.DeletePrincipal(ctx, githubID)
}

// SetOperator grants or revokes operator privileges for a principal.
func (s *Store) SetOperator(ctx context.Context, githubID int64, isOperator bool) error {
	var v int64
	if isOperator {
		v = 1
	}
	return s.q.SetPrincipalIsOperator(ctx, dbqueries.SetPrincipalIsOperatorParams{
		GithubID:   githubID,
		IsOperator: v,
	})
}

// SetAdmin grants or revokes admin privileges for a principal.
func (s *Store) SetAdmin(ctx context.Context, githubID int64, isAdmin bool) error {
	var v int64
	if isAdmin {
		v = 1
	}
	return s.q.SetPrincipalIsAdmin(ctx, dbqueries.SetPrincipalIsAdminParams{
		GithubID: githubID,
		IsAdmin:  v,
	})
}

// CountAdmins returns the number of principals with admin privileges.
func (s *Store) CountAdmins(ctx context.Context) (int64, error) {
	return s.q.CountAdmins(ctx)
}

// CountOperators returns the number of principals with operator privileges.
func (s *Store) CountOperators(ctx context.Context) (int64, error) {
	return s.q.CountOperators(ctx)
}

// ── Refresh tokens ───────────────────────────────────────────────────────────

func (s *Store) InsertRefreshToken(ctx context.Context, id, tokenHash, subject, role string, expiresAt time.Time) error {
	return s.q.InsertRefreshToken(ctx, dbqueries.InsertRefreshTokenParams{
		ID:        id,
		TokenHash: tokenHash,
		Subject:   subject,
		Role:      role,
		RepoSlug:  sql.NullString{},
		ExpiresAt: expiresAt,
	})
}

func (s *Store) GetRefreshTokenByHash(ctx context.Context, hash string) (RefreshToken, error) {
	return s.q.GetRefreshTokenByHash(ctx, hash)
}

func (s *Store) RotateRefreshToken(ctx context.Context, id, tokenHash string, expiresAt time.Time) error {
	_, err := s.q.RotateRefreshToken(ctx, dbqueries.RotateRefreshTokenParams{
		ID:        id,
		TokenHash: tokenHash,
		ExpiresAt: expiresAt,
	})
	return err
}

// ── Tiers ─────────────────────────────────────────────────────────────────────

func (s *Store) ListTiers(ctx context.Context) ([]Tier, error) {
	return s.q.ListTiers(ctx)
}

func (s *Store) GetTier(ctx context.Context, name string) (Tier, error) {
	return s.q.GetTier(ctx, name)
}

// GetDefaultTier returns the current platform default tier. Falls back to the
// built-in "small" baseline name if no tier is marked default.
func (s *Store) GetDefaultTierName(ctx context.Context) string {
	t, err := s.q.GetDefaultTier(ctx)
	if err != nil {
		return fallbackDefaultTier
	}
	return t.Name
}

func (s *Store) InsertTier(ctx context.Context, name string, maxApps, cpuMilli, memoryMB, blobGB, databaseGB, queueGB int64, hibernateAfter string) (Tier, error) {
	return s.q.InsertTier(ctx, dbqueries.InsertTierParams{
		Name:           name,
		MaxApps:        maxApps,
		CpuMilli:       cpuMilli,
		MemoryMb:       memoryMB,
		BlobGb:         blobGB,
		DatabaseGb:     databaseGB,
		QueueGb:        queueGB,
		HibernateAfter: hibernateAfter,
	})
}

// PatchTier reads the current tier, applies non-zero fields from the patch, and
// persists the result. Fields left at their zero value are unchanged.
func (s *Store) PatchTier(ctx context.Context, name string, maxApps, cpuMilli, memoryMB, blobGB, databaseGB, queueGB int64, hibernateAfter string) (Tier, error) {
	cur, err := s.q.GetTier(ctx, name)
	if err != nil {
		return Tier{}, err
	}
	if maxApps != 0 {
		cur.MaxApps = maxApps
	}
	if cpuMilli != 0 {
		cur.CpuMilli = cpuMilli
	}
	if memoryMB != 0 {
		cur.MemoryMb = memoryMB
	}
	if blobGB != 0 {
		cur.BlobGb = blobGB
	}
	if databaseGB != 0 {
		cur.DatabaseGb = databaseGB
	}
	if queueGB != 0 {
		cur.QueueGb = queueGB
	}
	if hibernateAfter != "" {
		cur.HibernateAfter = hibernateAfter
	}
	return s.q.UpdateTier(ctx, dbqueries.UpdateTierParams{
		Name:           name,
		MaxApps:        cur.MaxApps,
		CpuMilli:       cur.CpuMilli,
		MemoryMb:       cur.MemoryMb,
		BlobGb:         cur.BlobGb,
		DatabaseGb:     cur.DatabaseGb,
		QueueGb:        cur.QueueGb,
		HibernateAfter: cur.HibernateAfter,
	})
}

func (s *Store) DeleteTier(ctx context.Context, name string) error {
	return s.q.DeleteTier(ctx, name)
}

func (s *Store) SetDefaultTier(ctx context.Context, name string) error {
	return s.q.SetDefaultTier(ctx, name)
}

func (s *Store) CountReposByTier(ctx context.Context, tier string) (int64, error) {
	return s.q.CountReposByTier(ctx, tier)
}

func (s *Store) ListReposByTier(ctx context.Context, tier string) ([]Repo, error) {
	return s.q.ListReposByTier(ctx, tier)
}

func (s *Store) SetRepoTier(ctx context.Context, slug, tier string) (Repo, error) {
	return s.q.UpdateRepoTier(ctx, dbqueries.UpdateRepoTierParams{Slug: slug, Tier: tier})
}

// ── Repos ─────────────────────────────────────────────────────────────────────

// GetOrCreateRepo returns the repo by slug, creating it with the given defaultTier if absent.
func (s *Store) GetOrCreateRepo(ctx context.Context, slug, defaultTier string) (Repo, error) {
	return s.q.UpsertRepo(ctx, dbqueries.UpsertRepoParams{Slug: slug, Tier: defaultTier})
}

// GetRepo returns the repo by slug. Returns sql.ErrNoRows if not found.
func (s *Store) GetRepo(ctx context.Context, slug string) (Repo, error) {
	return s.q.GetRepo(ctx, slug)
}

// ListRepos returns all repos ordered by slug.
func (s *Store) ListRepos(ctx context.Context) ([]Repo, error) {
	return s.q.ListRepos(ctx)
}

// CountAppsByRepo returns the number of non-deleted apps in the repo.
func (s *Store) CountAppsByRepo(ctx context.Context, slug string) (int64, error) {
	return s.q.CountAppsByRepo(ctx, slug)
}

// ── Apps ──────────────────────────────────────────────────────────────────────

// GetApp returns the app by repo slug and name. Returns sql.ErrNoRows if not found.
func (s *Store) GetApp(ctx context.Context, repoSlug, name string) (App, error) {
	return s.q.GetApp(ctx, dbqueries.GetAppParams{RepoSlug: repoSlug, Name: name})
}

// ListApps returns all non-deleted apps in the repo, ordered by name.
func (s *Store) ListApps(ctx context.Context, repoSlug string) ([]App, error) {
	return s.q.ListAppsByRepo(ctx, repoSlug)
}

// UpsertApp creates or updates the app record. Returns the resulting row.
// idleAfter is an optional duration string (e.g. "24h"); empty string means platform default.
func (s *Store) UpsertApp(ctx context.Context, repoSlug, name, appType, namespace, image, idleAfter string) (App, error) {
	var ns sql.NullString
	if namespace != "" {
		ns = sql.NullString{String: namespace, Valid: true}
	}
	var img sql.NullString
	if image != "" {
		img = sql.NullString{String: image, Valid: true}
	}
	var ia sql.NullString
	if idleAfter != "" {
		ia = sql.NullString{String: idleAfter, Valid: true}
	}
	return s.q.UpsertApp(ctx, dbqueries.UpsertAppParams{
		RepoSlug:     repoSlug,
		Name:         name,
		Type:         appType,
		Namespace:    ns,
		ImageCurrent: img,
		IdleAfter:    ia,
	})
}

// SetAppHibernated records the app as hibernated with the given reason.
func (s *Store) SetAppHibernated(ctx context.Context, id int64, reason string) error {
	var r sql.NullString
	if reason != "" {
		r = sql.NullString{String: reason, Valid: true}
	}
	return s.q.SetAppHibernated(ctx, dbqueries.SetAppHibernatedParams{
		HibernationReason: r,
		ID:                id,
	})
}

// SetAppAwake clears hibernation state and marks the app running.
func (s *Store) SetAppAwake(ctx context.Context, id int64) error {
	return s.q.SetAppAwake(ctx, id)
}

// UpdateLastActiveAt refreshes the last_active_at timestamp for an app.
func (s *Store) UpdateLastActiveAt(ctx context.Context, id int64) error {
	return s.q.UpdateLastActiveAt(ctx, id)
}

// ListAllApps returns all non-deleted apps across all repos, ordered by repo then name.
func (s *Store) ListAllApps(ctx context.Context) ([]App, error) {
	return s.q.ListAllApps(ctx)
}

// GetAppByNamespace returns the app whose Kubernetes namespace matches the given value.
// Returns sql.ErrNoRows if not found.
func (s *Store) GetAppByNamespace(ctx context.Context, namespace string) (App, error) {
	return s.q.GetAppByNamespace(ctx, sql.NullString{String: namespace, Valid: true})
}

// MarkAppDeletionPending begins the deletion grace period for the given app.
func (s *Store) MarkAppDeletionPending(ctx context.Context, id int64) error {
	return s.q.MarkAppDeletionPending(ctx, id)
}

// UpdateAppStatus sets the runtime status string for an app (e.g. "running", "pending").
func (s *Store) UpdateAppStatus(ctx context.Context, id int64, status string) error {
	return s.q.UpdateAppStatus(ctx, dbqueries.UpdateAppStatusParams{ID: id, Status: status})
}

// UpdateAppImages records the current and last-healthy image digests for an app.
func (s *Store) UpdateAppImages(ctx context.Context, id int64, current, lastHealthy string) error {
	var cur, lh sql.NullString
	if current != "" {
		cur = sql.NullString{String: current, Valid: true}
	}
	if lastHealthy != "" {
		lh = sql.NullString{String: lastHealthy, Valid: true}
	}
	return s.q.UpdateAppImages(ctx, dbqueries.UpdateAppImagesParams{
		ID:               id,
		ImageCurrent:     cur,
		ImageLastHealthy: lh,
	})
}

// ── Operations ────────────────────────────────────────────────────────────────

// GetOperation returns the operation by ID. Returns sql.ErrNoRows if not found.
func (s *Store) GetOperation(ctx context.Context, id string) (Operation, error) {
	return s.q.GetOperation(ctx, id)
}

func (s *Store) CreateOperation(ctx context.Context, id, repoSlug, appName, kind string) (Operation, error) {
	return s.q.CreateOperation(ctx, dbqueries.CreateOperationParams{
		ID:       id,
		RepoSlug: repoSlug,
		AppName:  appName,
		Kind:     kind,
	})
}

func (s *Store) StartOperation(ctx context.Context, id string) error {
	return s.q.UpdateOperationStatus(ctx, dbqueries.UpdateOperationStatusParams{
		ID:     id,
		Status: "running",
		Error:  sql.NullString{},
	})
}

func (s *Store) SucceedOperation(ctx context.Context, id string) error {
	return s.q.UpdateOperationStatus(ctx, dbqueries.UpdateOperationStatusParams{
		ID:     id,
		Status: "succeeded",
		Error:  sql.NullString{},
	})
}

func (s *Store) FailOperation(ctx context.Context, id, message string) error {
	return s.q.UpdateOperationStatus(ctx, dbqueries.UpdateOperationStatusParams{
		ID:     id,
		Status: "failed",
		Error:  sql.NullString{String: message, Valid: true},
	})
}

// ListAppOperations returns all operations for an app in reverse chronological order.
func (s *Store) ListAppOperations(ctx context.Context, repoSlug, appName string) ([]Operation, error) {
	return s.q.ListOperationsByApp(ctx, dbqueries.ListOperationsByAppParams{
		RepoSlug: repoSlug,
		AppName:  appName,
	})
}

// ── Scale events ──────────────────────────────────────────────────────────────

// RecordScaleEvent writes a scale_to_0 or scale_to_1 event for the given app.
func (s *Store) RecordScaleEvent(ctx context.Context, namespace, app, event string) error {
	return s.q.InsertScaleEvent(ctx, dbqueries.InsertScaleEventParams{
		Namespace:  namespace,
		App:        app,
		Event:      event,
		OccurredAt: time.Now().UTC(),
	})
}

// ListScaleEventsSince returns scale events for a specific app since the given time.
func (s *Store) ListScaleEventsSince(ctx context.Context, namespace, app string, since time.Time) ([]ScaleEvent, error) {
	return s.q.ListScaleEventsSince(ctx, dbqueries.ListScaleEventsSinceParams{
		Namespace:  namespace,
		App:        app,
		OccurredAt: since,
	})
}

// ListAllScaleEventsSince returns all scale events across all apps since the given time.
func (s *Store) ListAllScaleEventsSince(ctx context.Context, since time.Time) ([]ScaleEvent, error) {
	return s.q.ListAllScaleEventsSince(ctx, since)
}

// ── Platform config ───────────────────────────────────────────────────────────

// GetPlatformConfig returns the single platform-wide configuration row.
func (s *Store) GetPlatformConfig(ctx context.Context) (PlatformConfig, error) {
	return s.q.GetPlatformConfig(ctx)
}

// PatchPlatformConfig reads the current config and applies non-zero fields.
func (s *Store) PatchPlatformConfig(ctx context.Context, ceilingMonthly, softLimitPct, hardLimitPct float64, defaultIdleAfter string) (PlatformConfig, error) {
	cur, err := s.q.GetPlatformConfig(ctx)
	if err != nil {
		return PlatformConfig{}, err
	}
	if ceilingMonthly != 0 {
		cur.BudgetCeilingMonthly = ceilingMonthly
	}
	if softLimitPct != 0 {
		cur.SoftLimitPct = softLimitPct
	}
	if hardLimitPct != 0 {
		cur.HardLimitPct = hardLimitPct
	}
	if defaultIdleAfter != "" {
		cur.DefaultIdleAfter = defaultIdleAfter
	}
	return s.q.UpdatePlatformConfig(ctx, dbqueries.UpdatePlatformConfigParams{
		BudgetCeilingMonthly:  cur.BudgetCeilingMonthly,
		SoftLimitPct:          cur.SoftLimitPct,
		HardLimitPct:          cur.HardLimitPct,
		DefaultIdleAfter:      cur.DefaultIdleAfter,
		BudgetSoftLimitActive: cur.BudgetSoftLimitActive,
		BudgetHardLimitActive: cur.BudgetHardLimitActive,
		BillingPeriod:         cur.BillingPeriod,
	})
}

// UpdatePlatformConfig writes all fields of the config row.
func (s *Store) UpdatePlatformConfig(ctx context.Context, cfg PlatformConfig) (PlatformConfig, error) {
	return s.q.UpdatePlatformConfig(ctx, dbqueries.UpdatePlatformConfigParams{
		BudgetCeilingMonthly:  cfg.BudgetCeilingMonthly,
		SoftLimitPct:          cfg.SoftLimitPct,
		HardLimitPct:          cfg.HardLimitPct,
		DefaultIdleAfter:      cfg.DefaultIdleAfter,
		BudgetSoftLimitActive: cfg.BudgetSoftLimitActive,
		BudgetHardLimitActive: cfg.BudgetHardLimitActive,
		BillingPeriod:         cfg.BillingPeriod,
	})
}

// ── Exemptions ────────────────────────────────────────────────────────────────

// AddAppExemption adds a permanent explicit exemption for a specific app.
func (s *Store) AddAppExemption(ctx context.Context, repoSlug, appName string) error {
	return s.q.UpsertExemption(ctx, dbqueries.UpsertExemptionParams{
		Kind:     "app",
		RepoSlug: repoSlug,
		AppName:  appName,
		Type:     "explicit",
	})
}

// RemoveAppExemption removes the explicit exemption for a specific app.
func (s *Store) RemoveAppExemption(ctx context.Context, repoSlug, appName string) error {
	return s.q.DeleteAppExemption(ctx, dbqueries.DeleteAppExemptionParams{
		RepoSlug: repoSlug,
		AppName:  appName,
	})
}

// AddRepoExemption adds a permanent explicit exemption for all apps in a repo.
func (s *Store) AddRepoExemption(ctx context.Context, repoSlug string) error {
	return s.q.UpsertExemption(ctx, dbqueries.UpsertExemptionParams{
		Kind:     "repo",
		RepoSlug: repoSlug,
		Type:     "explicit",
	})
}

// RemoveRepoExemption removes the explicit repo-level exemption.
func (s *Store) RemoveRepoExemption(ctx context.Context, repoSlug string) error {
	return s.q.DeleteRepoExemption(ctx, repoSlug)
}

// AddPeriodExemption grants a period exemption for a specific app (expires at periodEnd).
func (s *Store) AddPeriodExemption(ctx context.Context, repoSlug, appName string, periodEnd time.Time) error {
	return s.q.UpsertExemption(ctx, dbqueries.UpsertExemptionParams{
		Kind:      "app",
		RepoSlug:  repoSlug,
		AppName:   appName,
		Type:      "period",
		ExpiresAt: sql.NullTime{Time: periodEnd, Valid: true},
	})
}

// ListExemptions returns all active exemptions (explicit + non-expired period).
func (s *Store) ListExemptions(ctx context.Context) ([]Exemption, error) {
	return s.q.ListActiveExemptions(ctx)
}

// IsAppExempt reports whether the app has an active exemption (repo-level or app-level).
func (s *Store) IsAppExempt(ctx context.Context, repoSlug, appName string) (bool, error) {
	return s.q.IsAppExempt(ctx, dbqueries.IsAppExemptParams{
		RepoSlug: repoSlug,
		AppName:  appName,
	})
}

// ExpirePeriodExemptions deletes period exemptions whose expiry time has passed.
func (s *Store) ExpirePeriodExemptions(ctx context.Context, now time.Time) error {
	return s.q.ExpirePeriodExemptions(ctx, sql.NullTime{Time: now, Valid: true})
}

// ── Stale suppression ─────────────────────────────────────────────────────────

// SuppressStaleApp records that the stale-apps view should hide this app until
// the given time. If an existing entry exists it is replaced.
func (s *Store) SuppressStaleApp(ctx context.Context, repoSlug, appName string, until time.Time) error {
	return s.q.UpsertStaleSuppressed(ctx, dbqueries.UpsertStaleSuppressedParams{
		RepoSlug: repoSlug,
		AppName:  appName,
		Until:    until.Format(time.RFC3339),
	})
}

// ListActiveStaleSuppressed returns all stale-app suppressions that have not yet expired.
func (s *Store) ListActiveStaleSuppressed(ctx context.Context) ([]StaleSuppressed, error) {
	return s.q.ListActiveStaleSuppressed(ctx, time.Now().Format(time.RFC3339))
}

// ── Price snapshots ───────────────────────────────────────────────────────────

// InsertPriceSnapshot stores an immutable price fetch result.
func (s *Store) InsertPriceSnapshot(ctx context.Context, cpuPerCore, memPerGB, storagePerGB, registryPerGB float64, fetchedAt time.Time) (PriceSnapshot, error) {
	return s.q.InsertPriceSnapshot(ctx, dbqueries.InsertPriceSnapshotParams{
		ComputeCpuPerCoreHour: cpuPerCore,
		ComputeMemPerGbHour:   memPerGB,
		StoragePerGbMonth:     storagePerGB,
		RegistryPerGbMonth:    registryPerGB,
		FetchedAt:             fetchedAt,
	})
}

// GetLatestPriceSnapshot returns the most recently fetched price snapshot.
// Returns sql.ErrNoRows if no snapshot exists yet.
func (s *Store) GetLatestPriceSnapshot(ctx context.Context) (PriceSnapshot, error) {
	return s.q.GetLatestPriceSnapshot(ctx)
}

// ListPriceSnapshots returns all price snapshots ordered most-recent-first.
func (s *Store) ListPriceSnapshots(ctx context.Context) ([]PriceSnapshot, error) {
	return s.q.ListPriceSnapshots(ctx)
}
