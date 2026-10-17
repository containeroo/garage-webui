package garage

import (
	"context"
	"fmt"
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
	status, data, err := client.Request(context.Background(), "POST", "CreateBucket", url.Values{}, strings.NewReader(`{}`))
	require.NoError(t, err)
	assert.Equal(t, http.StatusCreated, status)
	assert.JSONEq(t, `{"id":"bucket"}`, string(data))
}

// TestNewRejectsUnsafeURLs rejects URLs containing credentials and non-HTTP schemes.
func TestNewRejectsUnsafeURLs(t *testing.T) {
	for _, raw := range []string{"", "file:///tmp/a", "http://user:pass@example.com", "http://example.com?q=1", "http://example.com?"} {
		_, err := New(raw, "token")
		assert.Error(t, err, raw)
	}
}

// TestRequestRejectsRedirects prevents token forwarding to a redirect target.
func TestRequestRejectsRedirects(t *testing.T) {
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { t.Error("unexpected redirect") }))
	t.Cleanup(target.Close)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, target.URL, http.StatusFound) }))
	t.Cleanup(server.Close)
	client, err := New(server.URL, "secret-token")
	require.NoError(t, err)
	status, _, err := client.Request(context.Background(), "GET", "ListKeys", nil, nil)
	require.NoError(t, err)
	assert.Equal(t, http.StatusFound, status)
}

func TestRequestResponseSizeLimit(t *testing.T) {
	for _, size := range []int{maxResponseBytes, maxResponseBytes + 1} {
		t.Run(fmt.Sprint(size), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				_, _ = io.WriteString(w, strings.Repeat("x", size))
			}))
			defer server.Close()
			client, err := New(server.URL, "token")
			require.NoError(t, err)
			_, data, err := client.Request(context.Background(), "GET", "ListKeys", nil, nil)
			if size > maxResponseBytes {
				require.ErrorContains(t, err, "exceeds")
				assert.Nil(t, data)
			} else {
				require.NoError(t, err)
				assert.Len(t, data, size)
			}
		})
	}
}
