package middleware

import (
	"net/http"

	httpkitmiddleware "github.com/containeroo/httpkit/middleware"
	httpresponse "github.com/gi8lino/garage-webui/internal/http/response"
)

// LimitLogin rate-limits password attempts and optionally sets Retry-After.
func (s *Policies) LimitLogin(retryAfter bool) httpkitmiddleware.Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !s.allowLogin(r) {
				if retryAfter {
					w.Header().Set("Retry-After", "60")
				}
				if s.logger != nil {
					s.logger.Debug("password attempt rate limited", "event", "password_rate_limited")
				}
				message := "too many attempts"
				if r.URL.Path == "/api/login" {
					message = "too many login attempts"
				}
				httpresponse.Error(w, message, http.StatusTooManyRequests)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
