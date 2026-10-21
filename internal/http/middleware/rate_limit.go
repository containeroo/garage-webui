package middleware

import (
	"context"
	"net"
	"net/http"

	httpresponse "github.com/containeroo/garage-webui/internal/http/response"
	httpkitmiddleware "github.com/containeroo/httpkit/middleware"
)

// LoginLimiter authorizes login attempts across application replicas.
type LoginLimiter interface {
	// AllowLogin records a login attempt and reports whether the peer remains within its budget.
	AllowLogin(context.Context, string) (bool, error)
}

// LimitLogin rate-limits password attempts and optionally sets Retry-After.
func (s *Policies) LimitLogin(retryAfter bool) httpkitmiddleware.Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !s.allowLogin(r) {
				if retryAfter {
					w.Header().Set("Retry-After", "60")
				}

				s.logger.Debug("Garage sign-in attempt rate limited", "event", "password_rate_limited")

				message := "too many attempts"
				if r.URL.Path == "/api/login" {
					message = "too many login attempts"
				}

				httpresponse.Problem(w, http.StatusTooManyRequests, message)

				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// allowLogin checks the immediate TCP peer without trusting forwarded headers.
func (s *Policies) allowLogin(r *http.Request) bool {
	peer, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		peer = r.RemoteAddr
	}

	permitted, err := s.limiter.AllowLogin(r.Context(), peer)
	if err != nil {
		s.logger.Error("login rate limit unavailable", "error", err)
		return false
	}

	return permitted
}
