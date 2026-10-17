package server

import (
	"bytes"
	"github.com/gi8lino/garage-webui/web"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	httpkitmiddleware "github.com/containeroo/httpkit/middleware"
	"github.com/gi8lino/garage-webui/internal/http/middleware"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRoutePoliciesRejectUnauthenticatedRequests(t *testing.T) {
	s := New(nil, nil, nil, "", "http://example.com", "", nil, false, web.Assets, testThemes)
	handler := s.Handler()
	for _, route := range []struct{ method, path string }{
		{http.MethodGet, "/api/profile"},
		{http.MethodPut, "/api/profile"},
		{http.MethodPost, "/api/profile/password"},
		{http.MethodGet, "/api/users"},
		{http.MethodPost, "/api/groups"},
		{http.MethodGet, "/api/garage/GetClusterHealth"},
		{http.MethodPost, "/api/uploads/presign"},
		{http.MethodPost, "/api/objects/presign"},
	} {
		t.Run(route.method+" "+route.path, func(t *testing.T) {
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, httptest.NewRequest(route.method, route.path, nil))
			assert.Equal(t, http.StatusUnauthorized, response.Code)
			assert.Equal(t, "no-store", response.Header().Get("Cache-Control"))
			assert.Contains(t, response.Header().Get("Content-Type"), "application/json")
			assert.JSONEq(t, `{"error":"authentication required","problems":{}}`, response.Body.String())
		})
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/setup", nil))
	assert.Equal(t, http.StatusForbidden, response.Code, "public setup still validates origin")
}

func TestAdminAndMutationMiddlewareOrder(t *testing.T) {
	s := &Server{publicURL: "https://example.com"}
	called := false
	policies := middleware.New(s.garage, s.auth, s.s3Origin, s.logger, s.validMutation, s.allowLogin)
	handler := httpkitmiddleware.Chain(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { called = true }), policies.RequireAdmin, policies.Mutation)
	for _, tc := range []struct {
		role, origin string
		status       int
		wantCalled   bool
	}{
		{"viewer", "", http.StatusForbidden, false},
		{"admin", "https://evil.example", http.StatusForbidden, false},
		{"admin", "https://example.com", http.StatusOK, true},
	} {
		called = false
		r := httptest.NewRequest(http.MethodPost, "/api/users", nil)
		r.Header.Set("Origin", tc.origin)
		r.Header.Set("Content-Type", "application/json")
		r = middleware.WithIdentity(r, middleware.Identity{User: "test", Role: tc.role})
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		assert.Equal(t, tc.status, w.Code)
		assert.Equal(t, tc.wantCalled, called)
	}
}

func TestAccessLogIsOptionalAndOmitsQuery(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		t.Run(map[bool]string{false: "disabled", true: "enabled"}[enabled], func(t *testing.T) {
			var logs bytes.Buffer
			logger := slog.New(slog.NewJSONHandler(&logs, nil))
			s := New(nil, nil, nil, "", "http://example.com", "", logger, enabled, web.Assets, testThemes)
			w := httptest.NewRecorder()
			s.Handler().ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/missing?secret=do-not-log", nil))
			if !enabled {
				assert.Empty(t, logs.String())
				return
			}
			require.Contains(t, logs.String(), `"event":"request_complete"`)
			assert.Contains(t, logs.String(), `"status":404`)
			assert.False(t, strings.Contains(logs.String(), "do-not-log"))
		})
	}
}
