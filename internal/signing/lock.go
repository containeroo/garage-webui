package signing

import "context"

// Purpose identifies one independently permissioned signing key for a bucket.
type Purpose string

const (
	// Write grants object uploads and deletions, without read or owner access.
	Write Purpose = "upload"
	// Read grants object listings and downloads, without write or owner access.
	Read Purpose = "read"
	// Copy grants read and write access for server-side copies, without owner access.
	Copy Purpose = "copy"
)

// BucketLock holds the exclusive provisioning lock for one bucket until Close.
// Its caller owns all Garage API operations performed while holding the lock.
type BucketLock interface {
	// KeyID reads the saved key ID for an individual purpose.
	KeyID(context.Context, Purpose) (string, error)
	// SaveKeyID persists a newly provisioned key even after request cancellation.
	SaveKeyID(context.Context, Purpose, string) error
	// KeyIDs lists the bucket's distinct managed Garage key IDs.
	KeyIDs(context.Context) ([]string, error)
	// DeleteKeyReferences clears IDs only after Garage cleanup succeeds.
	DeleteKeyReferences(context.Context) error
	// Close releases the bucket lock, including when the caller was canceled.
	Close() error
}

// BucketLocker coordinates signing-key operations across application replicas.
type BucketLocker interface {
	// LockBucket acquires an exclusive provisioning lock for the bucket.
	LockBucket(context.Context, string) (BucketLock, error)
}
