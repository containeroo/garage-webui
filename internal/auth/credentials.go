package auth

import (
	"context"

	"github.com/containeroo/garage-webui/internal/credential"
)

// Verify validates exactly one supported Garage credential kind.
func (s *Service) Verify(ctx context.Context, c Credential) (Identity, error) {
	switch c.Kind {
	case credential.Admin:
		if !validAdminCredential(c) {
			return Identity{}, ErrUnauthenticated
		}
		return s.Admin(ctx, c.Token)
	case credential.S3:
		if !validS3Credential(c) {
			return Identity{}, ErrUnauthenticated
		}
		if _, err := s.Buckets(ctx, c); err != nil {
			return Identity{}, err
		}
		return Identity{Credential: c, User: c.AccessKeyID}, nil
	default:
		return Identity{}, ErrUnauthenticated
	}
}

// validAdminCredential rejects mixed authentication methods and oversized secrets.
func validAdminCredential(c Credential) bool {
	return c.Token != "" && len(c.Token) <= 2048 && c.AccessKeyID == "" && c.SecretAccessKey == ""
}

// validS3Credential rejects mixed authentication methods and oversized secrets.
func validS3Credential(c Credential) bool {
	return c.Token == "" && c.AccessKeyID != "" && c.SecretAccessKey != "" &&
		len(c.AccessKeyID) <= 256 && len(c.SecretAccessKey) <= 1024
}
