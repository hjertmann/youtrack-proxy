package service

import (
	"sync"
	"time"

	"github.com/rs/zerolog/log"
)

type cacheEntry struct {
	states    ResolvedStateSet
	fetchedAt time.Time
}

// ResolvedStateCache is an in-memory, concurrency-safe, per-project cache
// for ResolvedStateSets with a configurable TTL.
type ResolvedStateCache struct {
	mu      sync.RWMutex
	entries map[string]cacheEntry
	ttl     time.Duration
}

// NewResolvedStateCache creates a cache that expires entries after ttl.
func NewResolvedStateCache(ttl time.Duration) *ResolvedStateCache {
	return &ResolvedStateCache{
		entries: make(map[string]cacheEntry),
		ttl:     ttl,
	}
}

// GetOrFetch returns the cached ResolvedStateSet for projectID, fetching via
// fetchFn on cache miss or expiry. Never returns an error to callers — fetch
// failures degrade to stale cache or empty set.
func (c *ResolvedStateCache) GetOrFetch(
	projectID string,
	fetchFn func(string) (ResolvedStateSet, error),
) ResolvedStateSet {
	// Fast path: read lock, check for fresh entry.
	c.mu.RLock()
	if entry, ok := c.entries[projectID]; ok && time.Since(entry.fetchedAt) < c.ttl {
		c.mu.RUnlock()
		return entry.states
	}
	c.mu.RUnlock()

	// Slow path: write lock, double-check, fetch on miss/expiry.
	c.mu.Lock()
	defer c.mu.Unlock()

	// Double-check: another goroutine may have refreshed while we waited.
	if entry, ok := c.entries[projectID]; ok && time.Since(entry.fetchedAt) < c.ttl {
		return entry.states
	}

	states, err := fetchFn(projectID)
	if err != nil {
		// Stale-on-error: return expired entry if available.
		if entry, ok := c.entries[projectID]; ok {
			log.Warn().Err(err).Str("projectID", projectID).
				Msg("Failed to refresh resolved states, using stale cache")
			return entry.states
		}
		// No cached entry at all: log and return empty set.
		log.Warn().Err(err).Str("projectID", projectID).
			Msg("Failed to fetch resolved states, treating as empty")
		return ResolvedStateSet{}
	}

	c.entries[projectID] = cacheEntry{states: states, fetchedAt: time.Now()}
	return states
}
