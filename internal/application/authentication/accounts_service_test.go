package authentication

import (
	"context"
	"testing"

	"github.com/containeroo/appkit/auth/local"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// credentialRepository captures account writes for service tests.
type credentialRepository struct {
	accountRepository
	hash string
}

// CreateAccount records a stored password hash.
func (r *credentialRepository) CreateAccount(_ context.Context, _, hash string) error {
	r.hash = hash
	return nil
}

// TestCreateAccountHashesCredential checks that persistence never receives a plaintext password.
func TestCreateAccountHashesCredential(t *testing.T) {
	repository := &credentialRepository{}
	service := &Service{accounts: repository}
	password := "a-secure-password"
	require.NoError(t, service.CreateAccount(context.Background(), "alice", password))
	assert.NotEqual(t, password, repository.hash)
	assert.NoError(t, local.VerifyPassword(repository.hash, password))
	assert.ErrorIs(t, service.CreateAccount(context.Background(), "alice", "short"), ErrInvalidUser)
}
