package middleware

import (
	"net/http"
	"net/url"
	"strings"

	httpresponse "github.com/containeroo/garage-webui/internal/http/response"
)

// sameOrigin verifies that the HTTP request Origin matches the configured public origin.
func sameOrigin(r *http.Request, publicURL string) bool {
	origin, err := url.Parse(r.Header.Get("Origin"))
	if err != nil || origin.Host == "" {
		return false
	}

	base, err := url.Parse(publicURL)
	return err == nil && origin.Scheme == base.Scheme && origin.Host == base.Host
}

// validMutation rejects cross-origin and non-JSON writes.
func validMutation(w http.ResponseWriter, r *http.Request, publicURL string) bool {
	if !sameOrigin(r, publicURL) || !strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
		httpresponse.Problem(w, http.StatusForbidden, "invalid request origin or content type")
		return false
	}

	return true
}

// RequireTransferOrigin protects streamed writes with the same strict origin policy.
func RequireTransferOrigin(publicURL string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodGet && !sameOrigin(r, publicURL) {
				httpresponse.Problem(w, http.StatusForbidden, "invalid transfer origin")
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
