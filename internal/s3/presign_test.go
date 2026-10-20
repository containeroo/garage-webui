package s3

import (
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/containeroo/garage-webui/internal/objectpath"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestUploadURL verifies the signed request scope and encoding.
func TestUploadURL(t *testing.T) {
	t.Parallel()

	signer, err := New("https://s3.example.com:8443", "garage", "GKtest", "private-secret", "https://web.example.com")
	require.NoError(t, err)
	first, err := signer.UploadURL("uploads", "dir/my file+☃.txt", time.Date(2026, 10, 17, 10, 30, 0, 0, time.UTC))
	require.NoError(t, err)
	u, err := url.Parse(first)
	require.NoError(t, err)
	assert.Equal(t, "s3.example.com:8443", u.Host)
	assert.Equal(t, "/uploads/dir/my file+☃.txt", u.Path)
	assert.Contains(t, u.EscapedPath(), "my%20file+%E2%98%83.txt")
	assert.Equal(t, "AWS4-HMAC-SHA256", u.Query().Get("X-Amz-Algorithm"))
	assert.Equal(t, "GKtest/20261017/garage/s3/aws4_request", u.Query().Get("X-Amz-Credential"))
	assert.Equal(t, "300", u.Query().Get("X-Amz-Expires"))
	assert.Len(t, u.Query().Get("X-Amz-Signature"), 64)
	again, err := signer.UploadURL("uploads", "dir/my file+☃.txt", time.Date(2026, 10, 17, 10, 30, 0, 0, time.UTC))
	require.NoError(t, err)
	assert.Equal(t, first, again)
	assert.NotContains(t, first, "private-secret")
}

// TestNewValidation protects signing configuration from malformed endpoints.
func TestNewValidation(t *testing.T) {
	t.Parallel()

	t.Run("empty endpoint", func(t *testing.T) {
		t.Parallel()

		raw := ""
		_, err := New(raw, "garage", "key", "secret", "https://web.example.com")
		assert.Error(t, err, raw)
	})

	t.Run("https://user@s3.example.com", func(t *testing.T) {
		t.Parallel()

		raw := "https://user@s3.example.com"
		_, err := New(raw, "garage", "key", "secret", "https://web.example.com")
		assert.Error(t, err, raw)
	})

	t.Run("https://s3.example.com/base", func(t *testing.T) {
		t.Parallel()

		raw := "https://s3.example.com/base"
		_, err := New(raw, "garage", "key", "secret", "https://web.example.com")
		assert.Error(t, err, raw)
	})

	t.Run("https://s3.example.com?q=1", func(t *testing.T) {
		t.Parallel()

		raw := "https://s3.example.com?q=1"
		_, err := New(raw, "garage", "key", "secret", "https://web.example.com")
		assert.Error(t, err, raw)
	})

	t.Run("ftp://s3.example.com", func(t *testing.T) {
		t.Parallel()

		raw := "ftp://s3.example.com"
		_, err := New(raw, "garage", "key", "secret", "https://web.example.com")
		assert.Error(t, err, raw)
	})

	t.Run("http://s3.example.com", func(t *testing.T) {
		t.Parallel()

		raw := "http://s3.example.com"
		_, err := New(raw, "garage", "key", "secret", "https://web.example.com")
		assert.Error(t, err, raw)
	})
}

// TestInvalidKeys ensures object keys cannot break URL scoping.
func TestInvalidKeys(t *testing.T) {
	t.Parallel()

	signer, err := New("http://localhost:3900", "garage", "key", "secret", "http://localhost:8080")
	require.NoError(t, err)
	t.Run("empty object key", func(t *testing.T) {
		t.Parallel()

		key := ""
		assert.False(t, objectpath.ValidObjectKey(key), key)
		_, err := signer.UploadURL("bucket", key, time.Now())
		assert.Error(t, err, key)
	})

	t.Run("../bad", func(t *testing.T) {
		t.Parallel()

		key := "../bad"
		assert.False(t, objectpath.ValidObjectKey(key), key)
		_, err := signer.UploadURL("bucket", key, time.Now())
		assert.Error(t, err, key)
	})

	t.Run("dir/../bad", func(t *testing.T) {
		t.Parallel()

		key := "dir/../bad"
		assert.False(t, objectpath.ValidObjectKey(key), key)
		_, err := signer.UploadURL("bucket", key, time.Now())
		assert.Error(t, err, key)
	})

	t.Run("/absolute", func(t *testing.T) {
		t.Parallel()

		key := "/absolute"
		assert.False(t, objectpath.ValidObjectKey(key), key)
		_, err := signer.UploadURL("bucket", key, time.Now())
		assert.Error(t, err, key)
	})

	t.Run("dir//bad", func(t *testing.T) {
		t.Parallel()

		key := "dir//bad"
		assert.False(t, objectpath.ValidObjectKey(key), key)
		_, err := signer.UploadURL("bucket", key, time.Now())
		assert.Error(t, err, key)
	})

	t.Run("bad\\key", func(t *testing.T) {
		t.Parallel()

		key := "bad\\key"
		assert.False(t, objectpath.ValidObjectKey(key), key)
		_, err := signer.UploadURL("bucket", key, time.Now())
		assert.Error(t, err, key)
	})

	t.Run("bad\nkey", func(t *testing.T) {
		t.Parallel()

		key := "bad\nkey"
		assert.False(t, objectpath.ValidObjectKey(key), key)
		_, err := signer.UploadURL("bucket", key, time.Now())
		assert.Error(t, err, key)
	})

	t.Run("oversized object key", func(t *testing.T) {
		t.Parallel()

		key := strings.Repeat("a", 1025)
		assert.False(t, objectpath.ValidObjectKey(key), key)
		_, err := signer.UploadURL("bucket", key, time.Now())
		assert.Error(t, err, key)
	})

	assert.True(t, objectpath.ValidObjectKey("backups/2026-10-17.zip"))
}
