package garage

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
)

// Bucket identifies a Garage bucket and its global aliases.
type Bucket struct {
	ID            string   `json:"id"`
	GlobalAliases []string `json:"globalAliases"`
}

// ResolveBucket returns the trusted ID and global alias for object operations.
func (c *Client) ResolveBucket(ctx context.Context, id string) (bucketID, alias string, err error) {
	bucket, err := c.BucketByID(ctx, id)
	if err != nil {
		return "", "", err
	}

	return bucket.ID, bucket.GlobalAliases[0], nil
}

// BucketByID retrieves a bucket by its trusted Garage identifier.
func (c *Client) BucketByID(ctx context.Context, id string) (Bucket, error) {
	var bucket Bucket
	status, data, err := c.Request(ctx, http.MethodGet, "GetBucketInfo", url.Values{"id": {id}}, nil)
	if err != nil {
		return bucket, err
	}

	if status != http.StatusOK {
		return bucket, errors.New("could not retrieve selected Garage bucket")
	}

	if err := json.Unmarshal(data, &bucket); err != nil {
		return Bucket{}, errors.New("invalid Garage bucket response")
	}

	if bucket.ID != id || len(bucket.GlobalAliases) == 0 {
		return Bucket{}, errors.New("selected bucket is invalid")
	}

	return bucket, nil
}
