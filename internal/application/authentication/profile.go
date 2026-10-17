package authentication

import (
	"context"

	"github.com/containeroo/appkit/auth/local"
)

// Profile contains the fields a user can edit independently.
type Profile struct {
	// Username is the account name.
	Username string `json:"username"`
	// Email is the account email.
	Email string `json:"email"`
	// DisplayName is the account display name.
	DisplayName string `json:"displayName"`
	// Theme is the user’s selected built-in theme.
	Theme string `json:"theme"`
	// LocalCredentialEnabled reports whether the user can use a local password.
	LocalCredentialEnabled bool `json:"localCredentialEnabled"`
}

// ChangeOwnPassword verifies and replaces the user's local credential.
func (s *Service) ChangeOwnPassword(ctx context.Context, username, current, password string) error {
	if !validPassword(password) {
		return ErrInvalidUser
	}

	existing, err := s.sessions.PasswordHash(ctx, username)
	if err != nil || local.VerifyPassword(existing, current) != nil {
		return ErrInvalidUser
	}

	hash, err := local.HashPassword(password)
	if err != nil {
		return err
	}

	return s.accounts.ChangeOwnPassword(ctx, username, existing, hash)
}

// UpdateOwnTheme changes only the authenticated user's theme preference.
func (s *Service) UpdateOwnTheme(ctx context.Context, username, theme string) error {
	if len(theme) == 0 || len(theme) > 128 {
		return ErrInvalidUser
	}
	return s.accounts.UpdateOwnTheme(ctx, username, theme)
}
