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

// Config describes the services and settings needed to serve the browser application.
type Config struct {
	// Authentication manages browser sessions and Garage token verification.
	Authentication *auth.Service
	// Objects provides identity-scoped object operations.
	Objects endpoint.ObjectServices
	// LoginLimiter applies shared login attempt restrictions.
	LoginLimiter middleware.LoginLimiter
	// S3Origin is the browser-accessible S3 endpoint for direct transfers.
	S3Origin string
	// PublicURL identifies the console's public origin.
	PublicURL string
	// RoutePrefix sets the base path for the application.
	RoutePrefix string
	// Logger records request and diagnostic details.
	Logger *slog.Logger
	// AccessLog enables HTTP request logging.
	AccessLog bool
	// Assets provides the embedded frontend.
	Assets fs.FS
	// Themes lists the available application themes.
	Themes []themes.Theme
	// TransferOptions configures backend S3 transport behavior.
	TransferOptions endpoint.TransferOptions
}

// New constructs the HTTP server from documented dependencies.
func New(cfg Config) *Server {
	return &Server{
		themes:          cfg.Themes,
		transferOptions: cfg.TransferOptions,
		assets:          cfg.Assets,
		objects:         cfg.Objects,
		loginLimiter:    cfg.LoginLimiter,
		s3Origin:        cfg.S3Origin,
		auth:            cfg.Authentication,
		publicURL:       cfg.PublicURL,
		prefix:          cfg.RoutePrefix,
		logger:          cfg.Logger,
		accessLog:       cfg.AccessLog,
	}
}
