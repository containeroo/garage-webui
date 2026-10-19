package auth

import (
	"context"
	"errors"

	"github.com/containeroo/garage-webui/internal/s3"
)

// Buckets validates S3 credentials through the S3 client, without handling HTTP or XML.
func (s *Service) Buckets(ctx context.Context, c Credential) ([]string, error) {
	client, err := s3.NewClient(s.S3URL, s.Region, c.AccessKeyID, c.SecretAccessKey, s.publicURL)
	if err != nil {
		return nil, ErrUnauthenticated
	}

	names, err := client.ListBuckets(ctx)
	if errors.Is(err, s3.ErrUnauthorized) {
		return nil, ErrUnauthenticated
	}

	return names, err
}
