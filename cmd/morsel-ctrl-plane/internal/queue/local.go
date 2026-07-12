package queue

import (
	"context"
	"crypto/rand"
	"database/sql"
	_ "embed"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/ifeanyiecheruo/morsel/cmd/morsel-ctrl-plane/internal/queue/queries"
	"github.com/ifeanyiecheruo/morsel/internal/ctxlog"
	_ "modernc.org/sqlite"
)

const (
	defaultVisibilityTimeout = 30 * time.Second

	// sqliteTimeLayout matches SQLite's strftime('%Y-%m-%dT%H:%M:%fZ', 'now')
	// output exactly (millisecond precision, T separator, Z suffix). String
	// ordering is therefore correct for visibility_until comparisons.
	sqliteTimeLayout = "2006-01-02T15:04:05.000Z"
)

// Package-level notifier state shared across all LocalQueue instances in the
// same process. Required for in-process long-poll to work when the caller
// creates a new LocalQueue per request rather than sharing a single instance.
var (
	notifierMu sync.Mutex
	notifiers  = make(map[string]chan struct{})
)

// LocalQueue implements Queue with one SQLite file per queue stored under
// baseDir. Storage layout:
//
//	{baseDir}/{namespace}/{name}.db  — messages + meta per queue
//	{baseDir}/{namespace}/quota.db   — total_bytes and limit_bytes per namespace
type LocalQueue struct {
	baseDir   string
	namespace string
}

// NewLocalQueue creates a LocalQueue rooted at baseDir for the given namespace.
func NewLocalQueue(baseDir, namespace string) *LocalQueue {
	return &LocalQueue{baseDir: baseDir, namespace: namespace}
}

// — path helpers —

func (lq *LocalQueue) queuePath(name string) string {
	return filepath.Join(lq.baseDir, lq.namespace, name+".db")
}

func (lq *LocalQueue) quotaPath() string {
	return filepath.Join(lq.baseDir, lq.namespace, "quota.db")
}

// — SQLite open helpers —

//go:embed messages_schema.sql
var messagesSchema string

//go:embed quota_schema.sql
var quotaSchema string

func openSQLite(path, schema string) (*sql.DB, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("mkdir: %w", err)
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	for _, pragma := range []string{
		`PRAGMA journal_mode=WAL`,
		`PRAGMA synchronous=NORMAL`,
		`PRAGMA busy_timeout=5000`,
	} {
		if _, err := db.Exec(pragma); err != nil {
			return nil, errors.Join(fmt.Errorf("%s: %w", pragma, err), db.Close())
		}
	}
	if _, err := db.Exec(schema); err != nil {
		return nil, errors.Join(fmt.Errorf("schema: %w", err), db.Close())
	}
	return db, nil
}

func (lq *LocalQueue) openQueue(name string) (*sql.DB, error) {
	p := lq.queuePath(name)
	if _, err := os.Stat(p); errors.Is(err, os.ErrNotExist) {
		return nil, ErrQueueNotFound
	}
	return openSQLite(p, messagesSchema)
}

func (lq *LocalQueue) openOrCreateQueue(name string) (*sql.DB, error) {
	return openSQLite(lq.queuePath(name), messagesSchema)
}

func (lq *LocalQueue) openQuota() (*sql.DB, error) {
	return openSQLite(lq.quotaPath(), quotaSchema)
}

// — notifier helpers (package-level, shared across all LocalQueue instances) —

func notifierKey(namespace, name string) string { return namespace + "/" + name }

func signalQueue(namespace, name string) {
	key := notifierKey(namespace, name)
	notifierMu.Lock()
	ch, ok := notifiers[key]
	notifierMu.Unlock()
	if ok {
		select {
		case ch <- struct{}{}:
		default:
		}
	}
}

// WaitForMessage blocks until an enqueue signals the queue or ctx is done.
func (lq *LocalQueue) WaitForMessage(ctx context.Context, name string) {
	key := notifierKey(lq.namespace, name)
	notifierMu.Lock()
	ch, ok := notifiers[key]
	if !ok {
		ch = make(chan struct{}, 1)
		notifiers[key] = ch
	}
	notifierMu.Unlock()
	select {
	case <-ch:
	case <-ctx.Done():
	}
}

// — Queue implementation —

func (lq *LocalQueue) CreateQueue(_ context.Context, name string) error {
	db, err := lq.openOrCreateQueue(name)
	if err != nil {
		return err
	}
	return db.Close()
}

func (lq *LocalQueue) DeleteQueue(_ context.Context, name string) error {
	p := lq.queuePath(name)
	if _, err := os.Stat(p); errors.Is(err, os.ErrNotExist) {
		return ErrQueueNotFound
	}
	for _, suffix := range []string{"", "-wal", "-shm"} {
		if err := os.Remove(p + suffix); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("remove %s: %w", p+suffix, err)
		}
	}
	return nil
}

func (lq *LocalQueue) ListQueues(ctx context.Context, idleAfter time.Duration) ([]Info, error) {
	return lq.listQueues(ctx, idleAfter)
}

func (lq *LocalQueue) IdleStatus(ctx context.Context, idleAfter time.Duration) ([]Info, error) {
	return lq.listQueues(ctx, idleAfter)
}

func (lq *LocalQueue) listQueues(ctx context.Context, idleAfter time.Duration) ([]Info, error) {
	dir := filepath.Join(lq.baseDir, lq.namespace)
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read dir: %w", err)
	}
	var out []Info
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".db") || e.Name() == "quota.db" {
			continue
		}
		name := strings.TrimSuffix(e.Name(), ".db")
		info, err := lq.queueInfo(ctx, name, idleAfter)
		if err != nil {
			return nil, err
		}
		out = append(out, info)
	}
	return out, nil
}

func (lq *LocalQueue) queueInfo(ctx context.Context, name string, idleAfter time.Duration) (Info, error) {
	db, err := lq.openQueue(name)
	if err != nil {
		return Info{}, err
	}
	defer func() {
		if err := db.Close(); err != nil {
			ctxlog.From(ctx).Error("close queue db", "err", err)
		}
	}()

	q := queries.New(db)
	count, err := q.CountMessages(ctx)
	if err != nil {
		return Info{}, fmt.Errorf("count: %w", err)
	}

	idle := true
	if idleAfter > 0 {
		lastExt, err := q.GetMeta(ctx, "last_external_enqueue_at")
		if err == nil {
			if t, err := time.Parse(sqliteTimeLayout, lastExt); err == nil && time.Since(t) < idleAfter {
				idle = false
			}
		}
	}
	return Info{Name: name, Depth: count, Idle: idle}, nil
}

func (lq *LocalQueue) Enqueue(ctx context.Context, name string, body []byte, senderID string) error {
	quotaDB, err := lq.openQuota()
	if err != nil {
		return fmt.Errorf("open quota: %w", err)
	}
	defer func() {
		if err := quotaDB.Close(); err != nil {
			ctxlog.From(ctx).Error("close quota db", "err", err)
		}
	}()

	quota, err := queries.New(quotaDB).GetQuota(ctx)
	if err != nil {
		return fmt.Errorf("read quota: %w", err)
	}
	if quota.LimitBytes > 0 && quota.TotalBytes+int64(len(body)) > quota.LimitBytes {
		return ErrQuotaExceeded
	}

	queueDB, err := lq.openOrCreateQueue(name)
	if err != nil {
		return fmt.Errorf("open queue: %w", err)
	}
	defer func() {
		if err := queueDB.Close(); err != nil {
			ctxlog.From(ctx).Error("close queue db", "err", err)
		}
	}()

	id, err := newMessageID()
	if err != nil {
		return err
	}
	now := time.Now().UTC().Format(sqliteTimeLayout)

	tx, err := queueDB.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}
	q := queries.New(tx)

	if err := q.InsertMessage(ctx, queries.InsertMessageParams{
		ID:         id,
		Body:       body,
		BodySize:   int64(len(body)),
		EnqueuedAt: now,
	}); err != nil {
		if rErr := tx.Rollback(); rErr != nil {
			ctxlog.From(ctx).Warn("rollback transaction", "err", rErr)
		}
		return fmt.Errorf("insert: %w", err)
	}

	if senderID != lq.namespace {
		if err := q.UpsertMeta(ctx, queries.UpsertMetaParams{
			Key:   "last_external_enqueue_at",
			Value: now,
		}); err != nil {
			if rErr := tx.Rollback(); rErr != nil {
				ctxlog.From(ctx).Warn("rollback transaction", "err", rErr)
			}
			return fmt.Errorf("update meta: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit: %w", err)
	}

	// Update quota best-effort; a crash here under-counts slightly but
	// self-corrects on the next full recalculation.
	if err := queries.New(quotaDB).AddBytes(ctx, int64(len(body))); err != nil {
		ctxlog.From(ctx).Warn("update quota bytes", "err", err)
	}

	signalQueue(lq.namespace, name)
	return nil
}

func (lq *LocalQueue) Dequeue(ctx context.Context, name string, visibilityTimeout time.Duration) (*Message, error) {
	db, err := lq.openQueue(name)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := db.Close(); err != nil {
			ctxlog.From(ctx).Error("close queue db", "err", err)
		}
	}()

	if visibilityTimeout <= 0 {
		visibilityTimeout = defaultVisibilityTimeout
	}
	until := time.Now().UTC().Add(visibilityTimeout).Format(sqliteTimeLayout)

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin: %w", err)
	}
	q := queries.New(tx)

	row, err := q.SelectNextMessage(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		if rErr := tx.Rollback(); rErr != nil {
			ctxlog.From(ctx).Warn("rollback transaction", "err", rErr)
		}
		return nil, nil
	}
	if err != nil {
		if rErr := tx.Rollback(); rErr != nil {
			ctxlog.From(ctx).Warn("rollback transaction", "err", rErr)
		}
		return nil, fmt.Errorf("select: %w", err)
	}

	if err := q.SetVisibility(ctx, queries.SetVisibilityParams{
		VisibilityUntil: sql.NullString{String: until, Valid: true},
		ID:              row.ID,
	}); err != nil {
		if rErr := tx.Rollback(); rErr != nil {
			ctxlog.From(ctx).Warn("rollback transaction", "err", rErr)
		}
		return nil, fmt.Errorf("update visibility: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit: %w", err)
	}

	t, parseErr := time.Parse(sqliteTimeLayout, row.EnqueuedAt)
	if parseErr != nil {
		ctxlog.From(ctx).Warn("parse enqueued_at timestamp", "value", row.EnqueuedAt, "err", parseErr)
	}
	return &Message{ID: row.ID, Body: row.Body, EnqueuedAt: t}, nil
}

func (lq *LocalQueue) Ack(ctx context.Context, name, id string) error {
	db, err := lq.openQueue(name)
	if err != nil {
		return err
	}
	defer func() {
		if err := db.Close(); err != nil {
			ctxlog.From(ctx).Error("close queue db", "err", err)
		}
	}()

	q := queries.New(db)
	bodySize, err := q.GetMessageBodySize(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil // idempotent
	}
	if err != nil {
		return fmt.Errorf("read size: %w", err)
	}

	if err := q.DeleteMessage(ctx, id); err != nil {
		return fmt.Errorf("delete: %w", err)
	}

	quotaDB, err := lq.openQuota()
	if err != nil {
		return fmt.Errorf("open quota: %w", err)
	}
	defer func() {
		if err := quotaDB.Close(); err != nil {
			ctxlog.From(ctx).Error("close quota db", "err", err)
		}
	}()
	// Clamp at zero in case of any accounting drift.
	if _, err := quotaDB.ExecContext(ctx,
		`UPDATE quota SET total_bytes = MAX(0, total_bytes - ?)`, bodySize); err != nil {
		ctxlog.From(ctx).Warn("decrement quota bytes", "err", err)
	}
	return nil
}

func (lq *LocalQueue) Depth(ctx context.Context, name string) (int64, error) {
	db, err := lq.openQueue(name)
	if err != nil {
		return 0, err
	}
	defer func() {
		if err := db.Close(); err != nil {
			ctxlog.From(ctx).Error("close queue db", "err", err)
		}
	}()

	count, err := queries.New(db).CountMessages(ctx)
	if err != nil {
		return 0, fmt.Errorf("count: %w", err)
	}
	return count, nil
}

func (lq *LocalQueue) SetQuota(ctx context.Context, limitBytes int64) error {
	db, err := lq.openQuota()
	if err != nil {
		return err
	}
	defer func() {
		if err := db.Close(); err != nil {
			ctxlog.From(ctx).Error("close quota db", "err", err)
		}
	}()
	return queries.New(db).SetLimitBytes(ctx, limitBytes)
}

func (lq *LocalQueue) Usage(ctx context.Context) (int64, error) {
	db, err := lq.openQuota()
	if err != nil {
		return 0, err
	}
	defer func() {
		if err := db.Close(); err != nil {
			ctxlog.From(ctx).Error("close quota db", "err", err)
		}
	}()

	quota, err := queries.New(db).GetQuota(ctx)
	if err != nil {
		return 0, fmt.Errorf("read quota: %w", err)
	}
	return quota.TotalBytes, nil
}

func newMessageID() (string, error) {
	b := make([]byte, 12)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return "msg_" + hex.EncodeToString(b), nil
}

var _ Queue = (*LocalQueue)(nil)
