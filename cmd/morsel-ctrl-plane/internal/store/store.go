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

// Store wraps the sqlc-generated query layer with typed, domain-level methods.
type Store struct {
	q *dbqueries.Queries
}

// New constructs a Store backed by the given queries instance.
func New(q *dbqueries.Queries) *Store {
	return &Store{q: q}
}

// ── Principals ───────────────────────────────────────────────────────────────

// ListPrincipals returns all operator principal usernames in lexicographic order.
func (s *Store) ListPrincipals(ctx context.Context) ([]string, error) {
	return s.q.ListPrincipals(ctx)
}

// AddPrincipal adds an operator principal. Idempotent — a duplicate username is silently ignored.
func (s *Store) AddPrincipal(ctx context.Context, username string) error {
	return s.q.InsertPrincipal(ctx, username)
}

// RemovePrincipal removes an operator principal. No-op if the username is not present.
func (s *Store) RemovePrincipal(ctx context.Context, username string) error {
	return s.q.DeletePrincipal(ctx, username)
}

func (s *Store) PrincipalExists(ctx context.Context, username string) (bool, error) {
	return s.q.PrincipalExists(ctx, username)
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

// ── Repos ─────────────────────────────────────────────────────────────────────

// GetOrCreateRepo returns the repo by slug, creating it with tier 'small' if absent.
func (s *Store) GetOrCreateRepo(ctx context.Context, slug string) (Repo, error) {
	return s.q.UpsertRepo(ctx, slug)
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
