// Package credentials provisions bucket-scoped Garage signing credentials.
package credentials

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/containeroo/garage-webui/internal/garage/admin"
	"github.com/containeroo/garage-webui/internal/signing"
)

// Provider manages Garage keys, permissions and browser CORS.
type Provider struct {
	// admin issues Garage requests on behalf of the signed-in administrator.
	admin *admin.Client

	// store coordinates managed key IDs across application instances.
	store signing.BucketLocker

	// origin is the browser origin allowed by managed bucket CORS rules.
	origin string

	// cache holds recently provisioned signing keys shared by the factory.
	cache *Cache

	// token isolates cached credentials by the administrator's Garage token.
	token string
}

// New constructs a provider with a required cache shared across requests.
// The cache isolates signing credentials by admin token, bucket and purpose.
// A nil cache is a programming error because provisioning must never silently skip caching.
func New(
	client *admin.Client,
	store signing.BucketLocker,
	origin string,
	cache *Cache,
	token string,
) *Provider {
	if cache == nil {
		panic("credentials: signing cache is required")
	}

	return &Provider{
		admin:  client,
		store:  store,
		origin: origin,
		cache:  cache,
		token:  token,
	}
}

// cached returns a verified signing key from the current token's bucket scope.
func (m *Provider) cached(bucketID, purpose string) (signing.Credential, bool) {
	return m.cache.Get(m.token, bucketID, purpose)
}

// remember caches a prepared key only after its Garage permissions and CORS succeed.
func (m *Provider) remember(bucketID, purpose string, key signing.Credential) {
	m.cache.Put(m.token, bucketID, purpose, key)
}

// Copy obtains the bucket-scoped read/write key used for server-side copying.
func (m *Provider) Copy(ctx context.Context, bucketID string) (signing.Credential, error) {
	return m.bucketCredential(ctx, bucketID, signing.Copy)
}

// Write obtains the bucket-scoped write-only key used for uploads and deletions.
func (m *Provider) Write(ctx context.Context, bucketID, _ string) (signing.Credential, error) {
	return m.bucketCredential(ctx, bucketID, signing.Write)
}

// Read obtains the bucket-scoped read-only key used for listing and downloads.
func (m *Provider) Read(ctx context.Context, bucketID string) (signing.Credential, error) {
	return m.bucketCredential(ctx, bucketID, signing.Read)
}

// bucketCredential coordinates Garage provisioning while holding the bucket lock.
// The generated key ID is saved immediately, so failed permission/CORS updates
// can be retried without losing track of a newly created Garage key.
func (m *Provider) bucketCredential(ctx context.Context, bucketID string, purpose signing.Purpose) (key signing.Credential, err error) {
	if cached, ok := m.cached(bucketID, string(purpose)); ok {
		return cached, nil
	}

	lock, err := m.store.LockBucket(ctx, bucketID)
	if err != nil {
		return signing.Credential{}, err
	}

	defer func() {
		err = errors.Join(err, lock.Close())
		if err == nil {
			m.remember(bucketID, string(purpose), key)
		} else {
			key = signing.Credential{}
		}
	}()

	previousID, err := lock.KeyID(ctx, purpose)
	if err != nil {
		return signing.Credential{}, err
	}

	provisionCtx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	key, err = m.managedKeyForPurpose(provisionCtx, previousID, bucketID, string(purpose))
	if key.AccessKeyID != "" && key.AccessKeyID != previousID {
		if saveErr := lock.SaveKeyID(ctx, purpose, key.AccessKeyID); saveErr != nil {
			return signing.Credential{}, errors.Join(err, saveErr)
		}
	}

	if err != nil {
		return signing.Credential{}, err
	}

	if err := m.enforcePurpose(provisionCtx, bucketID, key.AccessKeyID, purpose); err != nil {
		return signing.Credential{}, err
	}

	if m.origin != "" {
		if err := m.ensureCORS(provisionCtx, bucketID); err != nil {
			return signing.Credential{}, err
		}
	}

	return key, nil
}

// enforcePurpose grants the selected key only its intended bucket privileges.
func (m *Provider) enforcePurpose(ctx context.Context, bucketID, keyID string, purpose signing.Purpose) error {
	switch purpose {
	case signing.Write:
		return m.enforceWriteOnly(ctx, bucketID, keyID)
	case signing.Read:
		return m.enforceReadOnly(ctx, bucketID, keyID)
	case signing.Copy:
		if err := m.admin.AllowBucketKey(ctx, bucketID, keyID, admin.BucketPermissions{Read: true, Write: true}); err != nil {
			return err
		}
		return m.admin.DenyBucketKey(ctx, bucketID, keyID, admin.BucketPermissions{Owner: true})
	default:
		return fmt.Errorf("unknown bucket signing key purpose %q", purpose)
	}
}

// CredentialForPurpose creates or loads a purpose-limited S3 credential.
func (m *Provider) managedKeyForPurpose(ctx context.Context, keyID, bucketID, purpose string) (signing.Credential, error) {
	if keyID != "" {
		key, err := m.admin.GetKey(ctx, keyID, true)
		if err != nil && !errors.Is(err, admin.ErrNotFound) {
			return signing.Credential{}, err
		}

		if err == nil && usableKey(key) {
			return key, nil
		}
	}

	key, err := m.admin.CreateKey(ctx, "garage-webui-"+purpose+"-"+bucketID)
	if err != nil {
		return signing.Credential{}, fmt.Errorf("create Garage S3 key: %w", err)
	}

	if key.SecretAccessKey != "" {
		return key, nil
	}
	// Keep the generated ID even when retrieving the secret fails; it must be
	// persisted before further provisioning attempts to avoid orphaning keys.
	fresh, err := m.admin.GetKey(ctx, key.AccessKeyID, true)
	if err != nil || fresh.SecretAccessKey == "" {
		return signing.Credential{AccessKeyID: key.AccessKeyID}, errors.New("garage did not return a usable S3 secret")
	}

	return fresh, nil
}

// usableKey checks that Garage returned a non-expired signing credential.
func usableKey(key signing.Credential) bool {
	return key.AccessKeyID != "" && key.SecretAccessKey != "" && !key.Expired
}

// enforceWriteOnly grants write permission and revokes read/owner permissions.
func (m *Provider) enforceWriteOnly(ctx context.Context, bucketID, keyID string) error {
	if err := m.admin.AllowBucketKey(ctx, bucketID, keyID, admin.BucketPermissions{Write: true}); err != nil {
		return fmt.Errorf("grant object write permission: %w", err)
	}

	if err := m.admin.DenyBucketKey(ctx, bucketID, keyID, admin.BucketPermissions{Read: true, Owner: true}); err != nil {
		return fmt.Errorf("remove non-write permissions: %w", err)
	}

	return nil
}

// enforceReadOnly grants read permission and revokes write/owner permissions.
func (m *Provider) enforceReadOnly(ctx context.Context, bucketID, keyID string) error {
	if err := m.admin.AllowBucketKey(ctx, bucketID, keyID, admin.BucketPermissions{Read: true}); err != nil {
		return fmt.Errorf("grant object read permission: %w", err)
	}

	if err := m.admin.DenyBucketKey(ctx, bucketID, keyID, admin.BucketPermissions{Write: true, Owner: true}); err != nil {
		return fmt.Errorf("remove non-read permissions: %w", err)
	}

	return nil
}
