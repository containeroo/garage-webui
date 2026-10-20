package admin

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBucketByID(t *testing.T) {
	t.Parallel()

	t.Run("valid", func(t *testing.T) {
		t.Parallel()

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "/v2/GetBucketInfo", r.URL.Path)
			assert.Equal(t, "bucket", r.URL.Query().Get("id"))
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"id":"bucket","globalAliases":["name"]}`))
		}))
		defer server.Close()
		client, err := New(server.URL, "token")
		require.NoError(t, err)
		bucket, err := client.BucketByID(context.Background(), "bucket")

		require.NoError(t, err)
		assert.Equal(t, []string{"name"}, bucket.GlobalAliases)
	})

	t.Run("wrong id", func(t *testing.T) {
		t.Parallel()

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "/v2/GetBucketInfo", r.URL.Path)
			assert.Equal(t, "bucket", r.URL.Query().Get("id"))
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"id":"other","globalAliases":["name"]}`))
		}))
		defer server.Close()
		client, err := New(server.URL, "token")
		require.NoError(t, err)
		_, err = client.BucketByID(context.Background(), "bucket")

		require.Error(t, err)
	})

	t.Run("no alias", func(t *testing.T) {
		t.Parallel()

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "/v2/GetBucketInfo", r.URL.Path)
			assert.Equal(t, "bucket", r.URL.Query().Get("id"))
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"id":"bucket","globalAliases":[]}`))
		}))
		defer server.Close()
		client, err := New(server.URL, "token")
		require.NoError(t, err)
		_, err = client.BucketByID(context.Background(), "bucket")

		require.Error(t, err)
	})

	t.Run("invalid JSON", func(t *testing.T) {
		t.Parallel()

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "/v2/GetBucketInfo", r.URL.Path)
			assert.Equal(t, "bucket", r.URL.Query().Get("id"))
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{`))
		}))
		defer server.Close()
		client, err := New(server.URL, "token")
		require.NoError(t, err)
		_, err = client.BucketByID(context.Background(), "bucket")

		require.Error(t, err)
	})

	t.Run("not found", func(t *testing.T) {
		t.Parallel()

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "/v2/GetBucketInfo", r.URL.Path)
			assert.Equal(t, "bucket", r.URL.Query().Get("id"))
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{}`))
		}))
		defer server.Close()
		client, err := New(server.URL, "token")
		require.NoError(t, err)
		_, err = client.BucketByID(context.Background(), "bucket")

		require.Error(t, err)
	})
}
