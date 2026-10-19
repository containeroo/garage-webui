package postgres

import (
	"context"
	"fmt"
	"github.com/jackc/pgx/v5/pgxpool"
	"log/slog"
	"os"
	"testing"
	"time"
)

// TestLoginLimiter enforces limits using the database shared by all replicas.
func TestLoginLimiter(t *testing.T) {
	dsn := os.Getenv("GARAGE_WEBUI_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set GARAGE_WEBUI_TEST_DATABASE_URL for PostgreSQL integration tests")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	store := New(pool)
	if err := store.Migrate(ctx, slog.Default()); err != nil {
		t.Fatal(err)
	}
	peer := fmt.Sprintf("%s-%d", t.Name(), time.Now().UnixNano())
	for i := 0; i < loginAttemptsPerMinute; i++ {
		allowed, err := store.AllowLogin(ctx, peer)
		if err != nil || !allowed {
			t.Fatalf("attempt %d unexpectedly rejected: %v", i, err)
		}
	}
	allowed, err := store.AllowLogin(ctx, peer)
	if err != nil || allowed {
		t.Fatalf("excess attempt must be denied: %v", err)
	}
}
