package postgres

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Store persists credential sessions and bucket signing-key identifiers.
type Store struct {
	// db owns the connection pool.
	db *pgxpool.Pool
}

// New creates a PostgreSQL store.
func New(db *pgxpool.Pool) *Store {
	return &Store{
		db: db,
	}
}

// Open connects to PostgreSQL and applies pending migrations.
func Open(ctx context.Context, url string, maxConns, minIdleConns int32, logger *slog.Logger) (*Store, error) {
	if logger == nil {
		return nil, errors.New("database logger is required")
	}

	if maxConns < 0 || minIdleConns < 0 {
		return nil, errors.New("database connection limits cannot be negative")
	}

	config, err := pgxpool.ParseConfig(url)
	if err != nil {
		return nil, errors.New("invalid database configuration")
	}

	if maxConns > 0 {
		config.MaxConns = maxConns
	}

	if minIdleConns > 0 {
		config.MinIdleConns = minIdleConns
	}

	if config.MinIdleConns > config.MaxConns {
		return nil, errors.New("database minimum idle connections exceed maximum connections")
	}

	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	store := New(pool)
	if err := store.Ping(ctx); err != nil {
		store.Close()
		return nil, fmt.Errorf("connect database: %w", err)
	}

	if err := store.Migrate(ctx, logger); err != nil {
		store.Close()
		return nil, fmt.Errorf("migrate database: %w", err)
	}

	return store, nil
}

// Ping checks database connectivity.
func (s *Store) Ping(ctx context.Context) error { return s.db.Ping(ctx) }

// Close releases database connections.
func (s *Store) Close() { s.db.Close() }
