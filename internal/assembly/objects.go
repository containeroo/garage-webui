// Package assembly wires Garage identities to object services and storage.
package assembly

import (
	"context"
	"errors"
	"net/http"
	"slices"

	"github.com/containeroo/garage-webui/internal/application/buckets"
	"github.com/containeroo/garage-webui/internal/application/objects"
	"github.com/containeroo/garage-webui/internal/garage/credentials"
	"github.com/containeroo/garage-webui/internal/http/auth"
	"github.com/containeroo/garage-webui/internal/s3"
	"github.com/containeroo/garage-webui/internal/signing"
)

// Factory assembles credential-scoped services without relying on HTTP handlers.
type Factory struct {
	auth      *auth.Service
	store     signing.BucketLocker
	deletion  *buckets.Service
	cache     *credentials.Cache
	publicURL string
}

// New creates an object-service factory shared by all application requests.
func New(authentication *auth.Service, store signing.BucketLocker, publicURL string) *Factory {
	return &Factory{auth: authentication, store: store, deletion: buckets.New(store), publicURL: publicURL, cache: credentials.NewCache()}
}

// ForIdentity produces a service restricted to the current Garage credentials.
func (f *Factory) ForIdentity(id auth.Identity) (*objects.Service, error) {
	var provider s3.CredentialProvider
	var buckets objects.BucketResolver

	switch id.Credential.Kind {
	case "admin":
		if !canProvisionObjects(id) {
			return nil, objects.ErrForbidden
		}
		client, err := f.auth.AdminClient(id)
		if err != nil {
			return nil, err
		}
		provider = credentials.NewWithCache(client, f.store, f.publicURL, f.cache, id.Credential.Token)
		buckets = client
	case "s3":
		provider = sessionKeys{credential: signing.Credential{
			AccessKeyID: id.Credential.AccessKeyID, SecretAccessKey: id.Credential.SecretAccessKey,
		}}
		buckets = sessionBuckets{auth: f.auth, credential: id.Credential}
	default:
		return nil, objects.ErrForbidden
	}

	manager, err := s3.NewManager(provider, f.auth.S3URL, f.auth.Region, f.publicURL)
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
func canProvisionObjects(id auth.Identity) bool {
	for _, operation := range []string{
		"GetBucketInfo", "GetKeyInfo", "CreateKey", "AllowBucketKey", "DenyBucketKey", "UpdateBucket",
	} {
		if !id.Allows(operation) {
			return false
		}
	}

	return true
}

type sessionBuckets struct {
	auth       *auth.Service
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

type sessionKeys struct{ credential signing.Credential }

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
	return id.Allows("DeleteBucket") && id.Allows("DeleteKey") && id.Allows("GetBucketInfo")
}
