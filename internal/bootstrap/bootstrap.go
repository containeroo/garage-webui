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

	applicationgarage "github.com/gi8lino/garage-webui/internal/application/garage"
	"github.com/gi8lino/garage-webui/internal/application/objects"
	"github.com/gi8lino/garage-webui/internal/flags"
	"github.com/gi8lino/garage-webui/internal/garage"
	"github.com/gi8lino/garage-webui/internal/garage/credentials"
	"github.com/gi8lino/garage-webui/internal/http/auth"
	"github.com/gi8lino/garage-webui/internal/http/server"
	"github.com/gi8lino/garage-webui/internal/postgres"
	"github.com/gi8lino/garage-webui/internal/s3"
	"github.com/gi8lino/garage-webui/themes"
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
	client, err := garage.New(cfg.GarageURL, cfg.GarageToken)
	if err != nil {
		return nil, err
	}

	db, err := postgres.Open(ctx, cfg.Database.URL, cfg.Database.MaxConns, cfg.Database.MinIdleConns, logger)
	if err != nil {
		return nil, err
	}

	keys := credentials.New(client, db, cfg.Server.PublicURL)
	uploader, err := s3.NewManager(keys, cfg.S3PublicURL, cfg.S3Region, cfg.Server.PublicURL)
	if err != nil {
		db.Close()
		return nil, err
	}

	authentication, err := auth.New(ctx, db, auth.Config{
		PublicURL:      cfg.Server.PublicURL,
		RoutePrefix:    cfg.Server.RoutePrefix,
		Authentication: cfg.Authentication,
		Proxy:          cfg.Proxy,
		OIDC:           cfg.OIDC,
	})
	if err != nil {
		db.Close()
		return nil, err
	}

	var objectService *objects.Service
	var origin string
	if uploader != nil {
		objectService = objects.New(client, uploader)
		origin = uploader.Origin()
	}

	app := &Application{
		db: db,
		server: server.New(
			applicationgarage.New(client),
			authentication,
			objectService,
			origin,
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
