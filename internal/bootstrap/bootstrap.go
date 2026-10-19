// Package bootstrap constructs application dependencies.
package bootstrap

import (
	"context"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/containeroo/garage-webui/internal/assembly"
	"github.com/containeroo/garage-webui/internal/flags"
	"github.com/containeroo/garage-webui/internal/http/auth"
	"github.com/containeroo/garage-webui/internal/http/server"
	"github.com/containeroo/garage-webui/internal/postgres"
	"github.com/containeroo/garage-webui/internal/s3"
	"github.com/containeroo/garage-webui/themes"
)

// Application owns initialized services and HTTP handlers.
type Application struct {
	db          *postgres.Store
	server      *server.Server
	stopCleanup context.CancelFunc
	cleanupDone sync.WaitGroup
}

// New initializes the database, authentication and Garage client.
func New(ctx context.Context, cfg flags.Config, logger *slog.Logger, assets, themeFiles fs.FS) (*Application, error) {
	availableThemes, err := themes.Load(themeFiles)
	if err != nil {
		return nil, fmt.Errorf("load themes: %w", err)
	}

	db, err := postgres.Open(ctx, cfg.Database.URL, cfg.Database.MaxConns, cfg.Database.MinIdleConns, logger)
	if err != nil {
		return nil, err
	}

	authentication, err := auth.New(db, auth.Config{
		PublicURL:     cfg.Server.PublicURL,
		RoutePrefix:   cfg.Server.RoutePrefix,
		GarageURL:     cfg.GarageURL,
		S3URL:         cfg.S3PublicURL,
		Region:        cfg.S3Region,
		SessionSecret: cfg.SessionSecret,
	})
	if err != nil {
		db.Close()
		return nil, err
	}

	if cfg.S3PublicURL != "" {
		if _, err := s3.New(cfg.S3PublicURL, cfg.S3Region, "validation", "validation", cfg.Server.PublicURL); err != nil {
			db.Close()
			return nil, err
		}
	}

	app := &Application{
		db: db,
		server: server.New(
			authentication,
			assembly.New(authentication, db, cfg.Server.PublicURL),
			db,
			cfg.S3PublicURL,
			cfg.Server.PublicURL,
			cfg.Server.RoutePrefix,
			logger,
			cfg.Logging.AccessLog,
			assets, availableThemes,
		),
	}
	cleanupCtx, stopCleanup := context.WithCancel(ctx)
	app.stopCleanup = stopCleanup
	app.cleanupDone.Add(1)
	go func() {
		defer app.cleanupDone.Done()
		ticker := time.NewTicker(time.Hour)
		defer ticker.Stop()
		for {
			cleanupTimeout, cancel := context.WithTimeout(cleanupCtx, 30*time.Second)
			err := db.CleanupExpiredAuth(cleanupTimeout)
			if limitErr := db.CleanupExpiredLoginAttempts(cleanupTimeout); limitErr != nil && cleanupCtx.Err() == nil {
				logger.Warn("login limiter cleanup failed", "error", limitErr)
			}

			cancel()
			if err != nil && cleanupCtx.Err() == nil {
				logger.Warn("authentication record cleanup failed", "error", err)
			}

			select {
			case <-cleanupCtx.Done():
				return
			case <-ticker.C:
			}
		}
	}()
	return app, nil
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

// Close releases infrastructure resources.
func (a *Application) Close() {
	a.stopCleanup()
	a.cleanupDone.Wait()
	a.db.Close()
}
