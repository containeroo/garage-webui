// Package buckets owns the Garage bucket deletion workflow.
package buckets

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/containeroo/garage-webui/internal/garage/admin"
	"github.com/containeroo/garage-webui/internal/signing"
)

// AdminClient exposes only the typed Garage operations needed for bucket deletion.
type AdminClient interface {
	GetBucket(context.Context, string) (admin.BucketInfo, error)
	DeleteBucket(context.Context, string) error
	DeleteKey(context.Context, string) error
}

// Service coordinates bucket deletion with persistent managed signing-key references.
type Service struct {
	store signing.BucketLocker
}

// New constructs a bucket deletion service.
func New(store signing.BucketLocker) *Service { return &Service{store: store} }

// Delete removes an empty Garage bucket before revoking its managed signing keys.
// References survive interrupted cleanup so the next request can complete it.
func (s *Service) Delete(ctx context.Context, client AdminClient, bucketID string) (status int, response []byte, err error) {
	lock, err := s.store.LockBucket(ctx, bucketID)
	if err != nil {
		return 0, nil, err
	}

	defer func() {
		err = errors.Join(err, lock.Close())
		if err != nil {
			response = nil
		}
	}()

	keyIDs, err := lock.KeyIDs(ctx)
	if err != nil {
		return 0, nil, err
	}

	status, err = removeGarageBucket(ctx, client, bucketID, len(keyIDs) > 0)
	if err != nil {
		return status, nil, err
	}

	if err := revokeManagedKeys(ctx, client, keyIDs); err != nil {
		return status, nil, err
	}

	if err := lock.DeleteKeyReferences(ctx); err != nil {
		return status, nil, err
	}

	if status == http.StatusNoContent {
		return status, nil, nil
	}

	return status, []byte("{}"), nil
}

// removeGarageBucket checks for unfinished uploads before a destructive operation.
func removeGarageBucket(ctx context.Context, client AdminClient, bucketID string, hasManagedKeys bool) (int, error) {
	bucket, err := client.GetBucket(ctx, bucketID)
	if errors.Is(err, admin.ErrNotFound) && hasManagedKeys {
		return http.StatusNoContent, nil
	}

	if err != nil {
		return garageErrorStatus(err), err
	}

	if bucket.HasObjects() {
		return http.StatusConflict, errors.New("bucket contains objects or incomplete uploads")
	}

	if err := client.DeleteBucket(ctx, bucketID); err != nil {
		return garageErrorStatus(err), err
	}

	return http.StatusOK, nil
}

// revokeManagedKeys ignores already-missing keys when resuming a partial deletion.
func revokeManagedKeys(ctx context.Context, client AdminClient, keyIDs []string) error {
	for _, keyID := range keyIDs {
		err := client.DeleteKey(ctx, keyID)
		if errors.Is(err, admin.ErrNotFound) {
			continue
		}

		if err != nil {
			return fmt.Errorf("revoke managed signing key %s: %w", keyID, err)
		}
	}

	return nil
}

// garageErrorStatus preserves upstream client-error status codes for the web response.
func garageErrorStatus(err error) int {
	var status *admin.StatusError
	if errors.As(err, &status) {
		return status.Status
	}

	return 0
}
