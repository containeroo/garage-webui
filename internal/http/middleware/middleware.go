// Package middleware provides HTTP policies for the web interface.
package middleware

import (
	"log/slog"
	"net/http"

	"github.com/containeroo/garage-webui/internal/http/auth"
)

// Policies holds the dependencies used by the HTTP middleware.
type Policies struct {
	auth          *auth.Service
	s3Origin      string
	logger        *slog.Logger
	validMutation func(http.ResponseWriter, *http.Request) bool
	allowLogin    func(*http.Request) bool
}

// New constructs the middleware policies with their server dependencies.
func New(
	authentication *auth.Service,
	s3Origin string,
	logger *slog.Logger,
	validMutation func(http.ResponseWriter, *http.Request) bool,
	allowLogin func(*http.Request) bool,
) *Policies {
	return &Policies{
		auth:          authentication,
		s3Origin:      s3Origin,
		logger:        logger,
		validMutation: validMutation,
		allowLogin:    allowLogin,
	}
}
