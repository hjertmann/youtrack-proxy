package service

import (
	"fmt"
	"sync"
	"testing"
	"time"
)

func TestCacheMiss_CallsFetchFn(t *testing.T) {
	cache := NewResolvedStateCache(time.Hour)
	calls := 0

	result := cache.GetOrFetch("proj-1", func(pid string) (ResolvedStateSet, error) {
		calls++
		if pid != "proj-1" {
			t.Fatalf("fetchFn got pid=%q, want proj-1", pid)
		}
		return ResolvedStateSet{"done": {}, "fixed": {}}, nil
	})

	if calls != 1 {
		t.Fatalf("fetchFn called %d times, want 1", calls)
	}
	if len(result) != 2 {
		t.Fatalf("len = %d, want 2", len(result))
	}
	if _, ok := result["done"]; !ok {
		t.Error("missing 'done'")
	}
}

func TestCacheHit_DoesNotCallFetchFn(t *testing.T) {
	cache := NewResolvedStateCache(time.Hour)
	calls := 0
	fetchFn := func(pid string) (ResolvedStateSet, error) {
		calls++
		return ResolvedStateSet{"done": {}}, nil
	}

	// First call: populates cache.
	cache.GetOrFetch("proj-1", fetchFn)
	if calls != 1 {
		t.Fatalf("expected 1 call after first GetOrFetch, got %d", calls)
	}

	// Second call: should be a cache hit.
	result := cache.GetOrFetch("proj-1", fetchFn)
	if calls != 1 {
		t.Fatalf("expected 1 call total (cache hit), got %d", calls)
	}
	if _, ok := result["done"]; !ok {
		t.Error("missing 'done' from cached result")
	}
}

func TestCacheExpiry_RefetchesAfterTTL(t *testing.T) {
	cache := NewResolvedStateCache(50 * time.Millisecond)
	calls := 0
	fetchFn := func(pid string) (ResolvedStateSet, error) {
		calls++
		return ResolvedStateSet{fmt.Sprintf("state-%d", calls): {}}, nil
	}

	// Populate cache.
	r1 := cache.GetOrFetch("proj-1", fetchFn)
	if _, ok := r1["state-1"]; !ok {
		t.Fatal("expected state-1 from first fetch")
	}

	// Wait for TTL to expire.
	time.Sleep(80 * time.Millisecond)

	// Should trigger a re-fetch.
	r2 := cache.GetOrFetch("proj-1", fetchFn)
	if calls != 2 {
		t.Fatalf("expected 2 calls after expiry, got %d", calls)
	}
	if _, ok := r2["state-2"]; !ok {
		t.Fatal("expected state-2 from second fetch")
	}
}

func TestCacheStaleOnError_ReturnsStalEntry(t *testing.T) {
	cache := NewResolvedStateCache(50 * time.Millisecond)

	// Populate with good data.
	cache.GetOrFetch("proj-1", func(pid string) (ResolvedStateSet, error) {
		return ResolvedStateSet{"done": {}}, nil
	})

	// Wait for expiry.
	time.Sleep(80 * time.Millisecond)

	// Fetch fails — should return stale entry.
	result := cache.GetOrFetch("proj-1", func(pid string) (ResolvedStateSet, error) {
		return nil, fmt.Errorf("network error")
	})

	if _, ok := result["done"]; !ok {
		t.Fatal("expected stale 'done' after fetch failure")
	}
}

func TestCacheFetchError_NoStaleEntry_ReturnsEmpty(t *testing.T) {
	cache := NewResolvedStateCache(time.Hour)

	result := cache.GetOrFetch("unknown-proj", func(pid string) (ResolvedStateSet, error) {
		return nil, fmt.Errorf("not found")
	})

	if len(result) != 0 {
		t.Fatalf("expected empty set on error with no stale entry, got %d entries", len(result))
	}
}

func TestCacheIndependentProjects(t *testing.T) {
	cache := NewResolvedStateCache(time.Hour)

	cache.GetOrFetch("proj-a", func(pid string) (ResolvedStateSet, error) {
		return ResolvedStateSet{"fixed": {}}, nil
	})
	cache.GetOrFetch("proj-b", func(pid string) (ResolvedStateSet, error) {
		return ResolvedStateSet{"closed": {}}, nil
	})

	// Each project has its own entry.
	a := cache.GetOrFetch("proj-a", func(pid string) (ResolvedStateSet, error) {
		t.Fatal("should not re-fetch proj-a")
		return nil, nil
	})
	b := cache.GetOrFetch("proj-b", func(pid string) (ResolvedStateSet, error) {
		t.Fatal("should not re-fetch proj-b")
		return nil, nil
	})

	if _, ok := a["fixed"]; !ok {
		t.Error("proj-a missing 'fixed'")
	}
	if _, ok := b["closed"]; !ok {
		t.Error("proj-b missing 'closed'")
	}
}

func TestCacheConcurrentAccess(t *testing.T) {
	cache := NewResolvedStateCache(time.Hour)
	const goroutines = 50
	var wg sync.WaitGroup
	wg.Add(goroutines)

	for i := 0; i < goroutines; i++ {
		go func(id int) {
			defer wg.Done()
			pid := fmt.Sprintf("proj-%d", id%5) // 5 projects
			result := cache.GetOrFetch(pid, func(p string) (ResolvedStateSet, error) {
				return ResolvedStateSet{"done": {}}, nil
			})
			if _, ok := result["done"]; !ok {
				t.Errorf("goroutine %d: missing 'done' for %s", id, pid)
			}
		}(i)
	}

	wg.Wait()
}
