package jobs

import (
	"context"
	"encoding/base64"
	"encoding/xml"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/containeroo/garage-webui/internal/assembly"
	"github.com/containeroo/garage-webui/internal/auth"
	"github.com/containeroo/garage-webui/internal/credential"
	"github.com/containeroo/garage-webui/internal/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRecursiveJobDeletesAndPersists exercises authentication, S3 operations and durable SQLite progress.
func TestRecursiveJobDeletesAndPersists(t *testing.T) {
	t.Parallel()

	mu := &sync.Mutex{}
	objects := map[string]bool{"backup/": true, "backup/root.txt": true, "backup/nested/file.txt": true}
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/":
			w.Header().Set("Content-Type", "application/xml")
			_, _ = w.Write([]byte(`<ListAllMyBucketsResult><Buckets><Bucket><Name>bucket</Name></Bucket></Buckets></ListAllMyBucketsResult>`))
		case r.Method == http.MethodDelete && strings.HasPrefix(r.URL.Path, "/bucket/"):
			key := strings.TrimPrefix(r.URL.Path, "/bucket/")
			mu.Lock()
			delete(objects, key)
			mu.Unlock()
			w.WriteHeader(http.StatusNoContent)
		case r.Method == http.MethodGet && r.URL.Path == "/bucket":
			prefix := r.URL.Query().Get("prefix")
			mu.Lock()
			keys := make([]string, 0)
			children := map[string]bool{}
			for key := range objects {
				if !strings.HasPrefix(key, prefix) {
					continue
				}
				suffix := strings.TrimPrefix(key, prefix)
				if idx := strings.Index(suffix, "/"); idx >= 0 {
					children[prefix+suffix[:idx+1]] = true
				} else {
					keys = append(keys, key)
				}
			}
			mu.Unlock()
			sort.Strings(keys)
			w.Header().Set("Content-Type", "application/xml")
			_, _ = w.Write([]byte(`<ListBucketResult xmlns="http://s3.amazonaws.com/doc/2006-03-01/">`))
			for _, key := range keys {
				_, _ = w.Write([]byte(`<Contents><Key>` + xmlEscape(key) + `</Key></Contents>`))
			}
			for child := range children {
				_, _ = w.Write([]byte(`<CommonPrefixes><Prefix>` + xmlEscape(child) + `</Prefix></CommonPrefixes>`))
			}
			_, _ = w.Write([]byte(`</ListBucketResult>`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(api.Close)

	db, err := sqlite.Open(t.Context(), "sqlite:"+filepath.Join(t.TempDir(), "jobs.db"), slog.Default())
	require.NoError(t, err)
	t.Cleanup(db.Close)
	security, err := auth.New(db, auth.Config{GarageURL: "http://garage.example:3903", S3URL: api.URL, Region: "garage", SessionSecret: base64.StdEncoding.EncodeToString(make([]byte, 32))})
	require.NoError(t, err)
	coordinator, err := New(db, security, assembly.New(security, db, "http://localhost:8080"), slog.Default())
	require.NoError(t, err)
	identity, err := security.Verify(t.Context(), auth.Credential{Kind: credential.S3, AccessKeyID: "GKtest", SecretAccessKey: "secret"})
	require.NoError(t, err)
	queued, err := coordinator.Start(t.Context(), identity, "bucket", Selection{Folders: []string{"backup/"}})
	require.NoError(t, err)
	assert.Equal(t, "pending", queued.Status)
	claimed, ok, err := db.ClaimDeletionJob(t.Context(), coordinator.worker, time.Minute)
	require.NoError(t, err)
	require.True(t, ok)
	coordinator.execute(context.Background(), claimed)

	visible, err := coordinator.List(t.Context(), identity)
	require.NoError(t, err)
	require.Len(t, visible, 1)
	assert.Equal(t, "completed", visible[0].Status)
	assert.Equal(t, int64(3), visible[0].Deleted)
	assert.Empty(t, visible[0].EncryptedCredential)
	mu.Lock()
	assert.Empty(t, objects)
	mu.Unlock()
}

// xmlEscape encodes test fixture paths as XML content.
func xmlEscape(text string) string {
	var out strings.Builder
	_ = xml.EscapeText(&out, []byte(text))
	return out.String()
}
