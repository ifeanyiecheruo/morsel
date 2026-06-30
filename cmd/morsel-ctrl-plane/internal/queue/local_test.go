package queue_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/ifeanyiecheruo/morsel/cmd/morsel-ctrl-plane/internal/queue"
)

const testNS = "test-ns"

func newQueue(t *testing.T) queue.Queue {
	t.Helper()
	return queue.NewLocalQueue(t.TempDir(), testNS)
}

func TestCreateAndDeleteQueue(t *testing.T) {
	q := newQueue(t)
	ctx := context.Background()

	if err := q.CreateQueue(ctx, "jobs"); err != nil {
		t.Fatalf("CreateQueue: %v", err)
	}
	// Idempotent second call.
	if err := q.CreateQueue(ctx, "jobs"); err != nil {
		t.Fatalf("CreateQueue idempotent: %v", err)
	}
	if err := q.DeleteQueue(ctx, "jobs"); err != nil {
		t.Fatalf("DeleteQueue: %v", err)
	}
}

func TestDeleteQueueNotFound(t *testing.T) {
	q := newQueue(t)
	err := q.DeleteQueue(context.Background(), "nonexistent")
	if !errors.Is(err, queue.ErrQueueNotFound) {
		t.Fatalf("got %v, want ErrQueueNotFound", err)
	}
}

func TestEnqueueDequeueAck(t *testing.T) {
	q := newQueue(t)
	ctx := context.Background()

	if err := q.CreateQueue(ctx, "jobs"); err != nil {
		t.Fatal(err)
	}
	body := []byte("hello")
	if err := q.Enqueue(ctx, "jobs", body, testNS); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}

	msg, err := q.Dequeue(ctx, "jobs", 30*time.Second)
	if err != nil {
		t.Fatalf("Dequeue: %v", err)
	}
	if msg == nil {
		t.Fatal("expected message, got nil")
	}
	if string(msg.Body) != "hello" {
		t.Errorf("body = %q, want %q", msg.Body, "hello")
	}

	if err := q.Ack(ctx, "jobs", msg.ID); err != nil {
		t.Fatalf("Ack: %v", err)
	}
	depth, err := q.Depth(ctx, "jobs")
	if err != nil {
		t.Fatal(err)
	}
	if depth != 0 {
		t.Errorf("depth = %d, want 0", depth)
	}
}

func TestDequeueEmptyReturnsNil(t *testing.T) {
	q := newQueue(t)
	ctx := context.Background()
	if err := q.CreateQueue(ctx, "empty"); err != nil {
		t.Fatal(err)
	}
	msg, err := q.Dequeue(ctx, "empty", 30*time.Second)
	if err != nil {
		t.Fatalf("Dequeue: %v", err)
	}
	if msg != nil {
		t.Errorf("expected nil message from empty queue, got %+v", msg)
	}
}

func TestVisibilityTimeoutRedelivery(t *testing.T) {
	q := newQueue(t)
	ctx := context.Background()
	if err := q.CreateQueue(ctx, "vis"); err != nil {
		t.Fatal(err)
	}
	if err := q.Enqueue(ctx, "vis", []byte("work"), testNS); err != nil {
		t.Fatal(err)
	}

	msg, err := q.Dequeue(ctx, "vis", 10*time.Millisecond)
	if err != nil || msg == nil {
		t.Fatalf("first dequeue: err=%v msg=%v", err, msg)
	}

	time.Sleep(50 * time.Millisecond)

	msg2, err := q.Dequeue(ctx, "vis", 30*time.Second)
	if err != nil {
		t.Fatalf("second dequeue: %v", err)
	}
	if msg2 == nil {
		t.Fatal("expected redelivery after visibility timeout, got nil")
	}
	if msg.ID != msg2.ID {
		t.Errorf("redelivered id = %q, want %q", msg2.ID, msg.ID)
	}
}

func TestAckIdempotent(t *testing.T) {
	q := newQueue(t)
	ctx := context.Background()
	if err := q.CreateQueue(ctx, "q"); err != nil {
		t.Fatal(err)
	}
	if err := q.Enqueue(ctx, "q", []byte("x"), testNS); err != nil {
		t.Fatal(err)
	}
	msg, _ := q.Dequeue(ctx, "q", time.Minute)
	if err := q.Ack(ctx, "q", msg.ID); err != nil {
		t.Fatal(err)
	}
	if err := q.Ack(ctx, "q", msg.ID); err != nil {
		t.Fatalf("second Ack: %v", err)
	}
}

func TestQuotaEnforcement(t *testing.T) {
	q := newQueue(t)
	ctx := context.Background()
	if err := q.CreateQueue(ctx, "q"); err != nil {
		t.Fatal(err)
	}
	if err := q.SetQuota(ctx, 10); err != nil {
		t.Fatal(err)
	}

	if err := q.Enqueue(ctx, "q", []byte("0123456789"), testNS); err != nil {
		t.Fatalf("enqueue at limit: %v", err)
	}
	err := q.Enqueue(ctx, "q", []byte("x"), testNS)
	if !errors.Is(err, queue.ErrQuotaExceeded) {
		t.Fatalf("got %v, want ErrQuotaExceeded", err)
	}
}

func TestExternalEnqueueUpdatesIdleFlag(t *testing.T) {
	q := newQueue(t)
	ctx := context.Background()
	if err := q.CreateQueue(ctx, "jobs"); err != nil {
		t.Fatal(err)
	}

	// Self-enqueue: senderID == namespace → should NOT update last_external_enqueue_at.
	if err := q.Enqueue(ctx, "jobs", []byte("self"), testNS); err != nil {
		t.Fatal(err)
	}
	infos, err := q.ListQueues(ctx, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if len(infos) == 0 || !infos[0].Idle {
		t.Error("expected queue to be idle after self-enqueue")
	}

	// External enqueue: senderID != namespace → should mark as not idle.
	if err := q.Enqueue(ctx, "jobs", []byte("ext"), "sender-ns"); err != nil {
		t.Fatal(err)
	}
	infos, err = q.ListQueues(ctx, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if len(infos) == 0 || infos[0].Idle {
		t.Error("expected queue to be not idle after external enqueue")
	}
}

func TestListQueues(t *testing.T) {
	q := newQueue(t)
	ctx := context.Background()
	for _, name := range []string{"a", "b", "c"} {
		if err := q.CreateQueue(ctx, name); err != nil {
			t.Fatal(err)
		}
	}
	if err := q.Enqueue(ctx, "b", []byte("msg"), testNS); err != nil {
		t.Fatal(err)
	}

	infos, err := q.ListQueues(ctx, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if len(infos) != 3 {
		t.Fatalf("got %d queues, want 3", len(infos))
	}
	byName := make(map[string]queue.Info)
	for _, info := range infos {
		byName[info.Name] = info
	}
	if byName["b"].Depth != 1 {
		t.Errorf("queue b depth = %d, want 1", byName["b"].Depth)
	}
	if byName["a"].Depth != 0 {
		t.Errorf("queue a depth = %d, want 0", byName["a"].Depth)
	}
}

func TestUsageTracking(t *testing.T) {
	q := newQueue(t)
	ctx := context.Background()
	if err := q.CreateQueue(ctx, "q"); err != nil {
		t.Fatal(err)
	}

	body := []byte("hello world")
	if err := q.Enqueue(ctx, "q", body, testNS); err != nil {
		t.Fatal(err)
	}
	usage, err := q.Usage(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if usage != int64(len(body)) {
		t.Errorf("usage = %d, want %d", usage, len(body))
	}

	msg, _ := q.Dequeue(ctx, "q", time.Minute)
	if err := q.Ack(ctx, "q", msg.ID); err != nil {
		t.Fatal(err)
	}
	usage, err = q.Usage(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if usage != 0 {
		t.Errorf("usage after ack = %d, want 0", usage)
	}
}
