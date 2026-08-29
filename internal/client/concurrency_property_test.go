package client

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/hjertmann/youtrack-proxy/internal/config"
	"pgregory.net/rapid"
)

// TestProperty6_SlotReleaseOnAllOutcomes validates Property 6: Concurrency slot
// release on all outcomes.
//
// For any outbound YouTrack call (success, HTTP error, timeout, or panic), the
// concurrency semaphore count after the call SHALL equal the count before the
// call (slot released).
//
// The test spins up a random number of goroutines that each acquire a semaphore
// slot via acquireSemaphore, perform a random action (success, error, or panic),
// and defer the release function. After all goroutines complete, we verify all
// slots are available again.
//
// **Validates: Requirements 5.4, 5.6**
func TestProperty6_SlotReleaseOnAllOutcomes(t *testing.T) {
	const maxConcurrent int64 = 5

	cfg := &config.Config{
		QueueTimeout: 5 * time.Second,
	}

	rapid.Check(t, func(rt *rapid.T) {
		InitConcurrency(maxConcurrent)
		defer func() { ytSemaphore = nil }()

		numGoroutines := rapid.IntRange(10, 50).Draw(rt, "numGoroutines")

		// For each goroutine, draw an outcome: 0=success, 1=error, 2=panic.
		outcomes := make([]int, numGoroutines)
		for i := range outcomes {
			outcomes[i] = rapid.IntRange(0, 2).Draw(rt, "outcome")
		}

		var wg sync.WaitGroup
		wg.Add(numGoroutines)

		for i := 0; i < numGoroutines; i++ {
			go func(outcome int) {
				defer wg.Done()
				// Wrap in a recover so panics don't kill the test.
				defer func() { recover() }()

				release, err := acquireSemaphore(cfg)
				if err != nil {
					// Slot was never acquired — nothing to release.
					return
				}
				defer release()

				switch outcome {
				case 0:
					// Success — do nothing, release fires via defer.
				case 1:
					// Simulate an error path — release still fires via defer.
					_ = ErrQueueTimeout
				case 2:
					// Panic — release fires via defer before recover catches.
					panic("simulated panic")
				}
			}(outcomes[i])
		}

		wg.Wait()

		// All goroutines done. Every acquired slot should be released.
		// Verify by acquiring all maxConcurrent slots without blocking.
		ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
		defer cancel()
		for j := int64(0); j < maxConcurrent; j++ {
			if err := ytSemaphore.Acquire(ctx, 1); err != nil {
				rt.Fatalf("slot %d not released: %v (expected all %d slots available)", j, err, maxConcurrent)
			}
		}
		// Release them so the next iteration starts clean.
		ytSemaphore.Release(maxConcurrent)
	})
}
