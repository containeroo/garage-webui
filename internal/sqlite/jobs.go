package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/containeroo/garage-webui/internal/storage"
)

const jobColumns = `
	id,
	owner_hash,
	credential,
	bucket,
	targets,
	status,
	deleted,
	error,
	cancel_requested,
	worker,
	created_at,
	updated_at
`

// scanJob converts SQLite's millisecond timestamps into a persistent job.
func scanJob(row interface{ Scan(...any) error }) (storage.DeletionJob, error) {
	var job storage.DeletionJob
	var created, updated int64
	err := row.Scan(
		&job.ID,
		&job.Owner,
		&job.EncryptedCredential,
		&job.Bucket,
		&job.Targets,
		&job.Status,
		&job.Deleted,
		&job.Error,
		&job.CancelRequested,
		&job.Worker,
		&created,
		&updated,
	)
	job.CreatedAt = time.UnixMilli(created).UTC()
	job.UpdatedAt = time.UnixMilli(updated).UTC()
	return job, err
}

// CreateDeletionJob persists the job before scheduling its execution.
func (s *Store) CreateDeletionJob(ctx context.Context, job storage.DeletionJob) error {
	now := time.Now().UnixMilli()
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO webui_deletion_jobs (
			id,
			owner_hash,
			credential,
			bucket,
			targets,
			created_at,
			updated_at
		)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, job.ID, job.Owner, job.EncryptedCredential, job.Bucket, job.Targets, now, now)
	return err
}

// ListDeletionJobs lists the 100 newest jobs belonging to one credential.
func (s *Store) ListDeletionJobs(ctx context.Context, owner []byte) ([]storage.DeletionJob, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT `+jobColumns+`
		FROM webui_deletion_jobs
		WHERE owner_hash = ?
		ORDER BY created_at DESC
		LIMIT 100
	`, owner)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	jobs := make([]storage.DeletionJob, 0)
	for rows.Next() {
		job, err := scanJob(rows)
		if err != nil {
			return nil, err
		}
		jobs = append(jobs, job)
	}
	return jobs, rows.Err()
}

// ClaimDeletionJob atomically assigns one available job to this worker.
func (s *Store) ClaimDeletionJob(ctx context.Context, worker string, lease time.Duration) (storage.DeletionJob, bool, error) {
	now := time.Now().UnixMilli()
	row := s.db.QueryRowContext(ctx, `
		UPDATE webui_deletion_jobs
		SET
			status = 'running',
			worker = ?,
			lease_until = ?,
			updated_at = ?
		WHERE id = (
			SELECT id
			FROM webui_deletion_jobs
			WHERE status = 'pending'
				OR (status = 'running' AND lease_until < ?)
			ORDER BY created_at
			LIMIT 1
		)
		RETURNING `+jobColumns, worker, now+lease.Milliseconds(), now, now)
	job, err := scanJob(row)
	if errors.Is(err, sql.ErrNoRows) {
		return storage.DeletionJob{}, false, nil
	}
	return job, err == nil, err
}

// RenewDeletionJob updates only the claiming worker's live lease.
func (s *Store) RenewDeletionJob(ctx context.Context, id, worker string, lease time.Duration) (bool, bool, error) {
	now := time.Now().UnixMilli()
	var cancelled bool
	err := s.db.QueryRowContext(ctx, `
		UPDATE webui_deletion_jobs
		SET lease_until = ?
		WHERE id = ?
			AND worker = ?
			AND status = 'running'
			AND lease_until > ?
		RETURNING cancel_requested
	`, now+lease.Milliseconds(), id, worker, now).Scan(&cancelled)
	if errors.Is(err, sql.ErrNoRows) {
		return false, false, nil
	}
	return cancelled, err == nil, err
}

// RecordDeletionProgress increments batched progress only for the lease owner.
func (s *Store) RecordDeletionProgress(ctx context.Context, id, worker string, deleted int64) error {
	if deleted <= 0 {
		return nil
	}

	now := time.Now().UnixMilli()
	tag, err := s.db.ExecContext(ctx, `
		UPDATE webui_deletion_jobs
		SET
			deleted = deleted + ?,
			updated_at = ?
		WHERE id = ?
			AND worker = ?
			AND status = 'running'
			AND lease_until > ?
	`, deleted, now, id, worker, now)
	if err != nil {
		return err
	}
	n, err := tag.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return errors.New("deletion job lease lost")
	}
	return nil
}

// FinishDeletionJob records a terminal result and deletes the stored credential.
func (s *Store) FinishDeletionJob(ctx context.Context, id, worker, status, message string) error {
	now := time.Now().UnixMilli()
	tag, err := s.db.ExecContext(ctx, `
		UPDATE webui_deletion_jobs
		SET
			status = ?,
			error = ?,
			credential = NULL,
			worker = '',
			lease_until = NULL,
			updated_at = ?
		WHERE id = ?
			AND worker = ?
			AND status = 'running'
			AND lease_until > ?
	`, status, message, now, id, worker, now)
	if err != nil {
		return err
	}
	n, err := tag.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return errors.New("deletion job lease lost")
	}
	return nil
}

// CancelDeletionJob restricts cancellation to the credential that created the job.
func (s *Store) CancelDeletionJob(ctx context.Context, id string, owner []byte) (bool, error) {
	tag, err := s.db.ExecContext(ctx, `
		UPDATE webui_deletion_jobs
		SET
			cancel_requested = 1,
			status = CASE WHEN status = 'pending' THEN 'cancelled' ELSE status END,
			credential = CASE WHEN status = 'pending' THEN NULL ELSE credential END,
			updated_at = ?
		WHERE id = ?
			AND owner_hash = ?
			AND status IN ('pending', 'running')
	`, time.Now().UnixMilli(), id, owner)
	if err != nil {
		return false, err
	}
	n, err := tag.RowsAffected()
	return n > 0, err
}

// CleanupDeletionJobs bounds retained terminal jobs.
func (s *Store) CleanupDeletionJobs(ctx context.Context, cutoff time.Time) error {
	_, err := s.db.ExecContext(ctx, `
		DELETE FROM webui_deletion_jobs
		WHERE status IN ('completed', 'failed', 'cancelled')
			AND updated_at < ?
	`, cutoff.UnixMilli())
	return err
}
