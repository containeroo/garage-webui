package middleware

import (
	"github.com/containeroo/garage-webui/internal/http/auth"
	httpresponse "github.com/containeroo/garage-webui/internal/http/response"
	"net/http"
	"strings"
)

// ValidMutation rejects cross-origin and non-JSON writes.
func ValidMutation(publicURL string) func(http.ResponseWriter, *http.Request) bool {
	return func(w http.ResponseWriter, r *http.Request) bool {
		if !auth.SameOrigin(r, publicURL) || !strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
			httpresponse.Error(w, "invalid request origin or content type", http.StatusForbidden)
			return false
		}

		return true
	}
}

// RequireTransferOrigin protects non-JSON streamed writes with the same strict origin policy.
func RequireTransferOrigin(publicURL string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodGet && !auth.SameOrigin(r, publicURL) {
				httpresponse.Error(w, "invalid transfer origin", http.StatusForbidden)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
