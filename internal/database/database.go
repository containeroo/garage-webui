// Package database selects the configured persistence implementation.
package database

import (
	"context"
	"errors"
	"log/slog"
	"net/url"

	"github.com/containeroo/garage-webui/internal/postgres"
	"github.com/containeroo/garage-webui/internal/sqlite"
	"github.com/containeroo/garage-webui/internal/storage"
)

// Config configures a SQLite file or PostgreSQL database.
type Config struct {
	URL string
	// MaxConns and MinIdleConns configure only the PostgreSQL connection pool.
	MaxConns     int32
	MinIdleConns int32
}

// Open initializes the backend selected by the database URL scheme.
func Open(ctx context.Context, cfg Config, logger *slog.Logger) (storage.Store, error) {
	u, err := url.Parse(cfg.URL)
	if err != nil {
		return nil, errors.New("invalid database URL")
	}
	switch u.Scheme {
	case "postgres", "postgresql":
		return postgres.Open(ctx, cfg.URL, cfg.MaxConns, cfg.MinIdleConns, logger)
	case "sqlite":
		if cfg.MaxConns != 0 || cfg.MinIdleConns != 0 {
			return nil, errors.New("database pool settings apply only to PostgreSQL")
		}
		return sqlite.Open(ctx, cfg.URL, logger)
	default:
		return nil, errors.New("database URL must use postgres://, postgresql:// or sqlite:///")
	}
}
