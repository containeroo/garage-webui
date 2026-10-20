package credentials

import (
	"testing"

	"github.com/containeroo/garage-webui/internal/signing"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCacheScopesAndInvalidation verifies secrets never cross token or bucket boundaries.
func TestCacheScopesAndInvalidation(t *testing.T) {
	t.Parallel()

	t.Run("different admin token", func(t *testing.T) {
		t.Parallel()

		c := populatedCacheForTest()
		_, ok := c.Get("admin-b", "bucket-a", string(signing.Write))
		assert.False(t, ok, "cache must not cross credential boundaries")
	})

	t.Run("different bucket", func(t *testing.T) {
		t.Parallel()

		c := populatedCacheForTest()
		_, ok := c.Get("admin-a", "bucket-b", string(signing.Write))
		assert.False(t, ok, "cache must not cross credential boundaries")
	})

	t.Run("different purpose", func(t *testing.T) {
		t.Parallel()

		c := populatedCacheForTest()
		_, ok := c.Get("admin-a", "bucket-a", string(signing.Read))
		assert.False(t, ok, "cache must not cross credential boundaries")
	})
	t.Run("reuse matching credential", func(t *testing.T) {
		t.Parallel()

		c := populatedCacheForTest()
		got, ok := c.Get("admin-a", "bucket-a", string(signing.Write))
		require.True(t, ok, "signing cache did not reuse the proper key")
		assert.Equal(t, "first", got.AccessKeyID)
	})
	t.Run("invalidate bucket", func(t *testing.T) {
		t.Parallel()

		c := populatedCacheForTest()
		c.InvalidateBucket("bucket-a")
		_, ok := c.Get("admin-a", "bucket-a", string(signing.Write))
		assert.False(t, ok, "deleted bucket key is still cached")
	})
}

// populatedCacheForTest creates an isolated cache containing one signing credential.
func populatedCacheForTest() *Cache {
	c := NewCache()
	c.Put("admin-a", "bucket-a", string(signing.Write), signing.Credential{AccessKeyID: "first", SecretAccessKey: "secret"})
	return c
}

// TestProviderRequiresCache makes the constructor contract explicit.
func TestProviderRequiresCache(t *testing.T) {
	t.Parallel()

	require.Panics(t, func() {
		New(nil, nil, "http://localhost:8080", nil, "admin-secret")
	})
}
