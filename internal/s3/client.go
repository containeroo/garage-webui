package s3

import (
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"
)

// ErrUnauthorized reports credentials rejected by Garage's S3 API.
var ErrUnauthorized = errors.New("garage S3 credentials rejected")

// Client performs authenticated S3 reads required by the application backend.
type Client struct {
	signer *Presigner
	http   *http.Client
}

// NewClient constructs an S3 client using the supplied, unmodified key credentials.
func NewClient(endpoint, region, accessKeyID, secret, publicURL string) (*Client, error) {
	signer, err := New(endpoint, region, accessKeyID, secret, publicURL)
	if err != nil {
		return nil, err
	}

	return &Client{
		signer: signer,
		http: &http.Client{
			Timeout:       10 * time.Second,
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		},
	}, nil
}

// ListBuckets verifies S3 credentials and returns the bucket names Garage exposes.
func (c *Client) ListBuckets(ctx context.Context) ([]string, error) {
	address, err := c.signer.BucketsURL(time.Now())
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, address, nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("garage S3 ListBuckets: %w", err)
	}

	defer resp.Body.Close() // nolint:errcheck
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return nil, ErrUnauthorized
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("garage S3 ListBuckets returned HTTP %d", resp.StatusCode)
	}

	var result struct {
		Buckets []struct {
			Name string `xml:"Name"`
		} `xml:"Buckets>Bucket"`
	}
	if err := xml.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode Garage S3 ListBuckets: %w", err)
	}

	names := make([]string, 0, len(result.Buckets))
	for _, bucket := range result.Buckets {
		names = append(names, bucket.Name)
	}

	return names, nil
}
