package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	httpkitmiddleware "github.com/containeroo/httpkit/middleware"
	httpkitresponse "github.com/containeroo/httpkit/response"
	"github.com/stretchr/testify/assert"
)

func TestRequestContextAndAPIHeaders(t *testing.T) {
	t.Parallel()

	t.Run("/api/session", func(t *testing.T) {
		t.Parallel()

		handler := httpkitmiddleware.Chain(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, ok := w.(*httpkitresponse.RequestWriter)
			assert.True(t, ok)
			w.WriteHeader(http.StatusCreated)
		}), APIHeaders(), RequestContext())
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/session", nil))
		assert.Equal(t, "no-store", w.Header().Get("Cache-Control"))
		assert.Equal(t, http.StatusCreated, w.Code)
	})

	t.Run("/assets/app.js", func(t *testing.T) {
		t.Parallel()

		handler := httpkitmiddleware.Chain(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, ok := w.(*httpkitresponse.RequestWriter)
			assert.True(t, ok)
			w.WriteHeader(http.StatusCreated)
		}), APIHeaders(), RequestContext())
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/assets/app.js", nil))
		assert.Equal(t, "", w.Header().Get("Cache-Control"))
		assert.Equal(t, http.StatusCreated, w.Code)
	})
}
