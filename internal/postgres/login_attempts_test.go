package postgres

import (
	"context"
	"fmt"
	"github.com/jackc/pgx/v5/pgxpool"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestLoginLimiter enforces limits using the database shared by all replicas.
func TestLoginLimiter(t *testing.T) {
	t.Parallel()

	dsn := os.Getenv("GARAGE_WEBUI_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set GARAGE_WEBUI_TEST_DATABASE_URL for PostgreSQL integration tests")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	require.NoError(t, err)
	defer pool.Close()
	store := New(pool)
	{
		err := store.Migrate(ctx, slog.Default())
		require.NoError(t, err)
	}
	peer := fmt.Sprintf("%s-%d", t.Name(), time.Now().UnixNano())
	for i := 0; i < loginAttemptsPerMinute; i++ {
		allowed, err := store.AllowLogin(ctx, peer)
		require.NoError(t, err, "login attempt %d", i)
		assert.True(t, allowed, "attempt %d must be allowed within the budget", i)
	}
	allowed, err := store.AllowLogin(ctx, peer)
	require.NoError(t, err)
	assert.False(t, allowed, "excess attempt must be denied")
}
