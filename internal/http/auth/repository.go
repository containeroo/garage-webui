package auth

import (
	"context"

	"github.com/gi8lino/garage-webui/internal/application/authentication"
)

// sessionRepository persists OIDC authorization state.
type sessionRepository interface {
	SaveOIDCState(context.Context, []byte, string) error
	ConsumeOIDCState(context.Context, []byte) (string, error)
}

// repository provides the persistence needed by authentication.
type repository interface {
	authentication.Repository
	sessionRepository
}
