package credentials

import (
	"crypto/sha256"
	"sync"
	"time"

	"github.com/containeroo/garage-webui/internal/signing"
)

// Cache holds briefly verified Garage signing credentials in memory.
// Entries are isolated by admin token, bucket and purpose, and never persisted.
type Cache struct {
	// mu protects concurrent access to entries.
	mu sync.Mutex

	// entries stores signing credentials indexed by principal, bucket and purpose.
	entries map[cacheKey]cacheEntry
}

// cacheKey uniquely identifies a signing credential within the cache.
type cacheKey struct {
	// principal is the SHA-256 hash of the Garage admin token.
	principal [32]byte

	// bucket identifies the Garage bucket associated with the credential.
	bucket string

	// purpose identifies the signing operation the credential permits.
	purpose string
}

// cacheEntry holds a signing credential and its expiration time.
type cacheEntry struct {
	// credential contains the Garage S3 access key and secret used for signing.
	credential signing.Credential

	// expires defines when the cached credential becomes invalid.
	expires time.Time
}

const (
	cacheLifetime   = 20 * time.Second
	maxCacheEntries = 512
)

// NewCache creates a bounded, concurrency-safe per-process signing cache.
func NewCache() *Cache {
	return &Cache{
		entries: make(map[cacheKey]cacheEntry),
	}
}

// principalKey scopes cached keys to the exact Garage token used to provision them.
func principalKey(token, bucket, purpose string) cacheKey {
	return cacheKey{
		principal: sha256.Sum256([]byte(token)),
		bucket:    bucket,
		purpose:   purpose,
	}
}

// Get returns a recently provisioned credential without querying Garage again.
func (c *Cache) Get(token, bucket, purpose string) (signing.Credential, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	entry, ok := c.entries[principalKey(token, bucket, purpose)]
	if !ok || time.Now().After(entry.expires) || !usableKey(entry.credential) {
		return signing.Credential{}, false
	}

	return entry.credential, true
}

// Put stores a usable signing key for a short interval.
func (c *Cache) Put(token, bucket, purpose string, credential signing.Credential) {
	if !usableKey(credential) {
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if len(c.entries) >= maxCacheEntries {
		// Expired entries are discarded, then the cache is emptied if still full.
		now := time.Now()
		for key, item := range c.entries {
			if !item.expires.After(now) {
				delete(c.entries, key)
			}
		}

		if len(c.entries) >= maxCacheEntries {
			clear(c.entries)
		}
	}

	c.entries[principalKey(token, bucket, purpose)] = cacheEntry{
		credential: credential,
		expires:    time.Now().Add(cacheLifetime),
	}
}

// InvalidateBucket discards keys when Garage bucket permissions or keys change.
func (c *Cache) InvalidateBucket(bucket string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	for key := range c.entries {
		if key.bucket == bucket {
			delete(c.entries, key)
		}
	}
}
