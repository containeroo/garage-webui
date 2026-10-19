package buckets

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sync"
	"testing"

	"github.com/containeroo/garage-webui/internal/garage/admin"
	"github.com/containeroo/garage-webui/internal/signing"
)

// TestBucketUsagePreflight rejects nonempty Garage buckets.
func TestBucketUsagePreflight(t *testing.T) {
	if !(admin.BucketInfo{Objects: 1}).HasObjects() || !(admin.BucketInfo{UnfinishedMultipartUploads: 1}).HasObjects() {
		t.Fatal("nonempty bucket was accepted")
	}
	if (admin.BucketInfo{}).HasObjects() {
		t.Fatal("empty bucket was rejected")
	}
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
	if clientErr != nil {
		t.Fatal(clientErr)
	}
	service := New(store)

	status, _, err := service.Delete(context.Background(), client, "bucket")
	if err == nil || status != http.StatusOK {
		t.Fatalf("expected incomplete key cleanup after bucket deletion, got status=%d err=%v", status, err)
	}
	if !reflect.DeepEqual(store.keys, []string{"managed-read", "managed-write"}) {
		t.Fatalf("lost key references needed to resume deletion: %v", store.keys)
	}

	failKeyDelete = false
	status, _, err = service.Delete(context.Background(), client, "bucket")
	if err != nil || status != http.StatusNoContent {
		t.Fatalf("could not resume deletion, status=%d err=%v", status, err)
	}
	if len(store.keys) != 0 || deleteBucketCalls != 1 || len(deletedKeys) != 2 {
		t.Fatalf("cleanup incomplete: keys=%v calls=%d revoked=%v", store.keys, deleteBucketCalls, deletedKeys)
	}
}

// TestDeleteBucketFailureDoesNotRevokeKeys preserves usable credentials on failure.
func TestDeleteBucketFailureDoesNotRevokeKeys(t *testing.T) {
	for _, failWithObjects := range []bool{false, true} {
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
		if clientErr != nil {
			t.Fatal(clientErr)
		}
		service := New(store)
		status, _, err := service.Delete(context.Background(), client, "bucket")
		api.Close()
		if err == nil || status != http.StatusConflict || keysRevoked || len(store.keys) != 1 {
			t.Fatalf("deletion failure revoked managed keys: status=%d err=%v revoked=%t keys=%v", status, err, keysRevoked, store.keys)
		}
	}
}
