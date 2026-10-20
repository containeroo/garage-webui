package auth

import (
	"context"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestAdminValidationErrors distinguishes invalid tokens from upstream failures.
func TestAdminValidationErrors(t *testing.T) {
	t.Parallel()

	t.Run("unauthorized", func(t *testing.T) {
		t.Parallel()

		api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "/v2/GetCurrentAdminTokenInfo", r.URL.Path, "unexpected Garage token verification request")
			assert.Equal(t, "Bearer secret", r.Header.Get("Authorization"), "unexpected Garage token verification request")
			w.WriteHeader(401)
			_, _ = w.Write([]byte(`{}`))
		}))
		defer api.Close()
		svc, err := New(&testStore{records: map[string]testRecord{}}, Config{
			GarageURL:     api.URL,
			PublicURL:     "http://localhost:8080",
			SessionSecret: base64.StdEncoding.EncodeToString(make([]byte, 32)),
		})
		require.NoError(t, err)
		_, err = svc.Admin(context.Background(), "secret")
		require.Error(t, err)

		require.ErrorIs(t, err, ErrUnauthenticated)
	})

	t.Run("forbidden", func(t *testing.T) {
		t.Parallel()

		api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "/v2/GetCurrentAdminTokenInfo", r.URL.Path, "unexpected Garage token verification request")
			assert.Equal(t, "Bearer secret", r.Header.Get("Authorization"), "unexpected Garage token verification request")
			w.WriteHeader(403)
			_, _ = w.Write([]byte(`{}`))
		}))
		defer api.Close()
		svc, err := New(&testStore{records: map[string]testRecord{}}, Config{
			GarageURL:     api.URL,
			PublicURL:     "http://localhost:8080",
			SessionSecret: base64.StdEncoding.EncodeToString(make([]byte, 32)),
		})
		require.NoError(t, err)
		_, err = svc.Admin(context.Background(), "secret")
		require.Error(t, err)

		require.ErrorIs(t, err, ErrUnauthenticated)
	})

	t.Run("upstream failure", func(t *testing.T) {
		t.Parallel()

		api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "/v2/GetCurrentAdminTokenInfo", r.URL.Path, "unexpected Garage token verification request")
			assert.Equal(t, "Bearer secret", r.Header.Get("Authorization"), "unexpected Garage token verification request")
			w.WriteHeader(502)
			_, _ = w.Write([]byte(`{}`))
		}))
		defer api.Close()
		svc, err := New(&testStore{records: map[string]testRecord{}}, Config{
			GarageURL:     api.URL,
			PublicURL:     "http://localhost:8080",
			SessionSecret: base64.StdEncoding.EncodeToString(make([]byte, 32)),
		})
		require.NoError(t, err)
		_, err = svc.Admin(context.Background(), "secret")
		require.Error(t, err)

		assert.ErrorContains(t, err, "HTTP 502")
	})

	t.Run("unknown endpoint", func(t *testing.T) {
		t.Parallel()

		api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "/v2/GetCurrentAdminTokenInfo", r.URL.Path, "unexpected Garage token verification request")
			assert.Equal(t, "Bearer secret", r.Header.Get("Authorization"), "unexpected Garage token verification request")
			w.WriteHeader(404)
			_, _ = w.Write([]byte(`{}`))
		}))
		defer api.Close()
		svc, err := New(&testStore{records: map[string]testRecord{}}, Config{
			GarageURL:     api.URL,
			PublicURL:     "http://localhost:8080",
			SessionSecret: base64.StdEncoding.EncodeToString(make([]byte, 32)),
		})
		require.NoError(t, err)
		_, err = svc.Admin(context.Background(), "secret")
		require.Error(t, err)

		assert.ErrorContains(t, err, "HTTP 404")
	})

	t.Run("malformed response", func(t *testing.T) {
		t.Parallel()

		api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "/v2/GetCurrentAdminTokenInfo", r.URL.Path, "unexpected Garage token verification request")
			assert.Equal(t, "Bearer secret", r.Header.Get("Authorization"), "unexpected Garage token verification request")
			w.WriteHeader(200)
			_, _ = w.Write([]byte(`{`))
		}))
		defer api.Close()
		svc, err := New(&testStore{records: map[string]testRecord{}}, Config{
			GarageURL:     api.URL,
			PublicURL:     "http://localhost:8080",
			SessionSecret: base64.StdEncoding.EncodeToString(make([]byte, 32)),
		})
		require.NoError(t, err)
		_, err = svc.Admin(context.Background(), "secret")
		require.Error(t, err)

		assert.ErrorContains(t, err, "decode Garage GetCurrentAdminTokenInfo")
	})

	t.Run("missing fields", func(t *testing.T) {
		t.Parallel()

		api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "/v2/GetCurrentAdminTokenInfo", r.URL.Path, "unexpected Garage token verification request")
			assert.Equal(t, "Bearer secret", r.Header.Get("Authorization"), "unexpected Garage token verification request")
			w.WriteHeader(200)
			_, _ = w.Write([]byte(`{}`))
		}))
		defer api.Close()
		svc, err := New(&testStore{records: map[string]testRecord{}}, Config{
			GarageURL:     api.URL,
			PublicURL:     "http://localhost:8080",
			SessionSecret: base64.StdEncoding.EncodeToString(make([]byte, 32)),
		})
		require.NoError(t, err)
		_, err = svc.Admin(context.Background(), "secret")
		require.Error(t, err)

		assert.ErrorContains(t, err, "incomplete admin token metadata")
	})
}

// TestRejectMixedCredentials prevents secondary credential types from bypassing validation.
func TestRejectMixedCredentials(t *testing.T) {
	t.Parallel()

	assert.False(t, validAdminCredential(Credential{Token: "a", AccessKeyID: "b"}), "mixed Garage credential types accepted")
	assert.False(t, validS3Credential(Credential{Token: "a", AccessKeyID: "b", SecretAccessKey: "secret"}), "mixed Garage credential types accepted")
}
