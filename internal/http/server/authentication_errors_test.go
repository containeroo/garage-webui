package server

import (
	"context"
	"errors"
	"github.com/gi8lino/garage-webui/web"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	kit "github.com/containeroo/appkit/auth"
	appkitconfig "github.com/containeroo/appkit/config"
	"github.com/gi8lino/garage-webui/internal/application/authentication"
	"github.com/gi8lino/garage-webui/internal/http/auth"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type failingAuthStore struct {
	authentication.Repository
	passwordErr error
	sessionErr  error
	logoutErr   error
}

func (s *failingAuthStore) SetupRequired(context.Context) (bool, error) { return false, nil }
func (s *failingAuthStore) PasswordHash(context.Context, string) (string, error) {
	return "", s.passwordErr
}
func (s *failingAuthStore) Session(context.Context, []byte, string, bool) (string, string, bool, error) {
	return "", "", false, s.sessionErr
}
func (s *failingAuthStore) DeleteSession(context.Context, []byte) error         { return s.logoutErr }
func (s *failingAuthStore) SaveOIDCState(context.Context, []byte, string) error { return nil }
func (s *failingAuthStore) ConsumeOIDCState(context.Context, []byte) (string, error) {
	return "", nil
}

func TestAuthenticationFailuresReachClient(t *testing.T) {
	failure := errors.New("private database failure")
	store := &failingAuthStore{passwordErr: failure, sessionErr: failure, logoutErr: failure}
	mode := kit.ModeLocal
	service, err := auth.New(context.Background(), store, auth.Config{
		PublicURL:      "http://example.com",
		Authentication: appkitconfig.Authentication{ModeOverride: &mode},
	})
	require.NoError(t, err)
	handler := New(nil, service, nil, "", "http://example.com", "", slog.New(slog.NewTextHandler(io.Discard, nil)), false, web.Assets, testThemes).Handler()

	request := func(method, path, body string, cookie bool) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		r.Header.Set("Origin", "http://example.com")
		r.Header.Set("Content-Type", "application/json")
		if cookie {
			r.AddCookie(&http.Cookie{Name: "garage_webui_session", Value: "token"})
		}
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		return w
	}

	login := request(http.MethodPost, "/api/login", `{"username":"alice","password":"wrong"}`, false)
	assert.Equal(t, http.StatusServiceUnavailable, login.Code)
	assert.NotContains(t, login.Body.String(), failure.Error())
	store.passwordErr = kit.ErrUnauthenticated
	assert.Equal(t, http.StatusUnauthorized, request(http.MethodPost, "/api/login", `{"username":"missing","password":"wrong"}`, false).Code)

	for _, path := range []string{"/api/session", "/api/profile"} {
		w := request(http.MethodGet, path, "", true)
		assert.Equal(t, http.StatusServiceUnavailable, w.Code, path)
		assert.NotContains(t, w.Body.String(), failure.Error())
	}
	store.sessionErr = kit.ErrUnauthenticated
	assert.Equal(t, http.StatusUnauthorized, request(http.MethodGet, "/api/profile", "", true).Code)

	logout := request(http.MethodPost, "/api/logout", "{}", true)
	assert.Equal(t, http.StatusServiceUnavailable, logout.Code)
	assert.Empty(t, logout.Header().Values("Set-Cookie"), "keep the cookie so revocation can be retried")
	store.logoutErr = nil
	logout = request(http.MethodPost, "/api/logout", "{}", true)
	assert.Equal(t, http.StatusNoContent, logout.Code)
	assert.NotEmpty(t, logout.Header().Values("Set-Cookie"))
}
