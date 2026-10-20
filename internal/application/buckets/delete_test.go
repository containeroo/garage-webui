package buckets

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/containeroo/garage-webui/internal/garage/admin"
	"github.com/containeroo/garage-webui/internal/signing"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestBucketUsagePreflight rejects nonempty Garage buckets.
func TestBucketUsagePreflight(t *testing.T) {
	t.Parallel()

	t.Run("finished objects", func(t *testing.T) {
		t.Parallel()

		require.True(t, (admin.BucketInfo{Objects: 1}).HasObjects())
	})
	t.Run("unfinished multipart uploads", func(t *testing.T) {
		t.Parallel()

		require.True(t, (admin.BucketInfo{UnfinishedMultipartUploads: 1}).HasObjects())
	})
	t.Run("empty bucket", func(t *testing.T) {
		t.Parallel()

		assert.False(t, (admin.BucketInfo{}).HasObjects())
	})
}

// deletionStore tracks managed Garage keys while a test holds an explicit lock.
type deletionStore struct {
	mu   sync.Mutex
	keys []string
}

type deletionLock struct {
	store *deletionStore
	once  sync.Once
}

// LockBucket exclusively reserves the simulated bucket for the caller.
func (s *deletionStore) LockBucket(_ context.Context, _ string) (signing.BucketLock, error) {
	s.mu.Lock()
	return &deletionLock{store: s}, nil
}

// KeyID is unused by deletion, but is part of the lock contract.
func (l *deletionLock) KeyID(context.Context, signing.Purpose) (string, error) { return "", nil }

// SaveKeyID is unused by deletion, but is part of the lock contract.
func (l *deletionLock) SaveKeyID(context.Context, signing.Purpose, string) error { return nil }

// KeyIDs exposes the identifiers awaiting Garage cleanup.
func (l *deletionLock) KeyIDs(context.Context) ([]string, error) {
	return append([]string(nil), l.store.keys...), nil
}

// DeleteKeyReferences clears references only after all Garage operations succeed.
func (l *deletionLock) DeleteKeyReferences(context.Context) error {
	l.store.keys = nil
	return nil
}

// Close releases the simulated advisory lock exactly once.
func (l *deletionLock) Close() error {
	l.once.Do(l.store.mu.Unlock)
	return nil
}

// TestDeleteBucketRecovery keeps key references when deletion is interrupted.
func TestDeleteBucketRecovery(t *testing.T) {
	t.Parallel()

	bucketExists := true
	failKeyDelete := true
	var deletedKeys []string
	var deleteBucketCalls int
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v2/GetBucketInfo":
			if !bucketExists {
				http.NotFound(w, r)
				return
			}
			_, _ = io.WriteString(w, `{"id":"bucket","objects":0,"unfinishedUploads":0,"unfinishedMultipartUploads":0}`)
		case "/v2/DeleteBucket":
			deleteBucketCalls++
			bucketExists = false
			_, _ = io.WriteString(w, `{}`)
		case "/v2/DeleteKey":
			if failKeyDelete {
				w.WriteHeader(http.StatusServiceUnavailable)
				return
			}
			deletedKeys = append(deletedKeys, r.URL.Query().Get("id"))
			_, _ = io.WriteString(w, `{}`)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(api.Close)

	store := &deletionStore{keys: []string{"managed-read", "managed-write"}}
	client, clientErr := admin.New(api.URL, "test")
	require.NoError(t, clientErr)
	service := New(store)

	status, _, err := service.Delete(context.Background(), client, "bucket")
	require.Error(t, err)
	assert.Equal(t, http.StatusOK, status)
	assert.Equal(t, []string{"managed-read", "managed-write"}, store.keys)

	failKeyDelete = false
	status, _, err = service.Delete(context.Background(), client, "bucket")
	require.NoError(t, err)
	assert.Equal(t, http.StatusNoContent, status)
	assert.Empty(t, store.keys)
	assert.Equal(t, 1, deleteBucketCalls)
	assert.Len(t, deletedKeys, 2)
}

// TestDeleteBucketFailureDoesNotRevokeKeys preserves usable credentials on failure.
func TestDeleteBucketFailureDoesNotRevokeKeys(t *testing.T) {
	t.Parallel()

	t.Run("upstream deletion conflict", func(t *testing.T) {
		t.Parallel()

		failWithObjects := false
		var keysRevoked bool
		api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/v2/GetBucketInfo":
				objects := 0
				if failWithObjects {
					objects = 1
				}
				_, _ = fmt.Fprintf(w, `{"id":"bucket","objects":%d}`, objects)
			case "/v2/DeleteBucket":
				w.WriteHeader(http.StatusConflict)
			case "/v2/DeleteKey":
				keysRevoked = true
			}
		}))
		store := &deletionStore{keys: []string{"managed-key"}}
		client, clientErr := admin.New(api.URL, "test")
		require.NoError(t, clientErr)
		service := New(store)
		status, _, err := service.Delete(context.Background(), client, "bucket")
		api.Close()
		require.Error(t, err)
		assert.Equal(t, http.StatusConflict, status)
		assert.False(t, keysRevoked, "failed deletion must not revoke keys")
		assert.Equal(t, []string{"managed-key"}, store.keys)
	})

	t.Run("nonempty bucket", func(t *testing.T) {
		t.Parallel()

		failWithObjects := true
		var keysRevoked bool
		api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/v2/GetBucketInfo":
				objects := 0
				if failWithObjects {
					objects = 1
				}
				_, _ = fmt.Fprintf(w, `{"id":"bucket","objects":%d}`, objects)
			case "/v2/DeleteBucket":
				w.WriteHeader(http.StatusConflict)
			case "/v2/DeleteKey":
				keysRevoked = true
			}
		}))
		store := &deletionStore{keys: []string{"managed-key"}}
		client, clientErr := admin.New(api.URL, "test")
		require.NoError(t, clientErr)
		service := New(store)
		status, _, err := service.Delete(context.Background(), client, "bucket")
		api.Close()
		require.Error(t, err)
		assert.Equal(t, http.StatusConflict, status)
		assert.False(t, keysRevoked, "failed deletion must not revoke keys")
		assert.Equal(t, []string{"managed-key"}, store.keys)
	})
}
