package middleware

import (
	"errors"
	"net/http"

	application "github.com/gi8lino/garage-webui/internal/application/garage"
	httpresponse "github.com/gi8lino/garage-webui/internal/http/response"
)

// GaragePolicy enforces the operation-specific role and method before mutations.
func (s *Policies) GaragePolicy(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, ok := RequestIdentity(r)
		if !ok {
			httpresponse.Error(w, "authentication required", http.StatusUnauthorized)
			return
		}
		err := s.garage.Validate(id.Role, r.Method, r.PathValue("operation"))
		switch {
		case errors.Is(err, application.ErrForbidden):
			if s.logger != nil {
				s.logger.Debug("garage operation denied", "event", "garage_operation_denied", "operation", r.PathValue("operation"))
			}
			httpresponse.Error(w, "insufficient permissions", http.StatusForbidden)
			return
		case errors.Is(err, application.ErrUnknownOperation):
			httpresponse.Error(w, "not found", http.StatusNotFound)
			return
		case errors.Is(err, application.ErrMethod):
			httpresponse.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		next.ServeHTTP(w, r)
	})
}
