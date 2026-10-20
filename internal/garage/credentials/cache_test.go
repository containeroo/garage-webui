package credentials

import (
	"testing"

	"github.com/containeroo/garage-webui/internal/signing"
)

// TestCacheScopesAndInvalidation verifies secrets never cross token or bucket boundaries.
func TestCacheScopesAndInvalidation(t *testing.T) {
	c := NewCache()
	key := signing.Credential{AccessKeyID: "first", SecretAccessKey: "secret"}
	c.Put("admin-a", "bucket-a", "upload", key)
	for _, item := range []struct{ token, bucket, purpose string }{
		{"admin-b", "bucket-a", "upload"},
		{"admin-a", "bucket-b", "upload"},
		{"admin-a", "bucket-a", "read"},
	} {
		if _, ok := c.Get(item.token, item.bucket, item.purpose); ok {
			t.Fatalf("cache leaked key to %+v", item)
		}
	}
	if got, ok := c.Get("admin-a", "bucket-a", "upload"); !ok || got.AccessKeyID != key.AccessKeyID {
		t.Fatal("signing cache did not reuse the proper key")
	}
	c.InvalidateBucket("bucket-a")
	if _, ok := c.Get("admin-a", "bucket-a", "upload"); ok {
		t.Fatal("deleted bucket key is still cached")
	}
}

// TestProviderRequiresCache makes the constructor contract explicit.
func TestProviderRequiresCache(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("provider accepted a nil signing cache")
		}
	}()

	New(nil, nil, "http://localhost:8080", nil, "admin-secret")
}
