// Package sqlite persists sessions, login budgets and signing keys in a local SQLite file.
package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"path/filepath"
	"sync"

	"github.com/gofrs/flock"
	_ "modernc.org/sqlite"
)

// Store owns a single-connection SQLite database and per-bucket provisioning locks.
// A process lock prevents multiple instances from provisioning keys against the same file.
type Store struct {
	// db provides the SQLite database connection.
	db *sql.DB
	// processLock prevents concurrent processes from opening the database file.
	processLock *flock.Flock
	// logger records application diagnostics.
	logger *slog.Logger
	// mu protects bucket gate ownership and reference counts.
	mu sync.Mutex
	// buckets tracks provisioning gates for active bucket operations.
	buckets map[string]*bucketGate
	// closeOnce ensures database resources are closed only once.
	closeOnce sync.Once
}

// Open creates or reopens a persistent SQLite database and applies its schema.
func Open(ctx context.Context, rawURL string, logger *slog.Logger) (*Store, error) {
	if logger == nil {
		return nil, errors.New("database logger is required")
	}

	filename, err := databasePath(rawURL)
	if err != nil {
		return nil, err
	}

	if err := os.MkdirAll(filepath.Dir(filename), 0o700); err != nil {
		return nil, fmt.Errorf("create SQLite directory: %w", err)
	}

	// Create the file privately before SQLite opens it; existing file modes are preserved.
	file, err := os.OpenFile(filename, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("create SQLite database: %w", err)
	}
	if err := file.Close(); err != nil {
		return nil, err
	}

	filename, err = filepath.EvalSymlinks(filename)
	if err != nil {
		return nil, err
	}

	processLock := flock.New(filename+".lock", flock.SetPermissions(0o600))

	locked, err := processLock.TryLock()
	if err != nil || !locked {
		_ = processLock.Close()
		if err != nil {
			return nil, fmt.Errorf("lock SQLite database: %w", err)
		}
		return nil, errors.New("SQLite database is already in use; run one Web UI instance per SQLite database")
	}

	uri := url.URL{
		Scheme: "file",
		Path:   filename,
	}
	pragmas := url.Values{
		"_pragma": {
			"busy_timeout(5000)",
			"journal_mode(WAL)",
			"synchronous(FULL)",
			"foreign_keys(1)",
		},
	}
	uri.RawQuery = pragmas.Encode()

	db, err := sql.Open("sqlite", uri.String())
	if err != nil {
		_ = processLock.Close()
		return nil, fmt.Errorf("open SQLite: %w", err)
	}
	// SQLite allows one writer. Queue short writes through one connection while
	// per-bucket locks remain independent of database transactions and Garage calls.
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	store := &Store{db: db, processLock: processLock, logger: logger, buckets: map[string]*bucketGate{}}
	if err := db.PingContext(ctx); err != nil {
		store.Close()
		return nil, fmt.Errorf("connect SQLite: %w", err)
	}

	if err := store.migrate(ctx); err != nil {
		store.Close()
		return nil, fmt.Errorf("migrate SQLite: %w", err)
	}

	return store, nil
}

// databasePath accepts sqlite:///absolute/path and sqlite:relative/path URLs.
func databasePath(rawURL string) (string, error) {
	u, err := url.Parse(rawURL)
	if err != nil || u.Scheme != "sqlite" || u.Host != "" || u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" {
		return "", errors.New("SQLite URL must be sqlite:///absolute/path or sqlite:relative/path, without URL options")
	}
	path := u.Path
	if u.Opaque != "" {
		path = u.Opaque
	}
	if path == "" || path == ":memory:" {
		return "", errors.New("SQLite requires a persistent database file")
	}
	return filepath.Abs(path)
}

// Ping checks database availability for readiness probes.
func (s *Store) Ping(ctx context.Context) error { return s.db.PingContext(ctx) }

// Close closes the database before releasing the exclusive process lock.
func (s *Store) Close() {
	s.closeOnce.Do(func() {
		if err := s.db.Close(); err != nil {
			s.logger.Warn("close SQLite database", "error", err)
		}
		if err := s.processLock.Close(); err != nil {
			s.logger.Warn("release SQLite process lock", "error", err)
		}
	})
}
