package middleware

import (
	"context"
	"errors"
	"net/http"

	"github.com/containeroo/garage-webui/internal/http/auth"
	httpresponse "github.com/containeroo/garage-webui/internal/http/response"
)

type identityKey struct{}

func WithIdentity(r *http.Request, id auth.Identity) *http.Request {
	return r.WithContext(context.WithValue(r.Context(), identityKey{}, id))
}

func RequestIdentity(r *http.Request) (identity auth.Identity, found bool) {
	id, ok := r.Context().Value(identityKey{}).(auth.Identity)
	return id, ok
}

func (s *Policies) Authenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.auth == nil {
			httpresponse.Error(w, "authentication required", http.StatusUnauthorized)
			return
		}

		id, err := s.auth.Authenticate(r)
		if err != nil {
			status := 503
			if errors.Is(err, auth.ErrUnauthenticated) {
				status = 401
			}

			httpresponse.Error(w, "authentication required", status)

			return
		}

		next.ServeHTTP(w, WithIdentity(r, id))
	})
}
