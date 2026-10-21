package bootstrap

import (
	"io/fs"
	"log/slog"
	"net/http"

	"github.com/containeroo/garage-webui/internal/application/jobs"
	"github.com/containeroo/garage-webui/internal/assembly"
	"github.com/containeroo/garage-webui/internal/flags"
	"github.com/containeroo/garage-webui/internal/http/auth"
	"github.com/containeroo/garage-webui/internal/http/endpoint"
	"github.com/containeroo/garage-webui/internal/http/server"
	"github.com/containeroo/garage-webui/internal/s3"
	"github.com/containeroo/garage-webui/internal/storage"
	"github.com/containeroo/garage-webui/themes"
)

// newServer wires authentication, object services and HTTP handlers.
// The caller retains ownership of the database on both success and failure.
func newServer(
	cfg flags.Config,
	logger *slog.Logger,
	db storage.Store,
	assets fs.FS,
	availableThemes []themes.Theme,
) (*server.Server, *jobs.Service, error) {
	authentication, err := auth.New(db, auth.Config{
		PublicURL:     cfg.Server.PublicURL,
		RoutePrefix:   cfg.Server.RoutePrefix,
		GarageURL:     cfg.GarageURL,
		S3URL:         cfg.S3URL,
		S3PublicURL:   cfg.S3PublicURL,
		Region:        cfg.S3Region,
		SessionSecret: cfg.SessionSecret,
	})
	if err != nil {
		return nil, nil, err
	}

	if authentication.S3URL != "" {
		if _, err := s3.NewClient(authentication.S3URL, cfg.S3Region, "validation", "validation"); err != nil {
			return nil, nil, err
		}
	}

	if cfg.S3PublicURL != "" {
		if _, err := s3.New(cfg.S3PublicURL, cfg.S3Region, "validation", "validation", cfg.Server.PublicURL); err != nil {
			return nil, nil, err
		}
	}

	objectsFactory := assembly.New(authentication.Service, db, cfg.Server.PublicURL)
	backgroundJobs, err := jobs.New(db, authentication.Service, objectsFactory, logger)
	if err != nil {
		return nil, nil, err
	}

	return server.New(server.Config{
		Jobs:            backgroundJobs,
		Authentication:  authentication,
		Objects:         objectsFactory,
		LoginLimiter:    db,
		S3Origin:        cfg.S3PublicURL,
		PublicURL:       cfg.Server.PublicURL,
		RoutePrefix:     cfg.Server.RoutePrefix,
		Logger:          logger,
		AccessLog:       cfg.Logging.AccessLog,
		Assets:          assets,
		Themes:          availableThemes,
		TransferOptions: endpoint.TransferOptions{ResponseHeaderTimeout: cfg.S3ResponseHeaderTimeout},
	}), backgroundJobs, nil
}

// Handler returns the application handler.
func (a *Application) Handler() http.Handler { return a.server.Handler() }

// Management returns health endpoints.
func (a *Application) Management() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, r *http.Request) {
		if err := a.db.Ping(r.Context()); err != nil {
			http.Error(w, "not ready", http.StatusServiceUnavailable)
			return
		}

		w.WriteHeader(http.StatusOK)
	})

	return mux
}
