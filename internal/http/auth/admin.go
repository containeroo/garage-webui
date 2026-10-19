package auth

import (
	"context"
	"errors"
	"fmt"

	"github.com/containeroo/garage-webui/internal/garage/admin"
)

// Admin validates the supplied Garage token and reads its active scopes.
func (s *Service) Admin(ctx context.Context, token string) (Identity, error) {
	client, err := admin.New(s.GarageURL, token)
	if err != nil {
		return Identity{}, err
	}

	info, err := client.CurrentAdminToken(ctx)
	if errors.Is(err, admin.ErrUnauthorized) || errors.Is(err, admin.ErrForbidden) {
		return Identity{}, ErrUnauthenticated
	}

	if err != nil {
		return Identity{}, fmt.Errorf("verify Garage admin token: %w", err)
	}

	if info.Expired {
		return Identity{}, ErrUnauthenticated
	}

	return Identity{Credential: Credential{Kind: "admin", Token: token}, User: info.Name, Scope: info.Scope}, nil
}

// AdminClient constructs a client using the session's Garage token.
func (s *Service) AdminClient(id Identity) (*admin.Client, error) {
	if id.Credential.Kind != "admin" {
		return nil, ErrUnauthenticated
	}

	return admin.New(s.GarageURL, id.Credential.Token)
}
