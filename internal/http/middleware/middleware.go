// Package middleware provides HTTP policies for the web interface.
package middleware

import (
	"log/slog"
	"net/http"

	"github.com/containeroo/garage-webui/internal/http/auth"
)

// Policies holds the dependencies used by the HTTP middleware.
type Policies struct {
	// auth manages authenticated application sessions.
	auth *auth.Service
	// s3Origin identifies the S3 origin allowed by browser security policies.
	s3Origin string
	// logger records application diagnostics.
	logger *slog.Logger
	// validMutation validates the origin and CSRF requirements of mutation requests.
	validMutation func(http.ResponseWriter, *http.Request) bool
	// allowLogin checks whether the request is within its login attempt budget.
	allowLogin func(*http.Request) bool
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
