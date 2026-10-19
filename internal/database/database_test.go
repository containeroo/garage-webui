package database

import (
	"context"
	"log/slog"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSQLiteSelection(t *testing.T) {
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
	for _, value := range []string{"", "mysql://localhost/database", "postgres://%invalid"} {
		t.Run(value, func(t *testing.T) {
			_, err := Open(context.Background(), Config{URL: value}, slog.Default())
			require.Error(t, err)
		})
	}
}
