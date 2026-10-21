package server

import (
	"io/fs"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestFrontendFallbackRouting verifies ServeMux dispatches browser paths to the SPA.
func TestFrontendFallbackRouting(t *testing.T) {
	t.Parallel()

	assets := fstest.MapFS{
		"index.html":    &fstest.MapFile{Data: []byte(`<html><head><base href="/" /></head><body>garage app</body></html>`)},
		"404.html":      &fstest.MapFile{Data: []byte(`<html><head><base href="/" /></head><body>not found</body></html>`)},
		"assets/app.js": &fstest.MapFile{Data: []byte(`console.log("garage")`)},
	}
	var _ fs.FS = assets
	handler := New(Config{Assets: assets, Logger: slog.Default()}).Handler()

	t.Run("root serves index", func(t *testing.T) {
		t.Parallel()

		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/", nil))
		require.Equal(t, http.StatusOK, recorder.Code)
		assert.Contains(t, recorder.Body.String(), "garage app")
	})

	t.Run("login serves index", func(t *testing.T) {
		t.Parallel()

		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/login", nil))
		require.Equal(t, http.StatusOK, recorder.Code)
		assert.Contains(t, recorder.Body.String(), "garage app")
	})

	t.Run("static assets are served", func(t *testing.T) {
		t.Parallel()

		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/assets/app.js", nil))
		require.Equal(t, http.StatusOK, recorder.Code)
		assert.Contains(t, recorder.Body.String(), `console.log("garage")`)
	})

	t.Run("unknown browser page serves custom not found", func(t *testing.T) {
		t.Parallel()

		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodGet, "/does-not-exist", nil)
		request.Header.Set("Accept", "text/html")
		handler.ServeHTTP(recorder, request)
		require.Equal(t, http.StatusNotFound, recorder.Code)
		assert.Contains(t, recorder.Body.String(), "not found")
	})

	t.Run("unknown api stays not found", func(t *testing.T) {
		t.Parallel()

		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodGet, "/api/nonexistent", nil)
		handler.ServeHTTP(recorder, request)
		assert.Equal(t, http.StatusNotFound, recorder.Code)
		assert.NotContains(t, recorder.Body.String(), "garage app")
	})

	t.Run("base URL uses deployment prefix", func(t *testing.T) {
		t.Parallel()

		prefixed := New(Config{Assets: assets, Logger: slog.Default(), RoutePrefix: "/garage"}).Handler()
		recorder := httptest.NewRecorder()
		prefixed.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/", nil))
		require.Equal(t, http.StatusOK, recorder.Code)
		assert.Contains(t, recorder.Body.String(), `<base href="/garage/">`)
	})
}
