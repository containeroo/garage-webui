package s3

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestListBucketsClient decodes S3 XML while differentiating auth and transport failures.
func TestListBucketsClient(t *testing.T) {
	t.Parallel()

	t.Run("valid", func(t *testing.T) {
		t.Parallel()

		api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.NotEmpty(t, r.URL.Query().Get("X-Amz-Signature"), "missing signed S3 URL")
			w.WriteHeader(200)
			_, _ = w.Write([]byte(`<ListAllMyBucketsResult><Buckets><Bucket><Name>first</Name></Bucket><Bucket><Name>second</Name></Bucket></Buckets></ListAllMyBucketsResult>`))
		}))
		defer api.Close()
		client, err := NewClient(api.URL, "garage", "GKtest", "secret", "http://localhost:8080")
		require.NoError(t, err)
		result, err := client.ListBuckets(context.Background())

		require.NoError(t, err)
		assert.Equal(t, []string{"first", "second"}, result)
	})

	t.Run("denied", func(t *testing.T) {
		t.Parallel()

		api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.NotEmpty(t, r.URL.Query().Get("X-Amz-Signature"), "missing signed S3 URL")
			w.WriteHeader(403)
			_, _ = w.Write([]byte(`<Error/>`))
		}))
		defer api.Close()
		client, err := NewClient(api.URL, "garage", "GKtest", "secret", "http://localhost:8080")
		require.NoError(t, err)
		_, err = client.ListBuckets(context.Background())

		require.Error(t, err)
		require.ErrorIs(t, err, ErrUnauthorized)
	})

	t.Run("outage", func(t *testing.T) {
		t.Parallel()

		api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.NotEmpty(t, r.URL.Query().Get("X-Amz-Signature"), "missing signed S3 URL")
			w.WriteHeader(502)
			_, _ = w.Write([]byte(`<Error/>`))
		}))
		defer api.Close()
		client, err := NewClient(api.URL, "garage", "GKtest", "secret", "http://localhost:8080")
		require.NoError(t, err)
		_, err = client.ListBuckets(context.Background())

		require.Error(t, err)
		assert.NotErrorIs(t, err, ErrUnauthorized)
	})

	t.Run("invalid XML", func(t *testing.T) {
		t.Parallel()

		api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.NotEmpty(t, r.URL.Query().Get("X-Amz-Signature"), "missing signed S3 URL")
			w.WriteHeader(200)
			_, _ = w.Write([]byte(`<ListAllMyBucketsResult`))
		}))
		defer api.Close()
		client, err := NewClient(api.URL, "garage", "GKtest", "secret", "http://localhost:8080")
		require.NoError(t, err)
		_, err = client.ListBuckets(context.Background())

		require.Error(t, err)
		assert.NotErrorIs(t, err, ErrUnauthorized)
	})
}
