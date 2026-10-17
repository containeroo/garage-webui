package middleware

import (
	"net/http"

	httpresponse "github.com/gi8lino/garage-webui/internal/http/response"
)

// RequireAdmin restricts the route to authenticated administrators.
func (s *Policies) RequireAdmin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, ok := RequestIdentity(r)
		if !ok {
			httpresponse.Error(w, "authentication required", http.StatusUnauthorized)
			return
		}
		if id.Role != "admin" {
			if s.logger != nil {
				s.logger.Debug("authorization denied", "event", "authorization_denied", "required_role", "admin")
			}
			httpresponse.Error(w, "administrator access required", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}
