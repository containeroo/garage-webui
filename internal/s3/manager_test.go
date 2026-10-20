package s3

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/containeroo/garage-webui/internal/garage/admin"
	"github.com/containeroo/garage-webui/internal/garage/credentials"
	"github.com/containeroo/garage-webui/internal/signing"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// memoryKeyStore simulates a bucket lock held across provisioning operations.
type memoryKeyStore struct {
	mu      sync.Mutex
	ids     map[string]string
	readIDs map[string]string
	copyIDs map[string]string
}

type memoryBucketLock struct {
	store  *memoryKeyStore
	bucket string
	once   sync.Once
}

// LockBucket keeps the in-memory store locked until the caller closes the handle.
func (s *memoryKeyStore) LockBucket(ctx context.Context, bucket string) (signing.BucketLock, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.mu.Lock()
	if s.ids == nil {
		s.ids = make(map[string]string)
	}
	if s.readIDs == nil {
		s.readIDs = make(map[string]string)
	}
	if s.copyIDs == nil {
		s.copyIDs = make(map[string]string)
	}
	return &memoryBucketLock{store: s, bucket: bucket}, nil
}

// KeyID returns a key from its independent purpose map.
func (l *memoryBucketLock) KeyID(_ context.Context, purpose signing.Purpose) (string, error) {
	switch purpose {
	case signing.Write:
		return l.store.ids[l.bucket], nil
	case signing.Read:
		return l.store.readIDs[l.bucket], nil
	case signing.Copy:
		return l.store.copyIDs[l.bucket], nil
	default:
		return "", fmt.Errorf("unknown purpose %q", purpose)
	}
}

// SaveKeyID stores the key ID before Garage permissions are configured.
func (l *memoryBucketLock) SaveKeyID(_ context.Context, purpose signing.Purpose, id string) error {
	switch purpose {
	case signing.Write:
		l.store.ids[l.bucket] = id
	case signing.Read:
		l.store.readIDs[l.bucket] = id
	case signing.Copy:
		l.store.copyIDs[l.bucket] = id
	default:
		return fmt.Errorf("unknown purpose %q", purpose)
	}
	return nil
}

// KeyIDs lists the bucket's three managed credentials.
func (l *memoryBucketLock) KeyIDs(context.Context) ([]string, error) {
	return []string{l.store.ids[l.bucket], l.store.readIDs[l.bucket], l.store.copyIDs[l.bucket]}, nil
}

// DeleteKeyReferences forgets managed keys after Garage cleanup.
func (l *memoryBucketLock) DeleteKeyReferences(context.Context) error {
	delete(l.store.ids, l.bucket)
	delete(l.store.readIDs, l.bucket)
	delete(l.store.copyIDs, l.bucket)
	return nil
}

// Close releases the simulated lock exactly once.
func (l *memoryBucketLock) Close() error {
	l.once.Do(l.store.mu.Unlock)
	return nil
}

// fakeGarage contains only the Admin API operations needed for direct uploads.
type fakeGarage struct {
	mu            sync.Mutex
	created       map[string]string
	names         map[string]string
	cors          map[string][]json.RawMessage
	createCount   int
	allowCount    int
	denyCount     int
	updates       map[string]int
	failCORSWrite bool
}

// newFakeGarage builds an empty Admin API state for the tests.
func newFakeGarage() *fakeGarage {
	return &fakeGarage{
		created: map[string]string{},
		names:   map[string]string{},
		cors:    map[string][]json.RawMessage{},
		updates: map[string]int{},
	}
}

// ServeHTTP simulates the Garage Admin API with assertions on key permissions.
func (g *fakeGarage) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if r.Header.Get("Authorization") != "Bearer admin-secret" {
		http.Error(w, "missing admin token", http.StatusUnauthorized)
		return
	}

	write := func(v any) { _ = json.NewEncoder(w).Encode(v) }
	switch r.URL.Path {
	case "/v2/CreateKey":
		var input struct {
			Name string `json:"name"`
		}
		if json.NewDecoder(r.Body).Decode(&input) != nil ||
			(!strings.HasPrefix(input.Name, "garage-webui-upload-") &&
				!strings.HasPrefix(input.Name, "garage-webui-read-") &&
				!strings.HasPrefix(input.Name, "garage-webui-copy-")) {
			http.Error(w, "invalid key name", http.StatusBadRequest)
			return
		}
		g.createCount++
		id := fmt.Sprintf("GKbucket%d", g.createCount)
		g.created[id] = "secret-" + id
		g.names[id] = input.Name
		write(signing.Credential{
			AccessKeyID:     id,
			SecretAccessKey: g.created[id],
		})
	case "/v2/GetKeyInfo":
		id := r.URL.Query().Get("id")
		if r.URL.Query().Get("showSecretKey") != "true" {
			http.Error(w, "missing showSecretKey", http.StatusBadRequest)
			return
		}
		secret, ok := g.created[id]
		if !ok {
			http.NotFound(w, r)
			return
		}
		write(signing.Credential{
			AccessKeyID:     id,
			SecretAccessKey: secret,
		})
	case "/v2/AllowBucketKey", "/v2/DenyBucketKey":
		var input struct {
			BucketID    string          `json:"bucketId"`
			KeyID       string          `json:"accessKeyId"`
			Permissions map[string]bool `json:"permissions"`
		}
		if json.NewDecoder(r.Body).Decode(&input) != nil || g.created[input.KeyID] == "" || input.BucketID == "" {
			http.Error(w, "bad key grant", http.StatusBadRequest)
			return
		}
		keyName := g.names[input.KeyID]
		if r.URL.Path == "/v2/AllowBucketKey" {
			if !validAllowedPermission(input.Permissions, keyName) {
				http.Error(w, "incorrect signing key permissions", http.StatusBadRequest)
				return
			}

			g.allowCount++
		} else {
			if !validDeniedPermission(input.Permissions, keyName) {
				http.Error(w, "incorrect revoked permissions", http.StatusBadRequest)
				return
			}

			g.denyCount++
		}
		write(map[string]any{})
	case "/v2/GetBucketInfo":
		id := r.URL.Query().Get("id")
		write(map[string]any{"id": id, "corsRules": g.cors[id]})
	case "/v2/UpdateBucket":
		if g.failCORSWrite {
			http.Error(w, "cannot update CORS", http.StatusForbidden)
			return
		}
		id := r.URL.Query().Get("id")
		var input struct {
			CORSRules []json.RawMessage `json:"corsRules"`
		}
		if json.NewDecoder(r.Body).Decode(&input) != nil {
			http.Error(w, "invalid CORS update", http.StatusBadRequest)
			return
		}
		g.cors[id] = input.CORSRules
		g.updates[id]++
		write(map[string]any{})
	default:
		http.NotFound(w, r)
	}
}

// validAllowedPermission ensures managed keys never receive extra capabilities.
func validAllowedPermission(p map[string]bool, keyName string) bool {
	if strings.HasPrefix(keyName, "garage-webui-read-") {
		return p["read"] && !p["write"] && !p["owner"]
	}

	if strings.HasPrefix(keyName, "garage-webui-copy-") {
		return p["read"] && p["write"] && !p["owner"]
	}

	return p["write"] && !p["read"] && !p["owner"]
}

// validDeniedPermission verifies each key's complementary permissions are revoked.
func validDeniedPermission(p map[string]bool, keyName string) bool {
	if strings.HasPrefix(keyName, "garage-webui-read-") {
		return p["write"] && p["owner"] && !p["read"]
	}

	if strings.HasPrefix(keyName, "garage-webui-copy-") {
		return p["owner"] && !p["read"] && !p["write"]
	}

	return p["read"] && p["owner"] && !p["write"]
}

// managerForTest constructs an upload manager against a simulated Garage Admin API.
func managerForTest(t *testing.T, state *fakeGarage, store *memoryKeyStore) *Manager {
	t.Helper()
	adminServer := httptest.NewServer(state)
	t.Cleanup(adminServer.Close)
	client, err := admin.New(adminServer.URL, "admin-secret")
	require.NoError(t, err)

	manager, err := NewManager(credentials.New(client, store, "http://localhost:8080", credentials.NewCache(), "admin-secret"), "http://localhost:3900", "garage", "http://localhost:8080")
	require.NoError(t, err)

	return manager
}

// TestBucketScopedSigner verifies per-bucket keys, idempotent CORS updates, and
// exact object- and bucket-scoped presigned URLs.
func TestBucketScopedSigner(t *testing.T) {
	t.Parallel()

	state := newFakeGarage()
	store := &memoryKeyStore{}
	original := json.RawMessage(`{"ID":"existing-rule","AllowedOrigin":["https://other.example"],"AllowedMethod":["GET"],"AllowedHeader":["*"]}`)
	state.cors["one"] = []json.RawMessage{original}
	manager := managerForTest(t, state, store)
	keys := map[string]string{}
	t.Run("first bucket", func(t *testing.T) {
		bucket := "one"
		address, err := manager.UploadURL(context.Background(), bucket, "alias-"+bucket, "folder/file.txt", time.Now())
		require.NoError(t, err)

		u, err := url.Parse(address)
		require.NoError(t, err)

		assert.Equal(t, "/alias-"+bucket+"/folder/file.txt", u.Path)
		assert.Equal(t, "300", u.Query().Get("X-Amz-Expires"))

		assert.NotContains(t, address, "secret-", "secret leaked into signed URL")

		credential := strings.Split(u.Query().Get("X-Amz-Credential"), "/")[0]
		if previous, found := keys[bucket]; found {
			assert.Equal(t, previous, credential, "repeated uploads must reuse the bucket key")
		}

		keys[bucket] = credential
	})

	t.Run("second bucket", func(t *testing.T) {
		bucket := "two"
		address, err := manager.UploadURL(context.Background(), bucket, "alias-"+bucket, "folder/file.txt", time.Now())
		require.NoError(t, err)

		u, err := url.Parse(address)
		require.NoError(t, err)

		assert.Equal(t, "/alias-"+bucket+"/folder/file.txt", u.Path)
		assert.Equal(t, "300", u.Query().Get("X-Amz-Expires"))

		assert.NotContains(t, address, "secret-", "secret leaked into signed URL")

		credential := strings.Split(u.Query().Get("X-Amz-Credential"), "/")[0]
		if previous, found := keys[bucket]; found {
			assert.Equal(t, previous, credential, "repeated uploads must reuse the bucket key")
		}

		keys[bucket] = credential
	})

	t.Run("reuse first bucket", func(t *testing.T) {
		bucket := "one"
		address, err := manager.UploadURL(context.Background(), bucket, "alias-"+bucket, "folder/file.txt", time.Now())
		require.NoError(t, err)

		u, err := url.Parse(address)
		require.NoError(t, err)

		assert.Equal(t, "/alias-"+bucket+"/folder/file.txt", u.Path)
		assert.Equal(t, "300", u.Query().Get("X-Amz-Expires"))

		assert.NotContains(t, address, "secret-", "secret leaked into signed URL")

		credential := strings.Split(u.Query().Get("X-Amz-Credential"), "/")[0]
		if previous, found := keys[bucket]; found {
			assert.Equal(t, previous, credential, "repeated uploads must reuse the bucket key")
		}

		keys[bucket] = credential
	})

	assert.NotEqual(t, keys["two"], keys["one"])
	assert.Len(t, keys, 2)
	assert.Equal(t, 2, state.createCount)

	assert.Equal(t, 2, state.allowCount)
	assert.Equal(t, 2, state.denyCount)
	assert.Equal(t, 1, state.updates["one"])
	assert.Equal(t, 1, state.updates["two"])

	require.Len(t, state.cors["one"], 2)
	assert.Equal(t, string(original), string(state.cors["one"][1]))

	var rule struct {
		AllowedOrigin []string
		AllowedMethod []string
		AllowedHeader []string
	}
	{
		err := json.Unmarshal(state.cors["one"][0], &rule)
		require.NoError(t, err)
	}

	assert.Equal(t, []string{"http://localhost:8080"}, rule.AllowedOrigin)
	assert.NotEmpty(t, rule.AllowedMethod)
	assert.NotEmpty(t, rule.AllowedHeader)
}

// TestDeletedBucketSignerIsRecreated ensures stale key IDs are replaced only
// for the affected bucket.
func TestDeletedBucketSignerIsRecreated(t *testing.T) {
	t.Parallel()

	store := &memoryKeyStore{ids: map[string]string{"one": "GKdeleted"}}
	state := newFakeGarage()
	manager := managerForTest(t, state, store)
	{
		_, err := manager.UploadURL(context.Background(), "one", "alias-one", "file.txt", time.Now())
		require.NoError(t, err)
	}

	assert.NotEqual(t, "GKdeleted", store.ids["one"], "stale key was not replaced: %#v", store.ids)
	assert.Equal(t, 1, state.createCount)
}

// TestCORSConfigurationFailurePreventsSigning rejects an upload that could not
// be enabled for this application's browser origin.
func TestCORSConfigurationFailurePreventsSigning(t *testing.T) {
	t.Parallel()

	state := newFakeGarage()
	state.failCORSWrite = true
	store := &memoryKeyStore{}
	manager := managerForTest(t, state, store)
	{
		_, err := manager.UploadURL(context.Background(), "bucket", "alias", "file.txt", time.Now())
		require.Error(t, err, "expected CORS provisioning failure")
	}

	assert.NotEmpty(t, store.ids["bucket"], "created key ID must be persisted for retry")
}

// TestExistingCORSRule avoids updating CORS that already permits PUT requests.
func TestExistingCORSRule(t *testing.T) {
	t.Parallel()

	state := newFakeGarage()
	state.cors["bucket"] = []json.RawMessage{json.RawMessage(`{"AllowedOrigin":["http://localhost:8080"],"AllowedMethod":["PUT"],"AllowedHeader":["*"]}`)}
	manager := managerForTest(t, state, &memoryKeyStore{})
	{
		_, err := manager.UploadURL(context.Background(), "bucket", "alias", "file.txt", time.Now())
		require.NoError(t, err)
	}

	assert.Equal(t, 1, state.updates["bucket"], "multipart requires ETag exposure when an existing PUT rule omits it")

	assert.Len(t, state.cors["bucket"], 2, "existing CORS rule must be preserved")
}

// TestConcurrentBucketUploads ensures parallel uploads reuse a bucket's key
// without granting permissions on any unrelated bucket.
func TestConcurrentBucketUploads(t *testing.T) {
	t.Parallel()

	state := newFakeGarage()
	store := &memoryKeyStore{}
	manager := managerForTest(t, state, store)
	var wg sync.WaitGroup
	failures := make(chan error, 12)
	for i := range 12 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			bucket := fmt.Sprintf("bucket-%d", i%3)
			_, err := manager.UploadURL(context.Background(), bucket, bucket, "file.txt", time.Now())
			if err != nil {
				failures <- err
			}
		}()
	}

	wg.Wait()
	close(failures)
	for err := range failures {
		assert.NoError(t, err)
	}

	assert.Equal(t, 3, state.createCount)
	assert.Len(t, store.ids, 3)
	assert.Len(t, state.updates, 3)
}

// TestManagerConfiguration rejects a disabled manager and unsafe public URLs.
func TestManagerConfiguration(t *testing.T) {
	t.Parallel()

	manager, err := NewManager(nil, "", "garage", "https://web.example")
	require.NoError(t, err)
	assert.Nil(t, manager)

	t.Run("http://s3.example", func(t *testing.T) {
		t.Parallel()

		endpoint := "http://s3.example"
		{
			_, err := NewManager(nil, endpoint, "garage", "https://web.example")
			require.Error(t, err, "unsafe endpoint accepted: %s", endpoint)
		}
	})

	t.Run("https://s3.example/path", func(t *testing.T) {
		t.Parallel()

		endpoint := "https://s3.example/path"
		{
			_, err := NewManager(nil, endpoint, "garage", "https://web.example")
			require.Error(t, err, "unsafe endpoint accepted: %s", endpoint)
		}
	})

	t.Run("file:///tmp", func(t *testing.T) {
		t.Parallel()

		endpoint := "file:///tmp"
		{
			_, err := NewManager(nil, endpoint, "garage", "https://web.example")
			require.Error(t, err, "unsafe endpoint accepted: %s", endpoint)
		}
	})
}
