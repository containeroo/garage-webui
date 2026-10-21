package middleware

import (
	"context"
	"errors"
	"net/http"

	"github.com/containeroo/garage-webui/internal/http/auth"
	httpresponse "github.com/containeroo/garage-webui/internal/http/response"
)

// identityKey is the private request-context key for authenticated identities.
type identityKey struct{}

// WithIdentity returns a request carrying the verified identity in its context.
func WithIdentity(r *http.Request, id auth.Identity) *http.Request {
	return r.WithContext(context.WithValue(r.Context(), identityKey{}, id))
}

// RequestIdentity retrieves the authenticated identity from the request context.
func RequestIdentity(r *http.Request) (identity auth.Identity, found bool) {
	id, ok := r.Context().Value(identityKey{}).(auth.Identity)
	return id, ok
}

// Authenticate verifies the session and attaches its identity before invoking the next handler.
func (s *Policies) Authenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.auth == nil {
			httpresponse.Problem(w, http.StatusUnauthorized, "authentication required")
			return
		}

		id, err := s.auth.Authenticate(r)
		if err != nil {
			status := 503
			if errors.Is(err, auth.ErrUnauthenticated) {
				status = 401
			}

			httpresponse.Problem(w, status, "authentication required")

			return
		}

		next.ServeHTTP(w, WithIdentity(r, id))
	})
}
