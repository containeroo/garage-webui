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
	// signer signs outgoing S3 requests.
	signer *Presigner
	// http executes outgoing HTTP requests.
	http *http.Client
}

// NewClient constructs a server-side S3 client; HTTP is allowed independently of the browser origin.
func NewClient(endpoint, region, accessKeyID, secret string) (*Client, error) {
	signer, err := New(endpoint, region, accessKeyID, secret, "")
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

	// result decodes the S3 bucket listing response.
	var result struct {
		// Buckets contains the buckets returned by S3.
		Buckets []struct {
			// Name contains the bucket name returned by S3.
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
