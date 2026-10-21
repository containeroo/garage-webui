package storage

import (
	"context"
	"time"
)

// DeletionJob records a persistent, credential-scoped recursive deletion.
type DeletionJob struct {
	// ID uniquely identifies the job.
	ID string `json:"id"`
	// Owner identifies the encrypted credential without exposing the secret.
	Owner []byte `json:"-"`
	// EncryptedCredential contains an authenticated, encrypted Garage credential.
	EncryptedCredential []byte `json:"-"`
	// Bucket identifies the Garage bucket.
	Bucket string `json:"bucket"`
	// Target is the first selected object or folder shown in progress views.
	Target string `json:"target,omitempty"`
	// Targets holds JSON-encoded file and folder selections.
	Targets string `json:"-"`
	// Status is pending, running, completed, failed or cancelled.
	Status string `json:"status"`
	// Deleted counts successful S3 deletions.
	Deleted int64 `json:"deleted"`
	// Error is a safe summary of the latest failure.
	Error string `json:"error,omitempty"`
	// CancelRequested asks the worker to stop at the next checkpoint.
	CancelRequested bool `json:"cancelRequested"`
	// Worker identifies the current claim owner.
	Worker string `json:"-"`
	// CreatedAt records when the job was accepted.
	CreatedAt time.Time `json:"createdAt"`
	// UpdatedAt records the latest persistent progress update.
	UpdatedAt time.Time `json:"updatedAt"`
}

// DeletionJobs coordinates jobs through the shared database, including concurrent replicas.
type DeletionJobs interface {
	// CreateDeletionJob persists a job before any S3 operation begins.
	CreateDeletionJob(context.Context, DeletionJob) error
	// ListDeletionJobs returns jobs visible to the supplied Garage credential.
	ListDeletionJobs(context.Context, []byte) ([]DeletionJob, error)
	// ClaimDeletionJob atomically assigns an unclaimed or abandoned job to a worker.
	ClaimDeletionJob(context.Context, string, time.Duration) (DeletionJob, bool, error)
	// RenewDeletionJob extends the lease and reports whether cancellation was requested.
	RenewDeletionJob(context.Context, string, string, time.Duration) (cancelled bool, owned bool, err error)
	// RecordDeletionProgress records a batch of successful object deletions while a worker owns the lease.
	RecordDeletionProgress(context.Context, string, string, int64) error
	// FinishDeletionJob records a terminal status and erases its stored credential.
	FinishDeletionJob(context.Context, string, string, string, string) error
	// CancelDeletionJob stops an unstarted job or requests cancellation of a running job.
	CancelDeletionJob(context.Context, string, []byte) (bool, error)
	// CleanupDeletionJobs removes old terminal job records.
	CleanupDeletionJobs(context.Context, time.Time) error
}
