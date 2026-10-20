package s3

import (
	"context"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestObjectOperations checks scope, pagination parameters and isolated credentials.
func TestObjectOperations(t *testing.T) {
	state := newFakeGarage()
	store := &memoryKeyStore{}
	manager := managerForTest(t, state, store)
	now := time.Date(2026, 10, 17, 18, 0, 0, 0, time.UTC)

	listing, err := manager.ListURL(context.Background(), "bucket-a", "alias-a", "opaque/+token=", now)
	require.NoError(t, err)
	list, err := url.Parse(listing)
	require.NoError(t, err)
	assert.Equal(t, "/alias-a", list.Path)
	assert.Equal(t, "2", list.Query().Get("list-type"))
	assert.Equal(t, "url", list.Query().Get("encoding-type"))
	assert.Equal(t, "100", list.Query().Get("max-keys"))
	assert.Equal(t, "opaque/+token=", list.Query().Get("continuation-token"))
	assert.NotEmpty(t, list.Query().Get("X-Amz-Signature"))

	download, err := manager.DownloadURL(context.Background(), "bucket-a", "alias-a", "folder/report 2026+📊.pdf", now)
	require.NoError(t, err)
	got, err := url.Parse(download)
	require.NoError(t, err)
	assert.Equal(t, "/alias-a/folder/report 2026+📊.pdf", got.Path)
	assert.Contains(t, got.Query().Get("response-content-disposition"), "attachment;")
	assert.Contains(t, got.Query().Get("response-content-disposition"), "report%202026%2B")
	assert.Equal(t, list.Query().Get("X-Amz-Credential"), got.Query().Get("X-Amz-Credential"))

	deletion, err := manager.DeleteURL(context.Background(), "bucket-a", "alias-a", "folder/report.pdf", now)
	require.NoError(t, err)
	del, err := url.Parse(deletion)
	require.NoError(t, err)
	assert.Equal(t, "/alias-a/folder/report.pdf", del.Path)
	assert.NotEqual(t, del.Query().Get("X-Amz-Credential"), got.Query().Get("X-Amz-Credential"))

	upload, err := manager.UploadURL(context.Background(), "bucket-a", "alias-a", "folder/new.pdf", now)
	require.NoError(t, err)
	put, err := url.Parse(upload)
	require.NoError(t, err)
	assert.Equal(t, del.Query().Get("X-Amz-Credential"), put.Query().Get("X-Amz-Credential"))
	assert.Equal(t, 2, state.createCount)
	assert.Equal(t, 1, len(state.cors["bucket-a"])) // shared GET/PUT/DELETE rule
}

// TestReadKeyReuseAcrossBuckets checks dedicated read-only keys and concurrency.
func TestReadKeyReuseAcrossBuckets(t *testing.T) {
	state := newFakeGarage()
	store := &memoryKeyStore{}
	manager := managerForTest(t, state, store)
	var wg sync.WaitGroup
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			bucket := "one"
			if i%2 == 0 {
				bucket = "two"
			}

			_, err := manager.ListURL(context.Background(), bucket, bucket, "", time.Now())
			assert.NoError(t, err)
		}(i)
	}

	wg.Wait()
	assert.Len(t, store.readIDs, 2)
	assert.NotEqual(t, store.readIDs["one"], store.readIDs["two"])
	assert.Empty(t, store.ids)
	assert.Equal(t, 2, state.createCount)
	// Concurrent cache misses may all reach the lock before the first key is cached.
	// Each permission update must still use the correct bucket-scoped key.
	assert.GreaterOrEqual(t, state.allowCount, 2)
	assert.LessOrEqual(t, state.allowCount, 12)
	assert.Equal(t, state.allowCount, state.denyCount)
}

// TestObjectURLValidation rejects oversized tokens and malformed target keys.
func TestObjectURLValidation(t *testing.T) {
	signer, err := New("http://localhost:3900", "garage", "key", "secret", "http://localhost:8080")
	require.NoError(t, err)
	_, err = signer.ListURL("bucket", strings.Repeat("x", 4097), time.Now())
	require.Error(t, err)
	_, err = signer.DeleteURL("bucket", "../bad", time.Now())
	require.Error(t, err)
	_, err = signer.DownloadURL("bucket", "", time.Now())
	require.Error(t, err)
}
