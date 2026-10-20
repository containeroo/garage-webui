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
	if err != nil {
		t.Fatal(err)
	}

	manager, err := NewManager(credentials.New(client, store, "http://localhost:8080", credentials.NewCache(), "admin-secret"), "http://localhost:3900", "garage", "http://localhost:8080")
	if err != nil {
		t.Fatal(err)
	}

	return manager
}

// TestBucketScopedSigner verifies per-bucket keys, idempotent CORS updates, and
// exact object- and bucket-scoped presigned URLs.
func TestBucketScopedSigner(t *testing.T) {
	state := newFakeGarage()
	store := &memoryKeyStore{}
	original := json.RawMessage(`{"ID":"existing-rule","AllowedOrigin":["https://other.example"],"AllowedMethod":["GET"],"AllowedHeader":["*"]}`)
	state.cors["one"] = []json.RawMessage{original}
	manager := managerForTest(t, state, store)
	keys := map[string]string{}
	for _, bucket := range []string{"one", "two", "one"} {
		address, err := manager.UploadURL(context.Background(), bucket, "alias-"+bucket, "folder/file.txt", time.Now())
		if err != nil {
			t.Fatal(err)
		}

		u, err := url.Parse(address)
		if err != nil {
			t.Fatal(err)
		}

		if u.Path != "/alias-"+bucket+"/folder/file.txt" || u.Query().Get("X-Amz-Expires") != "300" {
			t.Fatalf("unexpected upload URL %s", address)
		}

		if strings.Contains(address, "secret-") {
			t.Fatal("secret leaked into signed URL")
		}

		credential := strings.Split(u.Query().Get("X-Amz-Credential"), "/")[0]
		if previous, found := keys[bucket]; found && previous != credential {
			t.Fatalf("key for bucket %q changed", bucket)
		}

		keys[bucket] = credential
	}

	if keys["one"] == keys["two"] || len(keys) != 2 || state.createCount != 2 {
		t.Fatalf("expected separate reusable keys per bucket: keys=%v creates=%d", keys, state.createCount)
	}

	if state.allowCount != 2 || state.denyCount != 2 || state.updates["one"] != 1 || state.updates["two"] != 1 {
		t.Fatalf("unexpected permission/CORS operations: allow=%d deny=%d updates=%v", state.allowCount, state.denyCount, state.updates)
	}

	if len(state.cors["one"]) != 2 || string(state.cors["one"][1]) != string(original) {
		t.Fatalf("existing CORS rules were changed: %s", state.cors["one"])
	}

	var rule struct {
		AllowedOrigin []string
		AllowedMethod []string
		AllowedHeader []string
	}
	if err := json.Unmarshal(state.cors["one"][0], &rule); err != nil {
		t.Fatal(err)
	}

	if len(rule.AllowedOrigin) != 1 || rule.AllowedOrigin[0] != "http://localhost:8080" || len(rule.AllowedMethod) == 0 || len(rule.AllowedHeader) == 0 {
		t.Fatalf("new rule does not allow upload: %+v", rule)
	}
}

// TestDeletedBucketSignerIsRecreated ensures stale key IDs are replaced only
// for the affected bucket.
func TestDeletedBucketSignerIsRecreated(t *testing.T) {
	store := &memoryKeyStore{ids: map[string]string{"one": "GKdeleted"}}
	state := newFakeGarage()
	manager := managerForTest(t, state, store)
	if _, err := manager.UploadURL(context.Background(), "one", "alias-one", "file.txt", time.Now()); err != nil {
		t.Fatal(err)
	}

	if store.ids["one"] == "GKdeleted" || state.createCount != 1 {
		t.Fatalf("stale key was not replaced: %#v", store.ids)
	}
}

// TestCORSConfigurationFailurePreventsSigning rejects an upload that could not
// be enabled for this application's browser origin.
func TestCORSConfigurationFailurePreventsSigning(t *testing.T) {
	state := newFakeGarage()
	state.failCORSWrite = true
	store := &memoryKeyStore{}
	manager := managerForTest(t, state, store)
	if _, err := manager.UploadURL(context.Background(), "bucket", "alias", "file.txt", time.Now()); err == nil {
		t.Fatal("expected CORS provisioning failure")
	}

	if store.ids["bucket"] == "" {
		t.Fatal("created key ID must be persisted for retry")
	}
}

// TestExistingCORSRule avoids updating CORS that already permits PUT requests.
func TestExistingCORSRule(t *testing.T) {
	state := newFakeGarage()
	state.cors["bucket"] = []json.RawMessage{json.RawMessage(`{"AllowedOrigin":["http://localhost:8080"],"AllowedMethod":["PUT"],"AllowedHeader":["*"]}`)}
	manager := managerForTest(t, state, &memoryKeyStore{})
	if _, err := manager.UploadURL(context.Background(), "bucket", "alias", "file.txt", time.Now()); err != nil {
		t.Fatal(err)
	}

	if state.updates["bucket"] != 1 {
		t.Fatal("multipart requires ETag exposure when an existing PUT rule omits it")
	}

	if len(state.cors["bucket"]) != 2 {
		t.Fatal("existing CORS rule must be preserved")
	}
}

// TestConcurrentBucketUploads ensures parallel uploads reuse a bucket's key
// without granting permissions on any unrelated bucket.
func TestConcurrentBucketUploads(t *testing.T) {
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
		t.Error(err)
	}

	if state.createCount != 3 || len(store.ids) != 3 || len(state.updates) != 3 {
		t.Fatalf("expected exactly one key and CORS rule per bucket: keys=%v updates=%v", store.ids, state.updates)
	}
}

// TestManagerConfiguration rejects a disabled manager and unsafe public URLs.
func TestManagerConfiguration(t *testing.T) {
	manager, err := NewManager(nil, "", "garage", "https://web.example")
	if err != nil || manager != nil {
		t.Fatalf("disabled manager: %v", err)
	}

	for _, endpoint := range []string{"http://s3.example", "https://s3.example/path", "file:///tmp"} {
		if _, err := NewManager(nil, endpoint, "garage", "https://web.example"); err == nil {
			t.Fatalf("unsafe endpoint accepted: %s", endpoint)
		}
	}
}
