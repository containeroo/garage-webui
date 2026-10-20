// Package bootstrap constructs application dependencies.
package bootstrap

import (
	"context"
	"fmt"
	"io/fs"
	"log/slog"
	"sync"

	"github.com/containeroo/garage-webui/internal/database"
	"github.com/containeroo/garage-webui/internal/flags"
	"github.com/containeroo/garage-webui/internal/http/server"
	"github.com/containeroo/garage-webui/internal/storage"
	"github.com/containeroo/garage-webui/themes"
)

// Application owns initialized services and HTTP handlers.
type Application struct {
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

	httpServer, err := newServer(cfg, logger, db, assets, availableThemes)
	if err != nil {
		db.Close()
		return nil, err
	}

	app := &Application{
		db:     db,
		server: httpServer,
	}
	app.startCleanup(ctx, logger)

	return app, nil
}

// Close releases infrastructure resources.
func (a *Application) Close() {
	a.stopCleanup()
	a.cleanupDone.Wait()
	a.db.Close()
}
