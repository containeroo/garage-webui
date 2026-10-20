package admin

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestTypedClientErrorMapping distinguishes token failures, resource absence and outages.
func TestTypedClientErrorMapping(t *testing.T) {
	t.Parallel()

	t.Run(http.StatusText(http.StatusUnauthorized), func(t *testing.T) {
		t.Parallel()

		api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusUnauthorized)
		}))
		defer api.Close()
		client, err := New(api.URL, "secret")
		require.NoError(t, err)
		_, err = client.CurrentAdminToken(context.Background())
		require.Error(t, err, "expected Garage rejection")
		require.ErrorIs(t, err, ErrUnauthorized)
		assert.NotContains(t, err.Error(), "secret", "client error exposed the Garage token")
	})

	t.Run(http.StatusText(http.StatusForbidden), func(t *testing.T) {
		t.Parallel()

		api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusForbidden)
		}))
		defer api.Close()
		client, err := New(api.URL, "secret")
		require.NoError(t, err)
		_, err = client.CurrentAdminToken(context.Background())
		require.Error(t, err, "expected Garage rejection")
		require.ErrorIs(t, err, ErrForbidden)
		assert.NotContains(t, err.Error(), "secret", "client error exposed the Garage token")
	})

	t.Run(http.StatusText(http.StatusNotFound), func(t *testing.T) {
		t.Parallel()

		api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		}))
		defer api.Close()
		client, err := New(api.URL, "secret")
		require.NoError(t, err)
		_, err = client.CurrentAdminToken(context.Background())
		require.Error(t, err, "expected Garage rejection")
		require.ErrorIs(t, err, ErrNotFound)
		assert.NotContains(t, err.Error(), "secret", "client error exposed the Garage token")
	})

	t.Run(http.StatusText(http.StatusConflict), func(t *testing.T) {
		t.Parallel()

		api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusConflict)
		}))
		defer api.Close()
		client, err := New(api.URL, "secret")
		require.NoError(t, err)
		_, err = client.CurrentAdminToken(context.Background())
		require.Error(t, err, "expected Garage rejection")
		require.ErrorIs(t, err, ErrConflict)
		assert.NotContains(t, err.Error(), "secret", "client error exposed the Garage token")
	})

	t.Run(http.StatusText(http.StatusBadGateway), func(t *testing.T) {
		t.Parallel()

		api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusBadGateway)
		}))
		defer api.Close()
		client, err := New(api.URL, "secret")
		require.NoError(t, err)
		_, err = client.CurrentAdminToken(context.Background())
		require.Error(t, err, "expected Garage rejection")

		assert.NotErrorIs(t, err, ErrUnauthorized)
		assert.NotContains(t, err.Error(), "secret", "client error exposed the Garage token")
	})
}

// TestTypedClientOperations prevents arbitrary methods and protects key secrets.
func TestTypedClientOperations(t *testing.T) {
	t.Parallel()

	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "Bearer secret", r.Header.Get("Authorization"), "admin token was not forwarded")
		switch r.URL.Path {
		case "/v2/GetCurrentAdminTokenInfo":
			_, _ = w.Write([]byte(`{"name":"operator","scope":["ListBuckets"],"expired":false}`))
		case "/v2/GetKeyInfo":
			assert.False(t, r.URL.Query().Has("showSecretKey"), "proxy leaked secret-key query flag")
			_, _ = w.Write([]byte(`{"accessKeyId":"key"}`))
		case "/v2/AllowBucketKey":
			assert.Equal(t, http.MethodPost, r.Method, "permission update used the wrong method")
			_, _ = w.Write([]byte(`{}`))
		default:
			assert.Failf(t, "unexpected endpoint", "%s", r.URL.Path)
		}
	}))
	defer api.Close()
	client, err := New(api.URL, "secret")
	require.NoError(t, err)
	info, err := client.CurrentAdminToken(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "operator", info.Name, "unexpected current token: %+v, %v", info, err)
	query := url.Values{"id": {"key"}, "showSecretKey": {"true"}}
	status, _, err := client.Proxy(context.Background(), OpGetKeyInfo, query, nil)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, status)
	assert.Equal(t, "true", query.Get("showSecretKey"), "proxy failed to isolate query: status=%d err=%v", status, err)
	{
		err := client.AllowBucketKey(context.Background(), "bucket", "key", BucketPermissions{Read: true})
		require.NoError(t, err)
	}
	{
		_, _, err := client.Proxy(context.Background(), Operation("../../Metrics"), nil, nil)
		require.Error(t, err, "unknown upstream endpoint was permitted")
	}
}

// TestGarageErrorsStartLowercase keeps API errors consistent with Go conventions.
func TestGarageErrorsStartLowercase(t *testing.T) {
	t.Parallel()

	t.Run("unauthorized", func(t *testing.T) {
		t.Parallel()

		err := ErrUnauthorized
		message := err.Error()
		require.NotEmpty(t, message)
		assert.GreaterOrEqual(t, message[0], byte('a'))
		assert.LessOrEqual(t, message[0], byte('z'))
	})

	t.Run("forbidden", func(t *testing.T) {
		t.Parallel()

		err := ErrForbidden
		message := err.Error()
		require.NotEmpty(t, message)
		assert.GreaterOrEqual(t, message[0], byte('a'))
		assert.LessOrEqual(t, message[0], byte('z'))
	})

	t.Run("not found", func(t *testing.T) {
		t.Parallel()

		err := ErrNotFound
		message := err.Error()
		require.NotEmpty(t, message)
		assert.GreaterOrEqual(t, message[0], byte('a'))
		assert.LessOrEqual(t, message[0], byte('z'))
	})

	t.Run("conflict", func(t *testing.T) {
		t.Parallel()

		err := ErrConflict
		message := err.Error()
		require.NotEmpty(t, message)
		assert.GreaterOrEqual(t, message[0], byte('a'))
		assert.LessOrEqual(t, message[0], byte('z'))
	})

	t.Run("status error", func(t *testing.T) {
		t.Parallel()

		err := &StatusError{Operation: OpGetBucketInfo, Status: http.StatusBadRequest}
		message := err.Error()
		require.NotEmpty(t, message)
		assert.GreaterOrEqual(t, message[0], byte('a'))
		assert.LessOrEqual(t, message[0], byte('z'))
	})
}
