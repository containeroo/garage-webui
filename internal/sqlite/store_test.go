package sqlite

import (
	"context"
	"errors"
	"log/slog"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/containeroo/garage-webui/internal/signing"
	"github.com/containeroo/garage-webui/internal/storage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var _ storage.Store = (*Store)(nil)

func openTestStore(t *testing.T) (store *Store, databaseURL string) {
	t.Helper()
	url := "sqlite:" + filepath.Join(t.TempDir(), "console.db")
	var err error
	store, err = Open(context.Background(), url, slog.Default())
	require.NoError(t, err)
	t.Cleanup(store.Close)
	return store, url
}

func TestSessionsPersistAndExpire(t *testing.T) {
	t.Parallel()

	store, url := openTestStore(t)
	ctx := context.Background()
	hash := []byte("session-hash")
	credential := []byte("encrypted-credential")
	require.NoError(t, store.SaveSession(ctx, hash, credential, time.Now().Add(time.Hour)))
	require.NoError(t, store.SetSessionTheme(ctx, hash, "Dark"))
	store.Close()
	reopened, err := Open(ctx, url, slog.Default())
	require.NoError(t, err)
	defer reopened.Close()
	saved, theme, err := reopened.LoadSession(ctx, hash)
	require.NoError(t, err)
	assert.Equal(t, credential, saved)
	assert.Equal(t, "Dark", theme)
	require.NoError(t, reopened.DeleteSession(ctx, hash))
	_, _, err = reopened.LoadSession(ctx, hash)
	assert.ErrorIs(t, err, storage.ErrSessionNotFound)
	expired := []byte("expired")
	require.NoError(t, reopened.SaveSession(ctx, expired, credential, time.Now().Add(-time.Second)))
	_, _, err = reopened.LoadSession(ctx, expired)
	assert.ErrorIs(t, err, storage.ErrSessionNotFound)
	require.NoError(t, reopened.CleanupExpiredAuth(ctx))
	var count int
	require.NoError(t, reopened.db.QueryRowContext(ctx, `SELECT count(*) FROM webui_sessions`).Scan(&count))
	assert.Zero(t, count)
	require.NoError(t, reopened.Ping(ctx))
}

func TestExclusiveProcessOwnership(t *testing.T) {
	t.Parallel()

	store, url := openTestStore(t)
	_, err := Open(context.Background(), url, slog.Default())
	require.ErrorContains(t, err, "already in use")
	store.Close()
	reopened, err := Open(context.Background(), url, slog.Default())
	require.NoError(t, err)
	reopened.Close()
	reopened.Close()
}

func TestBucketLocksAndRecovery(t *testing.T) {
	t.Parallel()

	store, url := openTestStore(t)
	ctx := context.Background()
	first, err := store.LockBucket(ctx, "bucket")
	require.NoError(t, err)
	other, err := store.LockBucket(ctx, "other")
	require.NoError(t, err)
	require.NoError(t, other.Close())
	waiting, cancel := context.WithTimeout(ctx, 30*time.Millisecond)
	defer cancel()
	_, err = store.LockBucket(waiting, "bucket")
	require.ErrorIs(t, err, context.DeadlineExceeded)
	canceled, cancelRequest := context.WithCancel(ctx)
	cancelRequest()
	require.NoError(t, first.SaveKeyID(canceled, signing.Write, "write-key"))
	require.NoError(t, first.SaveKeyID(ctx, signing.Read, "read-key"))
	require.NoError(t, first.SaveKeyID(ctx, signing.Copy, "read-key"))
	_, err = first.KeyID(ctx, "invalid")
	require.Error(t, err)
	require.Error(t, first.SaveKeyID(ctx, signing.Read, ""))
	ids, err := first.KeyIDs(ctx)
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"write-key", "read-key"}, ids)
	require.NoError(t, first.Close())
	require.NoError(t, first.Close())
	// Canceling the request must not roll back an identifier for a key created in Garage.
	store.Close()
	reopened, err := Open(ctx, url, slog.Default())
	require.NoError(t, err)
	defer reopened.Close()
	second, err := reopened.LockBucket(ctx, "bucket")
	require.NoError(t, err)
	id, err := second.KeyID(ctx, signing.Write)
	require.NoError(t, err)
	assert.Equal(t, "write-key", id)
	require.NoError(t, second.DeleteKeyReferences(ctx))
	ids, err = second.KeyIDs(ctx)
	require.NoError(t, err)
	assert.Empty(t, ids)
	require.NoError(t, second.Close())
	assert.Empty(t, store.buckets)
}

func TestBucketProvisioningDoesNotBlockSessionWrites(t *testing.T) {
	t.Parallel()

	store, _ := openTestStore(t)
	ctx := context.Background()
	lock, err := store.LockBucket(ctx, "bucket")
	require.NoError(t, err)
	defer lock.Close() //nolint:errcheck
	deadline, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	require.NoError(t, store.SaveSession(deadline, []byte("hash"), []byte("encrypted"), time.Now().Add(time.Hour)))
}

func TestConcurrentBucketLocks(t *testing.T) {
	t.Parallel()

	store, _ := openTestStore(t)
	var holders atomic.Int32
	var wg sync.WaitGroup
	failures := make(chan error, 20)
	for range 20 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			lock, err := store.LockBucket(context.Background(), "bucket")
			if err != nil {
				failures <- err
				return
			}
			if holders.Add(1) != 1 {
				failures <- errors.New("multiple bucket lock holders")
			}
			holders.Add(-1)
			if err := lock.Close(); err != nil {
				failures <- err
			}
		}()
	}
	wg.Wait()
	close(failures)
	for err := range failures {
		require.NoError(t, err)
	}
	assert.Empty(t, store.buckets)
}

func TestConcurrentLoginBudget(t *testing.T) {
	t.Parallel()

	now := time.Now()
	store, url := openTestStore(t)
	ctx := context.Background()
	var allowed atomic.Int32
	var wg sync.WaitGroup
	failures := make(chan error, 60)
	for range 60 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ok, err := store.allowLoginAt(ctx, "127.0.0.1", now)
			if err != nil {
				failures <- err
			}
			if ok {
				allowed.Add(1)
			}
		}()
	}
	wg.Wait()
	close(failures)
	for err := range failures {
		require.NoError(t, err)
	}
	assert.Equal(t, int32(loginAttemptsPerMinute), allowed.Load())
	store.Close()
	reopened, err := Open(ctx, url, slog.Default())
	require.NoError(t, err)
	defer reopened.Close()
	ok, err := reopened.allowLoginAt(ctx, "127.0.0.1", now)
	require.NoError(t, err)
	assert.False(t, ok)
	ok, err = reopened.AllowLogin(ctx, "127.0.0.2")
	require.NoError(t, err)
	assert.True(t, ok)
	_, err = reopened.AllowLogin(ctx, "")
	require.Error(t, err)
	_, err = reopened.db.ExecContext(ctx, `UPDATE webui_login_attempts SET window_start=?`, time.Now().Add(-10*time.Minute).Unix())
	require.NoError(t, err)
	require.NoError(t, reopened.CleanupExpiredLoginAttempts(ctx))
	var count int
	require.NoError(t, reopened.db.QueryRowContext(ctx, `SELECT count(*) FROM webui_login_attempts`).Scan(&count))
	assert.Zero(t, count)
}

func TestSQLiteURLValidation(t *testing.T) {
	t.Parallel()

	t.Run("empty URL", func(t *testing.T) {
		t.Parallel()

		url := ""
		_, err := databasePath(url)
		require.Error(t, err)
	})

	t.Run("postgres://localhost/db", func(t *testing.T) {
		t.Parallel()

		url := "postgres://localhost/db"
		_, err := databasePath(url)
		require.Error(t, err)
	})

	t.Run("sqlite://remote/db", func(t *testing.T) {
		t.Parallel()

		url := "sqlite://remote/db"
		_, err := databasePath(url)
		require.Error(t, err)
	})

	t.Run("sqlite:", func(t *testing.T) {
		t.Parallel()

		url := "sqlite:"
		_, err := databasePath(url)
		require.Error(t, err)
	})

	t.Run("sqlite::memory:", func(t *testing.T) {
		t.Parallel()

		url := "sqlite::memory:"
		_, err := databasePath(url)
		require.Error(t, err)
	})

	t.Run("sqlite:///tmp/db?mode=ro", func(t *testing.T) {
		t.Parallel()

		url := "sqlite:///tmp/db?mode=ro"
		_, err := databasePath(url)
		require.Error(t, err)
	})

	t.Run("sqlite:///tmp/db#fragment", func(t *testing.T) {
		t.Parallel()

		url := "sqlite:///tmp/db#fragment"
		_, err := databasePath(url)
		require.Error(t, err)
	})
	t.Run("sqlite:console.db", func(t *testing.T) {
		t.Parallel()

		url := "sqlite:console.db"
		path, err := databasePath(url)
		require.NoError(t, err)
		assert.True(t, filepath.IsAbs(path))
	})

	t.Run("sqlite:///tmp/console.db", func(t *testing.T) {
		t.Parallel()

		url := "sqlite:///tmp/console.db"
		path, err := databasePath(url)
		require.NoError(t, err)
		assert.True(t, filepath.IsAbs(path))
	})
}
