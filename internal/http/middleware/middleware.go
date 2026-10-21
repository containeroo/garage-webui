// Package middleware provides HTTP policies for the web interface.
package middleware

import (
	"log/slog"

	"github.com/containeroo/garage-webui/internal/http/auth"
)

// Policies holds the dependencies used by HTTP access policies.
type Policies struct {
	// auth manages authenticated browser sessions.
	auth *auth.Service
	// s3Origin identifies the S3 origin allowed by browser security policies.
	s3Origin string
	// publicURL identifies the allowed origin of browser mutations.
	publicURL string
	// limiter controls login attempt budgets across instances.
	limiter LoginLimiter
	// logger records application diagnostics.
	logger *slog.Logger
}

// New constructs HTTP policies from their real dependencies.
func New(authentication *auth.Service, s3Origin, publicURL string, limiter LoginLimiter, logger *slog.Logger) *Policies {
	return &Policies{
		auth:      authentication,
		s3Origin:  s3Origin,
		publicURL: publicURL,
		limiter:   limiter,
		logger:    logger,
	}
}
