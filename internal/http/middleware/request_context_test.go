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
	for _, tc := range []struct {
		path, cache string
	}{
		{"/api/session", "no-store"},
		{"/assets/app.js", ""},
	} {
		t.Run(tc.path, func(t *testing.T) {
			handler := httpkitmiddleware.Chain(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				_, ok := w.(*httpkitresponse.RequestWriter)
				assert.True(t, ok)
				w.WriteHeader(http.StatusCreated)
			}), APIHeaders(), RequestContext())
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, httptest.NewRequest(http.MethodGet, tc.path, nil))
			assert.Equal(t, tc.cache, w.Header().Get("Cache-Control"))
			assert.Equal(t, http.StatusCreated, w.Code)
		})
	}
}
