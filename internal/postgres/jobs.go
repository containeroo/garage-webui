package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/containeroo/garage-webui/internal/storage"
	"github.com/jackc/pgx/v5"
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

// scanJob loads a job from an ordered SQL row.
func scanJob(row pgx.Row) (storage.DeletionJob, error) {
	var job storage.DeletionJob
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
		&job.CreatedAt,
		&job.UpdatedAt,
	)
	return job, err
}

// CreateDeletionJob persists the authorized work and encrypted credential.
func (s *Store) CreateDeletionJob(ctx context.Context, job storage.DeletionJob) error {
	_, err := s.db.Exec(ctx, `
		INSERT INTO webui_deletion_jobs (
			id,
			owner_hash,
			credential,
			bucket,
			targets
		)
		VALUES ($1, $2, $3, $4, $5)
	`, job.ID, job.Owner, job.EncryptedCredential, job.Bucket, job.Targets)
	return err
}

// ListDeletionJobs returns at most 100 jobs for one credential, newest first.
func (s *Store) ListDeletionJobs(ctx context.Context, owner []byte) ([]storage.DeletionJob, error) {
	rows, err := s.db.Query(ctx, `
		SELECT `+jobColumns+`
		FROM webui_deletion_jobs
		WHERE owner_hash = $1
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

// ClaimDeletionJob atomically claims one pending or expired-lease job.
func (s *Store) ClaimDeletionJob(ctx context.Context, worker string, lease time.Duration) (storage.DeletionJob, bool, error) {
	row := s.db.QueryRow(ctx, `
		UPDATE webui_deletion_jobs
		SET
			status = 'running',
			worker = $1,
			lease_until = now() + ($2 * interval '1 second'),
			updated_at = now()
		WHERE id = (
			SELECT id
			FROM webui_deletion_jobs
			WHERE status = 'pending'
				OR (status = 'running' AND lease_until < now())
			ORDER BY created_at
			FOR UPDATE SKIP LOCKED
			LIMIT 1
		)
		RETURNING `+jobColumns, worker, lease.Seconds())
	job, err := scanJob(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return storage.DeletionJob{}, false, nil
	}
	return job, err == nil, err
}

// RenewDeletionJob extends only the current worker's lease and checks cancellation.
func (s *Store) RenewDeletionJob(ctx context.Context, id, worker string, lease time.Duration) (bool, bool, error) {
	var cancelled bool
	err := s.db.QueryRow(ctx, `
		UPDATE webui_deletion_jobs
		SET lease_until = now() + ($3 * interval '1 second')
		WHERE id = $1
			AND worker = $2
			AND status = 'running'
			AND lease_until > now()
		RETURNING cancel_requested
	`, id, worker, lease.Seconds()).Scan(&cancelled)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, false, nil
	}
	return cancelled, err == nil, err
}

// RecordDeletionProgress records a batch only while the worker holds its lease.
func (s *Store) RecordDeletionProgress(ctx context.Context, id, worker string, deleted int64) error {
	if deleted <= 0 {
		return nil
	}

	tag, err := s.db.Exec(ctx, `
		UPDATE webui_deletion_jobs
		SET
			deleted = deleted + $3,
			updated_at = now()
		WHERE id = $1
			AND worker = $2
			AND status = 'running'
			AND lease_until > now()
	`, id, worker, deleted)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return errors.New("deletion job lease lost")
	}
	return nil
}

// FinishDeletionJob records the terminal state and removes the encrypted credential.
func (s *Store) FinishDeletionJob(ctx context.Context, id, worker, status, message string) error {
	tag, err := s.db.Exec(ctx, `
		UPDATE webui_deletion_jobs
		SET
			status = $3,
			error = $4,
			credential = NULL,
			worker = '',
			lease_until = NULL,
			updated_at = now()
		WHERE id = $1
			AND worker = $2
			AND status = 'running'
			AND lease_until > now()
	`, id, worker, status, message)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return errors.New("deletion job lease lost")
	}
	return nil
}

// CancelDeletionJob restricts cancellation to jobs owned by the same credential.
func (s *Store) CancelDeletionJob(ctx context.Context, id string, owner []byte) (bool, error) {
	tag, err := s.db.Exec(ctx, `
		UPDATE webui_deletion_jobs
		SET
			cancel_requested = true,
			status = CASE WHEN status = 'pending' THEN 'cancelled' ELSE status END,
			credential = CASE WHEN status = 'pending' THEN NULL ELSE credential END,
			updated_at = now()
		WHERE id = $1
			AND owner_hash = $2
			AND status IN ('pending', 'running')
	`, id, owner)
	return tag.RowsAffected() > 0, err
}

// CleanupDeletionJobs bounds retention without removing running jobs.
func (s *Store) CleanupDeletionJobs(ctx context.Context, cutoff time.Time) error {
	_, err := s.db.Exec(ctx, `
		DELETE FROM webui_deletion_jobs
		WHERE status IN ('completed', 'failed', 'cancelled')
			AND updated_at < $1
	`, cutoff)
	return err
}
