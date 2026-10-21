package bootstrap

import (
	"context"
	"log/slog"
	"time"
)

// startCleanup schedules maintenance until startup is canceled or Close is called.
func (a *Application) startCleanup(ctx context.Context, logger *slog.Logger) {
	cleanupCtx, stopCleanup := context.WithCancel(ctx)
	a.stopCleanup = stopCleanup
	a.cleanupDone.Add(1)
	go func() {
		defer a.cleanupDone.Done()
		a.runCleanup(cleanupCtx, logger)
	}()
}

// runCleanup cleans immediately, then repeats hourly until shutdown.
func (a *Application) runCleanup(ctx context.Context, logger *slog.Logger) {
	ticker := time.NewTicker(time.Hour)
	defer ticker.Stop()
	for {
		a.cleanupExpiredRecords(ctx, logger)
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// cleanupExpiredRecords bounds each maintenance pass and suppresses shutdown errors.
func (a *Application) cleanupExpiredRecords(ctx context.Context, logger *slog.Logger) {
	cleanupCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	if err := a.db.CleanupExpiredAuth(cleanupCtx); err != nil && ctx.Err() == nil {
		logger.Warn("authentication record cleanup failed", "error", err)
	}
	if err := a.db.CleanupExpiredLoginAttempts(cleanupCtx); err != nil && ctx.Err() == nil {
		logger.Warn("login limiter cleanup failed", "error", err)
	}
	if err := a.jobs.Cleanup(cleanupCtx); err != nil && ctx.Err() == nil {
		logger.Warn("deletion job cleanup failed", "error", err)
	}
}
