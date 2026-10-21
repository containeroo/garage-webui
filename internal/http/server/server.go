// Package server wires application routes and HTTP middleware.
package server

import (
	"io/fs"
	"log/slog"

	"github.com/containeroo/garage-webui/internal/http/auth"
	"github.com/containeroo/garage-webui/internal/http/endpoint"
	"github.com/containeroo/garage-webui/internal/http/middleware"
	"github.com/containeroo/garage-webui/themes"
)

// Server combines authenticated routes and the static frontend.
type Server struct {
	// themes lists the available application themes.
	themes []themes.Theme
	// assets provides the embedded frontend files.
	assets fs.FS
	// s3Origin identifies the S3 origin allowed by browser security policies.
	s3Origin string
	// objects provides identity-scoped object operations.
	objects endpoint.ObjectServices
	// auth manages authenticated application sessions.
	auth *auth.Service
	// publicURL specifies the browser-facing application URL.
	publicURL string
	// prefix specifies the application route prefix.
	prefix string
	// logger records application diagnostics.
	logger *slog.Logger
	// accessLog enables HTTP access logging.
	accessLog bool
	// loginLimiter enforces the login attempt budget.
	loginLimiter middleware.LoginLimiter
	// transferOptions configures the internal S3 transport.
	transferOptions endpoint.TransferOptions
}

// New constructs the HTTP server.
func New(
	authentication *auth.Service,
	objectServices endpoint.ObjectServices,
	loginLimiter middleware.LoginLimiter,
	s3Origin string,
	publicURL, prefix string,
	logger *slog.Logger,
	accessLog bool,
	assets fs.FS,
	themeCatalog []themes.Theme,
	transferOptions endpoint.TransferOptions,
) *Server {
	return &Server{
		themes:          themeCatalog,
		transferOptions: transferOptions,
		assets:          assets,
		objects:         objectServices,
		loginLimiter:    loginLimiter,
		s3Origin:        s3Origin,
		auth:            authentication,
		publicURL:       publicURL,
		prefix:          prefix,
		logger:          logger,
		accessLog:       accessLog,
	}
}
