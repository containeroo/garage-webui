// Package assembly wires Garage identities to object services and storage.
package assembly

import (
	"context"
	"errors"
	"net/http"
	"slices"

	"github.com/containeroo/garage-webui/internal/application/buckets"
	"github.com/containeroo/garage-webui/internal/application/objects"
	auth "github.com/containeroo/garage-webui/internal/auth"
	"github.com/containeroo/garage-webui/internal/credential"
	"github.com/containeroo/garage-webui/internal/garage/admin"
	"github.com/containeroo/garage-webui/internal/garage/credentials"
	"github.com/containeroo/garage-webui/internal/s3"
	"github.com/containeroo/garage-webui/internal/signing"
)

// Factory assembles credential-scoped services without relying on HTTP handlers.
type Factory struct {
	// auth is used to create admin clients.
	auth *auth.Service
	// store is used to create signing keys.
	store signing.BucketLocker
	// deletion is used to delete buckets.
	deletion *buckets.Service
	// cache isolates credentials whose browser CORS provisioning has succeeded.
	cache *credentials.Cache
	// transferCache holds credentials for backend operations without CORS changes.
	transferCache *credentials.Cache
	// publicURL is used to create signed URLs.
	publicURL string
}

// New creates an object-service factory shared by all application requests.
func New(
	authentication *auth.Service,
	store signing.BucketLocker,
	publicURL string,
) *Factory {
	return &Factory{
		auth:          authentication,
		store:         store,
		deletion:      buckets.New(store),
		publicURL:     publicURL,
		cache:         credentials.NewCache(),
		transferCache: credentials.NewCache(),
	}
}

// ForIdentity produces a service restricted to the current Garage credentials.
func (f *Factory) ForIdentity(id auth.Identity) (*objects.Service, error) {
	return f.forEndpoint(id, false)
}

// ForDirectIdentity prepares browser-direct transfers against the public endpoint.
func (f *Factory) ForDirectIdentity(id auth.Identity) (*objects.Service, error) {
	return f.forEndpoint(id, true)
}

func (f *Factory) forEndpoint(id auth.Identity, direct bool) (*objects.Service, error) {
	endpoint, origin, cache := f.auth.S3URL, "", f.transferCache
	if direct {
		endpoint, origin, cache = f.auth.S3PublicURL, f.publicURL, f.cache
	}

	var provider s3.CredentialProvider
	var buckets objects.BucketResolver

	switch id.Credential.Kind {
	case credential.Admin:
		if !canProvisionObjects(id, direct) {
			return nil, objects.ErrForbidden
		}
		client, err := f.auth.AdminClient(id)
		if err != nil {
			return nil, err
		}
		provider = credentials.New(client, f.store, origin, cache, id.Credential.Token)
		buckets = client
	case credential.S3:
		provider = sessionKeys{
			credential: signing.Credential{
				AccessKeyID:     id.Credential.AccessKeyID,
				SecretAccessKey: id.Credential.SecretAccessKey,
			},
		}
		buckets = sessionBuckets{auth: f.auth, credential: id.Credential}
	default:
		return nil, objects.ErrForbidden
	}

	manager, err := s3.NewManager(provider, endpoint, f.auth.Region, origin)
	if err != nil {
		return nil, err
	}

	if manager == nil {
		return nil, errors.New("s3 endpoint not configured")
	}

	return objects.New(buckets, manager), nil
}

// canProvisionObjects requires every privilege used by managed-key provisioning.
// In particular, GetKeyInfo alone must never grant access to an existing signing key.
func canProvisionObjects(id auth.Identity, direct bool) bool {
	if direct && !id.Allows(admin.OpUpdateBucket) {
		return false
	}
	for _, operation := range []admin.Operation{
		admin.OpGetBucketInfo,
		admin.OpGetKeyInfo,
		admin.OpCreateKey,
		admin.OpAllowBucketKey,
		admin.OpDenyBucketKey,
	} {
		if !id.Allows(operation) {
			return false
		}
	}

	return true
}

// sessionBuckets restricts bucket names to those visible to the current S3 credential.
type sessionBuckets struct {
	// auth is used to resolve bucket names.
	auth *auth.Service
	// credential is used to resolve bucket names.
	credential auth.Credential
}

// ResolveBucket resolves only names visible to the current S3 credential.
func (b sessionBuckets) ResolveBucket(ctx context.Context, name string) (string, string, error) {
	names, err := b.auth.Buckets(ctx, b.credential)
	if err != nil {
		return "", "", err
	}

	if !slices.Contains(names, name) {
		return "", "", errors.New("bucket is not accessible")
	}

	return name, name, nil
}

// sessionKeys signs with the S3 credential provided by the current session.
type sessionKeys struct {
	// credential holds the signing credentials for the current session.
	credential signing.Credential
}

// Read signs with the S3 credential provided by the current session.
func (k sessionKeys) Read(context.Context, string) (signing.Credential, error) {
	return k.credential, nil
}

// Write signs with the S3 credential provided by the current session.
func (k sessionKeys) Write(context.Context, string, string) (signing.Credential, error) {
	return k.credential, nil
}

// Copy signs with the S3 credential provided by the current session.
func (k sessionKeys) Copy(context.Context, string) (signing.Credential, error) {
	return k.credential, nil
}

// DeleteBucket enforces token scopes before delegating to the application service.
func (f *Factory) DeleteBucket(ctx context.Context, id auth.Identity, bucketID string) (int, []byte, error) {
	if !canDeleteBucket(id) {
		return http.StatusForbidden, nil, objects.ErrForbidden
	}

	if bucketID == "" || len(bucketID) > 256 {
		return http.StatusBadRequest, nil, errors.New("invalid bucket ID")
	}

	client, err := f.auth.AdminClient(id)
	if err != nil {
		return 0, nil, err
	}

	f.cache.InvalidateBucket(bucketID)
	defer f.cache.InvalidateBucket(bucketID)

	return f.deletion.Delete(ctx, client, bucketID)
}

// canDeleteBucket requires permission to revoke the Web UI's managed signing keys.
func canDeleteBucket(id auth.Identity) bool {
	return id.Allows(admin.OpDeleteBucket) &&
		id.Allows(admin.OpDeleteKey) &&
		id.Allows(admin.OpGetBucketInfo)
}
