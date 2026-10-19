package middleware

import (
	"net/http"
	"strings"

	httpkitmiddleware "github.com/containeroo/httpkit/middleware"
)

// APIHeaders disables caching for API responses.
func APIHeaders() httpkitmiddleware.Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if strings.HasPrefix(r.URL.Path, "/api/") {
				w.Header().Set("Cache-Control", "no-store")
			}

			next.ServeHTTP(w, r)
		})
	}
}
