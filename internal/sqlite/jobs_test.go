package sqlite

import (
	"context"
	"testing"
	"time"

	"github.com/containeroo/garage-webui/internal/storage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestPersistentDeletionJobClaims exercises retry, isolation, cancellation and credential cleanup.
func TestPersistentDeletionJobClaims(t *testing.T) {
	t.Parallel()
	store, url := openTestStore(t)
	ctx := context.Background()
	owner := []byte("owner-one")
	job := storage.DeletionJob{ID: "first", Owner: owner, EncryptedCredential: []byte("encrypted"), Bucket: "bucket", Targets: `{"folders":["folder/"]}`}
	require.NoError(t, store.CreateDeletionJob(ctx, job))
	claimed, ok, err := store.ClaimDeletionJob(ctx, "worker-1", time.Minute)
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, job.ID, claimed.ID)
	assert.Equal(t, job.EncryptedCredential, claimed.EncryptedCredential)
	_, ok, err = store.ClaimDeletionJob(ctx, "worker-2", time.Minute)
	require.NoError(t, err)
	assert.False(t, ok)
	require.NoError(t, store.RecordDeletionProgress(ctx, job.ID, "worker-1", 3))
	_, err = store.db.ExecContext(ctx, `
		UPDATE webui_deletion_jobs
		SET lease_until = ?
		WHERE id = ?
	`, time.Now().Add(-time.Second).UnixMilli(), job.ID)
	require.NoError(t, err)
	claimed, ok, err = store.ClaimDeletionJob(ctx, "worker-2", time.Minute)
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, int64(3), claimed.Deleted)
	_, owned, err := store.RenewDeletionJob(ctx, job.ID, "worker-1", time.Minute)
	require.NoError(t, err)
	assert.False(t, owned)
	found, err := store.CancelDeletionJob(ctx, job.ID, []byte("someone-else"))
	require.NoError(t, err)
	assert.False(t, found)
	found, err = store.CancelDeletionJob(ctx, job.ID, owner)
	require.NoError(t, err)
	require.True(t, found)
	cancelled, owned, err := store.RenewDeletionJob(ctx, job.ID, "worker-2", time.Minute)
	require.NoError(t, err)
	require.True(t, owned)
	assert.True(t, cancelled)
	require.NoError(t, store.FinishDeletionJob(ctx, job.ID, "worker-2", "cancelled", ""))
	visible, err := store.ListDeletionJobs(ctx, owner)
	require.NoError(t, err)
	require.Len(t, visible, 1)
	assert.Equal(t, "cancelled", visible[0].Status)
	assert.Empty(t, visible[0].EncryptedCredential)
	store.Close()
	reopened, err := Open(ctx, url, store.logger)
	require.NoError(t, err)
	defer reopened.Close()
	visible, err = reopened.ListDeletionJobs(ctx, owner)
	require.NoError(t, err)
	assert.Len(t, visible, 1)
}
