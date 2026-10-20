package admin

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRequest verifies the Garage endpoint, authorization and payload.
func TestRequest(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/base/v2/CreateBucket", r.URL.Path)
		assert.Equal(t, "Bearer secret-token", r.Header.Get("Authorization"))
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id":"bucket"}`))
	}))
	t.Cleanup(server.Close)
	client, err := New(server.URL+"/base", "secret-token")
	require.NoError(t, err)
	status, data, err := client.Proxy(context.Background(), OpCreateBucket, url.Values{}, []byte(`{}`))
	require.NoError(t, err)
	assert.Equal(t, http.StatusCreated, status)
	assert.JSONEq(t, `{"id":"bucket"}`, string(data))
}

// TestNewRejectsUnsafeURLs rejects URLs containing credentials and non-HTTP schemes.
func TestNewRejectsUnsafeURLs(t *testing.T) {
	t.Parallel()

	t.Run("empty endpoint", func(t *testing.T) {
		t.Parallel()

		raw := ""
		_, err := New(raw, "token")
		assert.Error(t, err, raw)
	})

	t.Run("file:///tmp/a", func(t *testing.T) {
		t.Parallel()

		raw := "file:///tmp/a"
		_, err := New(raw, "token")
		assert.Error(t, err, raw)
	})

	t.Run("http://user:pass@example.com", func(t *testing.T) {
		t.Parallel()

		raw := "http://user:pass@example.com"
		_, err := New(raw, "token")
		assert.Error(t, err, raw)
	})

	t.Run("http://example.com?q=1", func(t *testing.T) {
		t.Parallel()

		raw := "http://example.com?q=1"
		_, err := New(raw, "token")
		assert.Error(t, err, raw)
	})

	t.Run("http://example.com?", func(t *testing.T) {
		t.Parallel()

		raw := "http://example.com?"
		_, err := New(raw, "token")
		assert.Error(t, err, raw)
	})
}

// TestRequestRejectsRedirects prevents token forwarding to a redirect target.
func TestRequestRejectsRedirects(t *testing.T) {
	t.Parallel()

	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { assert.Fail(t, "unexpected redirect") }))
	t.Cleanup(target.Close)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, target.URL, http.StatusFound) }))
	t.Cleanup(server.Close)
	client, err := New(server.URL, "secret-token")
	require.NoError(t, err)
	status, _, err := client.Proxy(context.Background(), OpListKeys, nil, nil)
	require.NoError(t, err)
	assert.Equal(t, http.StatusFound, status)
}

func TestRequestResponseSizeLimit(t *testing.T) {
	t.Parallel()

	t.Run("at response limit", func(t *testing.T) {
		t.Parallel()

		size := maxResponseBytes
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = io.WriteString(w, strings.Repeat("x", size))
		}))
		defer server.Close()
		client, err := New(server.URL, "token")
		require.NoError(t, err)
		_, data, err := client.Proxy(context.Background(), OpListKeys, nil, nil)
		require.NoError(t, err)
		assert.Len(t, data, size)
	})

	t.Run("exceeds response limit", func(t *testing.T) {
		t.Parallel()

		size := maxResponseBytes + 1
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = io.WriteString(w, strings.Repeat("x", size))
		}))
		defer server.Close()
		client, err := New(server.URL, "token")
		require.NoError(t, err)
		_, data, err := client.Proxy(context.Background(), OpListKeys, nil, nil)
		require.ErrorContains(t, err, "exceeds")
		assert.Nil(t, data)
	})
}
