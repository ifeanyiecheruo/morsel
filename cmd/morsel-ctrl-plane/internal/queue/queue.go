// Package queue defines the Queue interface and its LocalQueue implementation
// (one SQLite file per queue). The queue-service binary and the control-plane
// platform interface both use this package; the namespace is fixed at
// construction time so interface methods carry no namespace parameter.
package queue

import (
	"context"
	"errors"
	"time"
)

var (
	// ErrQueueNotFound is returned when the named queue does not exist.
	ErrQueueNotFound = errors.New("queue not found")

	// ErrQuotaExceeded is returned by Enqueue when total message storage
	// would exceed the namespace quota limit.
	ErrQuotaExceeded = errors.New("queue quota exceeded")
)

// LongPollTimeout is the maximum duration WaitForMessage blocks before
// returning, used by the dequeue HTTP handler.
const LongPollTimeout = 5 * time.Second

// Queue is the queue storage interface scoped to a single namespace.
// The namespace is set at construction time and is not repeated on each method.
type Queue interface {
	// CreateQueue creates a named queue. Idempotent — safe to call if the
	// queue already exists.
	CreateQueue(ctx context.Context, name string) error

	// DeleteQueue drops the named queue and all its messages.
	// Returns ErrQueueNotFound if the queue does not exist.
	DeleteQueue(ctx context.Context, name string) error

	// ListQueues returns all queues in this namespace with current depth and
	// idle status. idleAfter controls when last_external_enqueue_at is
	// considered stale enough to mark the queue idle (0 = always idle).
	ListQueues(ctx context.Context, idleAfter time.Duration) ([]Info, error)

	// Enqueue inserts body into the named queue. senderID is the Kubernetes
	// namespace of the calling pod; when it differs from the Queue's own
	// namespace the enqueue is treated as external and updates
	// last_external_enqueue_at. Returns ErrQuotaExceeded if the namespace
	// quota would be exceeded.
	Enqueue(ctx context.Context, name string, body []byte, senderID string) error

	// Dequeue claims the next available message and sets its visibility
	// timeout. Returns (nil, nil) if the queue is empty.
	Dequeue(ctx context.Context, name string, visibilityTimeout time.Duration) (*Message, error)

	// WaitForMessage blocks until a message is enqueued or ctx is cancelled.
	// Used by the dequeue handler to implement long-poll.
	WaitForMessage(ctx context.Context, name string)

	// Ack removes a message by ID. Idempotent — safe to call multiple times.
	Ack(ctx context.Context, name, id string) error

	// Depth returns the count of all pending messages (including in-flight
	// messages whose visibility timeout has not yet expired).
	Depth(ctx context.Context, name string) (int64, error)

	// SetQuota sets the total byte limit for this namespace.
	SetQuota(ctx context.Context, limitBytes int64) error

	// Usage returns the total message bytes currently stored in this namespace.
	Usage(ctx context.Context) (int64, error)

	// IdleStatus returns the idle flag for every queue in this namespace.
	// This is the internal endpoint consumed by the hibernation watcher.
	IdleStatus(ctx context.Context, idleAfter time.Duration) ([]Info, error)
}

// Info describes a single queue as returned by ListQueues and IdleStatus.
type Info struct {
	Name  string
	Depth int64
	Idle  bool
}

// Message is a dequeued message. Body is the raw bytes stored at enqueue time.
type Message struct {
	ID         string
	Body       []byte
	EnqueuedAt time.Time
}
