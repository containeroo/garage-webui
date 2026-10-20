package database

import (
	"context"
	"log/slog"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSQLiteSelection(t *testing.T) {
	t.Parallel()

	cfg := Config{URL: "sqlite:" + filepath.Join(t.TempDir(), "console.db")}
	store, err := Open(context.Background(), cfg, slog.Default())
	require.NoError(t, err)
	defer store.Close()
	require.NoError(t, store.Ping(context.Background()))
	cfg.MaxConns = 2
	_, err = Open(context.Background(), cfg, slog.Default())
	require.ErrorContains(t, err, "only to PostgreSQL")
}

func TestInvalidDatabaseURL(t *testing.T) {
	t.Parallel()

	t.Run("empty URL", func(t *testing.T) {
		t.Parallel()

		value := ""
		_, err := Open(context.Background(), Config{URL: value}, slog.Default())
		require.Error(t, err)
	})

	t.Run("mysql://localhost/database", func(t *testing.T) {
		t.Parallel()

		value := "mysql://localhost/database"
		_, err := Open(context.Background(), Config{URL: value}, slog.Default())
		require.Error(t, err)
	})

	t.Run("postgres://%invalid", func(t *testing.T) {
		t.Parallel()

		value := "postgres://%invalid"
		_, err := Open(context.Background(), Config{URL: value}, slog.Default())
		require.Error(t, err)
	})
}
