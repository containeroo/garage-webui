package auth

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/containeroo/garage-webui/internal/credential"
	"github.com/containeroo/garage-webui/internal/garage/admin"
	"github.com/containeroo/garage-webui/internal/storage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// sessionRecord holds encrypted credentials and their expiry in the test store.
type sessionRecord struct {
	// data contains the authenticated encrypted credential.
	data []byte
	// expiry is the configured session expiration.
	expiry time.Time
	// theme is the current session theme.
	theme string
}

// sessionStore is an in-memory implementation of session persistence for core tests.
type sessionStore struct {
	// entries holds persisted session data indexed by its hash.
	entries map[string]sessionRecord
}

// SaveSession records the encrypted credential and its expiry.
func (s *sessionStore) SaveSession(_ context.Context, hash, data []byte, expiry time.Time) error {
	s.entries[string(hash)] = sessionRecord{data: data, expiry: expiry, theme: "Light"}
	return nil
}

// LoadSession retrieves a stored encrypted credential and its theme.
func (s *sessionStore) LoadSession(_ context.Context, hash []byte) ([]byte, string, error) {
	record, ok := s.entries[string(hash)]
	if !ok || record.expiry.Before(time.Now()) {
		return nil, "", storage.ErrSessionNotFound
	}
	return record.data, record.theme, nil
}

// DeleteSession removes a stored session by its hash.
func (s *sessionStore) DeleteSession(_ context.Context, hash []byte) error {
	delete(s.entries, string(hash))
	return nil
}

// SetSessionTheme records the selected session theme.
func (s *sessionStore) SetSessionTheme(_ context.Context, hash []byte, theme string) error {
	record := s.entries[string(hash)]
	record.theme = theme
	s.entries[string(hash)] = record
	return nil
}

// hashSessionToken decodes an opaque browser token for core session operations.
func hashSessionToken(t *testing.T, token string) []byte {
	t.Helper()
	decoded, err := base64.RawURLEncoding.DecodeString(token)
	require.NoError(t, err)
	require.Len(t, decoded, 32)
	hash := sha256.Sum256(decoded)
	return hash[:]
}

// TestCoreSessions ensures session encryption and rotation require no HTTP request objects.
func TestCoreSessions(t *testing.T) {
	t.Parallel()

	t.Run("create, revalidate, and revoke", func(t *testing.T) {
		t.Parallel()

		upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "Bearer valid-token", r.Header.Get("Authorization"))
			assert.Equal(t, "/v2/GetCurrentAdminTokenInfo", r.URL.Path)
			assert.NoError(t, json.NewEncoder(w).Encode(map[string]any{"name": "alice", "scope": []string{"ListBuckets"}}))
		}))
		t.Cleanup(upstream.Close)

		store := &sessionStore{entries: make(map[string]sessionRecord)}
		config := Config{GarageURL: upstream.URL, SessionSecret: base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{7}, 32))}
		service, err := New(store, config)
		require.NoError(t, err)

		token, err := service.Login(t.Context(), nil, Credential{Kind: credential.Admin, Token: "valid-token"})
		require.NoError(t, err)
		hash := hashSessionToken(t, token)
		require.Len(t, store.entries, 1)
		for _, record := range store.entries {
			assert.NotContains(t, string(record.data), "valid-token")
		}

		identity, err := service.Authenticate(t.Context(), hash)
		require.NoError(t, err)
		assert.Equal(t, "alice", identity.User)
		assert.True(t, identity.Allows(admin.OpListBuckets))

		require.NoError(t, service.SetTheme(t.Context(), hash, "Dark"))
		identity, err = service.Authenticate(t.Context(), hash)
		require.NoError(t, err)
		assert.Equal(t, "Dark", identity.Theme)

		rotated, err := service.Login(t.Context(), hash, Credential{Kind: credential.Admin, Token: "valid-token"})
		require.NoError(t, err)
		_, err = service.Authenticate(t.Context(), hash)
		require.ErrorIs(t, err, ErrUnauthenticated)

		newHash := hashSessionToken(t, rotated)
		require.NoError(t, service.Logout(t.Context(), newHash))
		_, err = service.Authenticate(t.Context(), newHash)
		require.ErrorIs(t, err, ErrUnauthenticated)
	})

	t.Run("invalid session hashes", func(t *testing.T) {
		t.Parallel()

		store := &sessionStore{entries: make(map[string]sessionRecord)}
		service, err := New(store, Config{
			GarageURL:     "http://garage.example:3903",
			SessionSecret: base64.StdEncoding.EncodeToString(make([]byte, 32)),
		})
		require.NoError(t, err)

		_, err = service.Authenticate(t.Context(), nil)
		require.ErrorIs(t, err, ErrUnauthenticated)
		_, err = service.Authenticate(t.Context(), []byte("short"))
		require.ErrorIs(t, err, ErrUnauthenticated)
		require.NoError(t, service.Logout(t.Context(), nil))
	})
}
