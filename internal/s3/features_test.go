package s3

import (
	"context"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/containeroo/garage-webui/internal/objectaction"
	"github.com/containeroo/garage-webui/internal/objectpath"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestInvalidMultipartDoesNotProvision checks validation at the service boundary.
func TestInvalidMultipartDoesNotProvision(t *testing.T) {
	t.Parallel()

	t.Run("unknown multipart action", func(t *testing.T) {
		t.Parallel()

		manager := unprovisionedManagerForTest(t)
		ctx, now := t.Context(), time.Now()
		_, err := manager.MultipartURL(ctx, "bucket", "alias", "object", "", objectaction.Action("unknown"), 0, now)
		require.Error(t, err)
	})

	t.Run("start rejects upload ID", func(t *testing.T) {
		t.Parallel()

		manager := unprovisionedManagerForTest(t)
		ctx, now := t.Context(), time.Now()
		_, err := manager.MultipartURL(ctx, "bucket", "alias", "object", "unexpected", objectaction.MultipartStart, 0, now)
		require.Error(t, err)
	})

	t.Run("part requires upload ID", func(t *testing.T) {
		t.Parallel()

		manager := unprovisionedManagerForTest(t)
		ctx, now := t.Context(), time.Now()
		_, err := manager.MultipartURL(ctx, "bucket", "alias", "object", "", objectaction.MultipartPart, 1, now)
		require.Error(t, err)
	})

	t.Run("part rejects excessive part number", func(t *testing.T) {
		t.Parallel()

		manager := unprovisionedManagerForTest(t)
		ctx, now := t.Context(), time.Now()
		_, err := manager.MultipartURL(ctx, "bucket", "alias", "object", "upload", objectaction.MultipartPart, 10001, now)
		require.Error(t, err)
	})

	t.Run("completion rejects part number", func(t *testing.T) {
		t.Parallel()

		manager := unprovisionedManagerForTest(t)
		ctx, now := t.Context(), time.Now()
		_, err := manager.MultipartURL(ctx, "bucket", "alias", "object", "upload", objectaction.MultipartComplete, 1, now)
		require.Error(t, err)
	})

	t.Run("unknown multipart copy action", func(t *testing.T) {
		t.Parallel()

		manager := unprovisionedManagerForTest(t)
		ctx, now := t.Context(), time.Now()
		_, _, err := manager.CopyMultipartURL(ctx, "bucket", "alias", "source", "target", "", objectaction.Action("unknown"), 0, 0, 0, now)
		require.Error(t, err)
	})

	t.Run("copy start rejects upload ID", func(t *testing.T) {
		t.Parallel()

		manager := unprovisionedManagerForTest(t)
		ctx, now := t.Context(), time.Now()
		_, _, err := manager.CopyMultipartURL(ctx, "bucket", "alias", "source", "target", "unexpected", objectaction.MultipartCopyStart, 0, 0, 0, now)
		require.Error(t, err)
	})

	t.Run("copy part rejects reversed range", func(t *testing.T) {
		t.Parallel()

		manager := unprovisionedManagerForTest(t)
		ctx, now := t.Context(), time.Now()
		_, _, err := manager.CopyMultipartURL(ctx, "bucket", "alias", "source", "target", "upload", objectaction.MultipartCopyPart, 1, 5, 4, now)
		require.Error(t, err)
	})

	t.Run("copy part rejects excessive range", func(t *testing.T) {
		t.Parallel()

		manager := unprovisionedManagerForTest(t)
		ctx, now := t.Context(), time.Now()
		_, _, err := manager.CopyMultipartURL(ctx, "bucket", "alias", "source", "target", "upload", objectaction.MultipartCopyPart, 1, 0, MaxUploadSize, now)
		require.Error(t, err)
	})

	t.Run("copy completion rejects part number", func(t *testing.T) {
		t.Parallel()

		manager := unprovisionedManagerForTest(t)
		ctx, now := t.Context(), time.Now()
		_, _, err := manager.CopyMultipartURL(ctx, "bucket", "alias", "source", "target", "upload", objectaction.MultipartCopyComplete, 2, 0, 0, now)
		require.Error(t, err)
	})

}

// unprovisionedManagerForTest verifies rejected requests never provision credentials or CORS.
func unprovisionedManagerForTest(t *testing.T) *Manager {
	t.Helper()
	state := newFakeGarage()
	store := &memoryKeyStore{}
	t.Cleanup(func() {
		assert.Zero(t, state.createCount)
		assert.Zero(t, state.allowCount)
		assert.Empty(t, state.cors)
		assert.Empty(t, store.ids)
		assert.Empty(t, store.copyIDs)
	})
	return managerForTest(t, state, store)
}

// TestMultipartAndCopySigning checks S3 operation scoping and signed copy headers.
func TestMultipartAndCopySigning(t *testing.T) {
	t.Parallel()

	signer, err := New("http://localhost:3900", "garage", "GKtest", "a-secret", "http://localhost:8080")
	require.NoError(t, err)

	now := time.Date(2026, 10, 17, 18, 0, 0, 0, time.UTC)
	begin, err := signer.BeginMultipartURL("bucket", "folder/archive.tar", now)
	require.NoError(t, err)

	u, err := url.Parse(begin)
	require.NoError(t, err)

	{
		_, ok := u.Query()["uploads"]
		require.True(t, ok, "missing multipart start parameter")
	}

	assert.Equal(t, "300", u.Query().Get("X-Amz-Expires"), "wrong URL lifetime")

	partURL, err := signer.PartURL("bucket", "folder/archive.tar", "opaque=+id", 10000, now)
	require.NoError(t, err)

	parsed, err := url.Parse(partURL)
	require.NoError(t, err)

	assert.Equal(t, "10000", parsed.Query().Get("partNumber"), "incorrect part scope")
	assert.Equal(t, "opaque=+id", parsed.Query().Get("uploadId"), "incorrect part scope")

	{
		_, err := signer.PartURL("bucket", "archive", "id", 10001, now)
		require.Error(t, err, "accepted too many parts")
	}

	copyURL, header, err := signer.CopyURL("bucket", "folder/source #1.txt", "folder/target.txt", now)
	require.NoError(t, err)

	parsed, err = url.Parse(copyURL)
	require.NoError(t, err)

	assert.Equal(t, "host;x-amz-copy-source", parsed.Query().Get("X-Amz-SignedHeaders"), "copy source header not signed")

	assert.Contains(t, header, "source%20%231.txt", "copy source not escaped: %s", header)

	partCopyURL, headers, err := signer.CopyPartURL("bucket", "old", "new", "upload", 1, 0, 16<<20-1, now)
	require.NoError(t, err)

	parsed, err = url.Parse(partCopyURL)
	require.NoError(t, err)

	assert.Equal(t, "host;x-amz-copy-source;x-amz-copy-source-range", parsed.Query().Get("X-Amz-SignedHeaders"), "missing signed copy range")

	assert.Equal(t, "bytes=0-16777215", headers["x-amz-copy-source-range"], "incorrect signed copy range")

	{
		_, _, err := signer.CopyPartURL("bucket", "old", "new", "upload", 1, 2, 1, now)
		require.Error(t, err, "accepted reversed range")
	}
}

// TestSeparateCopyCredential checks that copies never reuse read-only or write-only keys.
func TestSeparateCopyCredential(t *testing.T) {
	t.Parallel()

	manager := managerForTest(t, newFakeGarage(), &memoryKeyStore{})
	ctx, now := context.Background(), time.Now()
	read, err := manager.PreviewURL(ctx, "bucket-a", "alias", "item.txt", now)
	require.NoError(t, err)

	write, err := manager.UploadURL(ctx, "bucket-a", "alias", "item.txt", now)
	require.NoError(t, err)

	copyURL, _, err := manager.CopyURL(ctx, "bucket-a", "alias", "item.txt", "item-2.txt", now)
	require.NoError(t, err)

	credential := func(address string) string {
		parsed, err := url.Parse(address)
		require.NoError(t, err)

		return strings.Split(parsed.Query().Get("X-Amz-Credential"), "/")[0]
	}
	assert.NotEqual(t, credential(write), credential(read), "privileges are not isolated across the three per-bucket keys")
	assert.NotEqual(t, credential(read), credential(copyURL), "privileges are not isolated across the three per-bucket keys")
	assert.NotEqual(t, credential(write), credential(copyURL), "privileges are not isolated across the three per-bucket keys")
}

// TestListPrefixValidation covers folder prefixes and traversal rejection.
func TestListPrefixValidation(t *testing.T) {
	t.Parallel()

	t.Run("empty prefix", func(t *testing.T) {
		t.Parallel()

		prefix := ""
		require.True(t, objectpath.ValidPrefix(prefix), "rejected valid prefix %q", prefix)
	})

	t.Run("pictures/", func(t *testing.T) {
		t.Parallel()

		prefix := "pictures/"
		require.True(t, objectpath.ValidPrefix(prefix), "rejected valid prefix %q", prefix)
	})

	t.Run("pictures/cats", func(t *testing.T) {
		t.Parallel()

		prefix := "pictures/cats"
		require.True(t, objectpath.ValidPrefix(prefix), "rejected valid prefix %q", prefix)
	})

	t.Run("météo/", func(t *testing.T) {
		t.Parallel()

		prefix := "météo/"
		require.True(t, objectpath.ValidPrefix(prefix), "rejected valid prefix %q", prefix)
	})

	t.Run("/root", func(t *testing.T) {
		t.Parallel()

		prefix := "/root"
		assert.False(t, objectpath.ValidPrefix(prefix), "accepted unsafe prefix %q", prefix)
	})

	t.Run("../escape", func(t *testing.T) {
		t.Parallel()

		prefix := "../escape"
		assert.False(t, objectpath.ValidPrefix(prefix), "accepted unsafe prefix %q", prefix)
	})

	t.Run("foo/../bar", func(t *testing.T) {
		t.Parallel()

		prefix := "foo/../bar"
		assert.False(t, objectpath.ValidPrefix(prefix), "accepted unsafe prefix %q", prefix)
	})

	t.Run("foo\\bar", func(t *testing.T) {
		t.Parallel()

		prefix := "foo\\bar"
		assert.False(t, objectpath.ValidPrefix(prefix), "accepted unsafe prefix %q", prefix)
	})

	t.Run("bad\nkey", func(t *testing.T) {
		t.Parallel()

		prefix := "bad\nkey"
		assert.False(t, objectpath.ValidPrefix(prefix), "accepted unsafe prefix %q", prefix)
	})

	signer, err := New("http://localhost:3900", "garage", "key", "secret", "http://localhost:8080")
	require.NoError(t, err)

	result, err := signer.ListPrefixURL("bucket", "folder/", "", time.Now())
	require.NoError(t, err)

	address, err := url.Parse(result)
	require.NoError(t, err)

	assert.Equal(t, "folder/", address.Query().Get("prefix"), "listing is not folder-scoped")
	assert.Equal(t, "/", address.Query().Get("delimiter"), "listing is not folder-scoped")
}
