package client

import (
	"context"
	"testing"
	"time"

	"github.com/hjertmann/youtrack-proxy/internal/config"
)

// TestConcurrencyLimiter_BlocksAndTimesOut verifies that when all concurrency
// slots are held, a new acquire times out, and that releasing a slot allows a
// subsequent acquire to succeed.
func TestConcurrencyLimiter_BlocksAndTimesOut(t *testing.T) {
	InitConcurrency(2)
	defer func() { ytSemaphore = nil }()

	// Acquire both available slots.
	if err := ytSemaphore.Acquire(context.Background(), 1); err != nil {
		t.Fatalf("slot 1: %v", err)
	}
	if err := ytSemaphore.Acquire(context.Background(), 1); err != nil {
		t.Fatalf("slot 2: %v", err)
	}

	// Third acquire should time out.
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if err := ytSemaphore.Acquire(ctx, 1); err == nil {
		t.Fatal("expected timeout error when all slots held, got nil")
	}

	// Release one slot.
	ytSemaphore.Release(1)

	// Now a third acquire should succeed.
	ctx2, cancel2 := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel2()
	if err := ytSemaphore.Acquire(ctx2, 1); err != nil {
		t.Fatalf("expected success after release, got: %v", err)
	}

	// Clean up: release the 2 held slots.
	ytSemaphore.Release(2)
}

// TestAcquireSemaphore_ReturnsErrQueueTimeout verifies that the acquireSemaphore
// helper returns ErrQueueTimeout when the semaphore is saturated.
func TestAcquireSemaphore_ReturnsErrQueueTimeout(t *testing.T) {
	InitConcurrency(1)
	defer func() { ytSemaphore = nil }()

	// Saturate the single slot.
	if err := ytSemaphore.Acquire(context.Background(), 1); err != nil {
		t.Fatalf("initial acquire: %v", err)
	}
	defer ytSemaphore.Release(1)

	// acquireSemaphore should fail with ErrQueueTimeout using a short timeout.
	_, err := acquireSemaphore(cfgShortTimeout)
	if err != ErrQueueTimeout {
		t.Fatalf("err = %v, want ErrQueueTimeout", err)
	}
}

// TestAcquireSemaphore_NilSkips verifies that acquireSemaphore is a no-op when
// InitConcurrency has not been called (backward compat).
func TestAcquireSemaphore_NilSkips(t *testing.T) {
	old := ytSemaphore
	ytSemaphore = nil
	defer func() { ytSemaphore = old }()

	release, err := acquireSemaphore(cfgShortTimeout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	release() // should not panic
}

// TestAcquireSemaphore_ReleaseFunctionWorks verifies the release function
// returned by acquireSemaphore actually frees the slot.
func TestAcquireSemaphore_ReleaseFunctionWorks(t *testing.T) {
	InitConcurrency(1)
	defer func() { ytSemaphore = nil }()

	release, err := acquireSemaphore(cfgShortTimeout)
	if err != nil {
		t.Fatalf("first acquire: %v", err)
	}

	// Slot is held — second acquire should fail.
	_, err = acquireSemaphore(cfgShortTimeout)
	if err != ErrQueueTimeout {
		t.Fatalf("expected ErrQueueTimeout while slot held, got: %v", err)
	}

	// Release and retry — should succeed.
	release()

	release2, err := acquireSemaphore(cfgShortTimeout)
	if err != nil {
		t.Fatalf("acquire after release: %v", err)
	}
	release2()
}

// cfgShortTimeout is a minimal config with a very short queue timeout for tests.
var cfgShortTimeout = &config.Config{
	QueueTimeout: 50 * time.Millisecond,
}
