package admin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
)

// Bucket identifies a Garage bucket and its global aliases.
type Bucket struct {
	// ID is the immutable Garage bucket ID.
	ID string `json:"id"`
	// GlobalAliases identifies S3-visible bucket names.
	GlobalAliases []string `json:"globalAliases"`
}

// BucketInfo includes both object usage and browser CORS settings.
type BucketInfo struct {
	// ID identifies the Garage bucket.
	ID string `json:"id"`
	// GlobalAliases holds the public S3 bucket names.
	GlobalAliases []string `json:"globalAliases"`
	// Objects counts the finished objects stored in the bucket.
	Objects int64 `json:"objects"`
	// UnfinishedUploads counts incomplete uploads.
	UnfinishedUploads int64 `json:"unfinishedUploads"`
	// UnfinishedMultipartUploads counts incomplete multipart uploads.
	UnfinishedMultipartUploads int64 `json:"unfinishedMultipartUploads"`
	// CORSRules preserves Garage's existing CORS configuration.
	CORSRules []json.RawMessage `json:"corsRules"`
}

// HasObjects reports whether Garage still tracks objects or unfinished uploads.
func (b BucketInfo) HasObjects() bool {
	return b.Objects > 0 || b.UnfinishedUploads > 0 || b.UnfinishedMultipartUploads > 0
}

// GetBucket loads the trusted Garage bucket metadata.
func (c *Client) GetBucket(ctx context.Context, id string) (BucketInfo, error) {
	var bucket BucketInfo
	if err := c.request(ctx, OpGetBucketInfo, url.Values{"id": {id}}, nil, &bucket); err != nil {
		return BucketInfo{}, err
	}

	if bucket.ID != id {
		return BucketInfo{}, fmt.Errorf("garage returned an unexpected bucket identifier")
	}

	return bucket, nil
}

// ResolveBucket returns the trusted ID and first global S3 alias for object operations.
func (c *Client) ResolveBucket(ctx context.Context, id string) (string, string, error) {
	bucket, err := c.BucketByID(ctx, id)
	if err != nil {
		return "", "", err
	}

	return bucket.ID, bucket.GlobalAliases[0], nil
}

// BucketByID retrieves a bucket with a usable global S3 alias.
func (c *Client) BucketByID(ctx context.Context, id string) (Bucket, error) {
	info, err := c.GetBucket(ctx, id)
	if err != nil {
		return Bucket{}, err
	}

	if len(info.GlobalAliases) == 0 {
		return Bucket{}, errors.New("garage bucket has no global alias")
	}

	return Bucket{ID: info.ID, GlobalAliases: info.GlobalAliases}, nil
}

// UpdateBucketCORS updates only CORS rules, retaining unrelated bucket settings.
func (c *Client) UpdateBucketCORS(ctx context.Context, id string, rules []json.RawMessage) error {
	return c.request(ctx, OpUpdateBucket, url.Values{"id": {id}}, struct {
		CORSRules []json.RawMessage `json:"corsRules"`
	}{CORSRules: rules}, nil)
}

// DeleteBucket deletes a Garage bucket after it has been verified empty.
func (c *Client) DeleteBucket(ctx context.Context, id string) error {
	return c.request(ctx, OpDeleteBucket, url.Values{"id": {id}}, nil, nil)
}
