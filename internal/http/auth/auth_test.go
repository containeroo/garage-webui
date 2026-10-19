package auth

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type testRecord struct {
	credential []byte
	expires    time.Time
	theme      string
}
type testStore struct{ records map[string]testRecord }

func (s *testStore) SaveSession(_ context.Context, h, c []byte, e time.Time) error {
	s.records[string(h)] = testRecord{c, e, "Light"}
	return nil
}
func (s *testStore) LoadSession(_ context.Context, h []byte) ([]byte, string, error) {
	v, ok := s.records[string(h)]
	if !ok || v.expires.Before(time.Now()) {
		return nil, "", pgx.ErrNoRows
	}
	return v.credential, v.theme, nil
}
func (s *testStore) DeleteSession(_ context.Context, h []byte) error {
	delete(s.records, string(h))
	return nil
}
func (s *testStore) SetSessionTheme(_ context.Context, h []byte, theme string) error {
	v := s.records[string(h)]
	v.theme = theme
	s.records[string(h)] = v
	return nil
}
func TestCredentialSessions(t *testing.T) {
	revoked := false
	scope := []string{"ListBuckets"}
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if revoked || r.Header.Get("Authorization") != "Bearer individual-token" {
			w.WriteHeader(401)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"name": "alice", "scope": scope, "expired": false})
	}))
	defer upstream.Close()
	store := &testStore{records: map[string]testRecord{}}
	cfg := Config{PublicURL: "https://console.example", GarageURL: upstream.URL, SessionSecret: base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{42}, 32)), RoutePrefix: "/console"}
	service, err := New(store, cfg)
	require.NoError(t, err)
	response := httptest.NewRecorder()
	request := httptest.NewRequest("POST", "/api/login", nil)
	require.NoError(t, service.Login(response, request, Credential{Kind: "admin", Token: "individual-token"}))
	cookie := response.Result().Cookies()[0]
	assert.True(t, cookie.HttpOnly)
	assert.True(t, cookie.Secure)
	assert.Equal(t, "/console/", cookie.Path)
	for _, record := range store.records {
		assert.NotContains(t, string(record.credential), "individual-token")
	}
	assert.NotContains(t, cookie.Value, "individual-token")
	current := httptest.NewRequest("GET", "/api/session", nil)
	current.AddCookie(cookie)
	// A second replica with the same encryption key can load and revoke the session.
	replica, err := New(store, cfg)
	require.NoError(t, err)
	id, err := replica.Authenticate(current)
	require.NoError(t, err)
	assert.Equal(t, "alice", id.User)
	assert.True(t, id.Allows("ListBuckets"))
	assert.False(t, id.Allows("CreateKey"))
	scope = []string{"CreateKey"}
	id, err = service.Authenticate(current)
	require.NoError(t, err)
	assert.True(t, id.Allows("CreateKey"))
	assert.False(t, id.Allows("ListBuckets"))
	revoked = true
	_, err = service.Authenticate(current)
	require.ErrorIs(t, err, ErrUnauthenticated)
	revoked = false
	require.NoError(t, replica.Logout(httptest.NewRecorder(), current))
	_, err = service.Authenticate(current)
	require.ErrorIs(t, err, ErrUnauthenticated)
	require.ErrorIs(t, service.Login(httptest.NewRecorder(), request, Credential{Kind: "local", Token: "individual-token"}), ErrUnauthenticated)
}
func TestSessionTamperingAndExpiration(t *testing.T) {
	store := &testStore{records: map[string]testRecord{}}
	cfg := Config{PublicURL: "http://console.example", GarageURL: "http://garage:3903", SessionSecret: base64.StdEncoding.EncodeToString(make([]byte, 32))}
	service, err := New(store, cfg)
	require.NoError(t, err)
	req := httptest.NewRequest("GET", "/api/session", nil)
	req.AddCookie(&http.Cookie{Name: cookieName, Value: base64.RawURLEncoding.EncodeToString(make([]byte, 32))})
	h := sessionHash(req)
	store.records[string(h)] = testRecord{credential: bytes.Repeat([]byte{1}, 80), expires: time.Now().Add(time.Hour)}
	_, err = service.Authenticate(req)
	assert.ErrorIs(t, err, ErrUnauthenticated)
	store.records[string(h)] = testRecord{expires: time.Now().Add(-time.Minute)}
	_, err = service.Authenticate(req)
	assert.ErrorIs(t, err, ErrUnauthenticated)
	cfg.SessionSecret = "bad"
	_, err = New(store, cfg)
	require.Error(t, err)
}
