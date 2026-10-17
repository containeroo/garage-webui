package middleware

import (
	"context"
	"errors"
	"net/http"

	kit "github.com/containeroo/appkit/auth"
	httpresponse "github.com/gi8lino/garage-webui/internal/http/response"
)

type identityKey struct{}

// Identity identifies the authenticated user for downstream handlers.
type Identity struct {
	User string
	Role string
}

// WithIdentity attaches an authenticated identity to a request.
func WithIdentity(r *http.Request, id Identity) *http.Request {
	return r.WithContext(context.WithValue(r.Context(), identityKey{}, id))
}

// RequestIdentity retrieves the authenticated identity from the request context.
func RequestIdentity(r *http.Request) (identity Identity, found bool) {
	id, ok := r.Context().Value(identityKey{}).(Identity)
	return id, ok
}

// Authenticate resolves the user once and passes their identity to the route.
func (s *Policies) Authenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.auth == nil {
			httpresponse.Error(w, "authentication required", http.StatusUnauthorized)
			return
		}
		user, role, err := s.auth.Authenticate(r)
		if err != nil {
			if !errors.Is(err, kit.ErrUnauthenticated) {
				if s.logger != nil {
					s.logger.Error("authentication failed", "error", err)
				}
				httpresponse.Error(w, "authentication service unavailable", http.StatusServiceUnavailable)
				return
			}
			if s.logger != nil {
				s.logger.Debug("authentication denied", "event", "authentication_denied", "method", r.Method)
			}
			httpresponse.Error(w, "authentication required", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, WithIdentity(r, Identity{User: user, Role: role}))
	})
}
