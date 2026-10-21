// Package bootstrap constructs application dependencies.
package bootstrap

import (
	"context"
	"fmt"
	"io/fs"
	"log/slog"
	"sync"

	"github.com/containeroo/garage-webui/internal/application/jobs"
	"github.com/containeroo/garage-webui/internal/database"
	"github.com/containeroo/garage-webui/internal/flags"
	"github.com/containeroo/garage-webui/internal/http/server"
	"github.com/containeroo/garage-webui/internal/storage"
	"github.com/containeroo/garage-webui/themes"
)

// Application owns initialized services and HTTP handlers.
type Application struct {
	// jobs coordinates durable background tasks.
	jobs *jobs.Service
	// stopJobs interrupts the background worker during shutdown.
	stopJobs context.CancelFunc
	// jobsDone waits until background work releases database resources.
	jobsDone sync.WaitGroup
	// db provides persistent application storage.
	db storage.Store
	// server serves the application HTTP endpoints.
	server *server.Server
	// stopCleanup cancels background authentication cleanup.
	stopCleanup context.CancelFunc
	// cleanupDone tracks background cleanup shutdown.
	cleanupDone sync.WaitGroup
}

// New constructs application services and starts background maintenance.
func New(ctx context.Context, cfg flags.Config, logger *slog.Logger, assets, themeFiles fs.FS) (*Application, error) {
	availableThemes, err := themes.Load(themeFiles)
	if err != nil {
		return nil, fmt.Errorf("load themes: %w", err)
	}

	db, err := database.Open(ctx, cfg.Database, logger)
	if err != nil {
		return nil, err
	}

	httpServer, backgroundJobs, err := newServer(cfg, logger, db, assets, availableThemes)
	if err != nil {
		db.Close()
		return nil, err
	}

	app := &Application{
		db:     db,
		jobs:   backgroundJobs,
		server: httpServer,
	}
	app.startCleanup(ctx, logger)
	workerCtx, stopJobs := context.WithCancel(ctx)
	app.stopJobs = stopJobs
	app.jobsDone.Add(1)
	go func() { defer app.jobsDone.Done(); backgroundJobs.Run(workerCtx) }()

	return app, nil
}

// Close releases infrastructure resources.
func (a *Application) Close() {
	a.stopJobs()
	a.jobsDone.Wait()
	a.stopCleanup()
	a.cleanupDone.Wait()
	a.db.Close()
}
