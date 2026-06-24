// Package store provides typed access to the Morsel SQLite database.
package store

import (
	"context"
	"database/sql"
	"time"

	dbqueries "github.com/ifeanyiecheruo/morsel/internal/db/queries"
)

// RefreshToken is re-exported so callers do not need to import db/queries directly.
type RefreshToken = dbqueries.RefreshToken

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

// PrincipalExists reports whether the given username is a registered operator.
func (s *Store) PrincipalExists(ctx context.Context, username string) (bool, error) {
	return s.q.PrincipalExists(ctx, username)
}

// ── Refresh tokens ───────────────────────────────────────────────────────────

// InsertRefreshToken stores a new refresh token record.
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

// GetRefreshTokenByHash looks up a refresh token by its hash.
func (s *Store) GetRefreshTokenByHash(ctx context.Context, hash string) (RefreshToken, error) {
	return s.q.GetRefreshTokenByHash(ctx, hash)
}

// RotateRefreshToken replaces the hash and expiry on an existing token record.
func (s *Store) RotateRefreshToken(ctx context.Context, id, tokenHash string, expiresAt time.Time) error {
	_, err := s.q.RotateRefreshToken(ctx, dbqueries.RotateRefreshTokenParams{
		ID:        id,
		TokenHash: tokenHash,
		ExpiresAt: expiresAt,
	})
	return err
}
