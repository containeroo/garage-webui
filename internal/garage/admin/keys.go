package admin

import (
	"context"
	"errors"
	"fmt"
	"net/url"

	"github.com/containeroo/garage-webui/internal/signing"
)

// BucketPermissions describes only the privileges being granted or revoked.
type BucketPermissions struct {
	// Read controls object read access.
	Read bool `json:"read,omitempty"`
	// Write controls object write access.
	Write bool `json:"write,omitempty"`
	// Owner controls bucket ownership privileges.
	Owner bool `json:"owner,omitempty"`
}

// GetKey returns a Garage S3 key, optionally including its signing secret.
func (c *Client) GetKey(ctx context.Context, id string, withSecret bool) (signing.Credential, error) {
	query := url.Values{"id": {id}}
	if withSecret {
		query.Set("showSecretKey", "true")
	}

	var key signing.Credential
	if err := c.request(ctx, OpGetKeyInfo, query, nil, &key); err != nil {
		return signing.Credential{}, err
	}

	if key.AccessKeyID != id {
		return signing.Credential{}, fmt.Errorf("garage returned an unexpected key identifier")
	}

	return key, nil
}

// CreateKey creates a named Garage S3 key, initially without bucket access.
func (c *Client) CreateKey(ctx context.Context, name string) (signing.Credential, error) {
	var key signing.Credential
	if name == "" {
		return key, errors.New("s3 key name is required")
	}

	if err := c.request(ctx, OpCreateKey, nil, struct {
		Name string `json:"name"`
	}{Name: name}, &key); err != nil {
		return signing.Credential{}, err
	}

	if key.AccessKeyID == "" {
		return signing.Credential{}, errors.New("garage returned an empty S3 key identifier")
	}

	return key, nil
}

// AllowBucketKey grants the selected privileges on exactly one bucket.
func (c *Client) AllowBucketKey(ctx context.Context, bucketID, keyID string, permissions BucketPermissions) error {
	return c.bucketPermission(ctx, OpAllowBucketKey, bucketID, keyID, permissions)
}

// DenyBucketKey revokes the selected privileges on exactly one bucket.
func (c *Client) DenyBucketKey(ctx context.Context, bucketID, keyID string, permissions BucketPermissions) error {
	return c.bucketPermission(ctx, OpDenyBucketKey, bucketID, keyID, permissions)
}

// bucketPermission sends one typed Garage bucket permission change.
func (c *Client) bucketPermission(ctx context.Context, op Operation, bucketID, keyID string, permissions BucketPermissions) error {
	return c.request(ctx, op, nil, struct {
		BucketID    string            `json:"bucketId"`
		AccessKeyID string            `json:"accessKeyId"`
		Permissions BucketPermissions `json:"permissions"`
	}{bucketID, keyID, permissions}, nil)
}

// DeleteKey revokes an S3 key managed by Garage.
func (c *Client) DeleteKey(ctx context.Context, id string) error {
	return c.request(ctx, OpDeleteKey, url.Values{"id": {id}}, nil, nil)
}
