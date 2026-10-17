package authentication

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"

	kit "github.com/containeroo/appkit/auth"
	"github.com/containeroo/appkit/auth/local"
)

// sessionRepository persists setup and browser sessions.
type sessionRepository interface {
	SetupRequired(context.Context) (bool, error)
	Setup(context.Context, string, string, string, []byte) error
	Session(context.Context, []byte, string, bool) (username, role string, externalAdmin bool, err error)
	PasswordHash(context.Context, string) (string, error)
	DeleteSession(context.Context, []byte) error
	CreateSession(context.Context, string, string, *Identity, bool, string, []byte) error
}

// LocalLoginEnabled reports whether local sign-in is available.
func (s *Service) LocalLoginEnabled() bool { return s.mode == kit.ModeLocal || s.localLogin }

// SetupRequired reports whether the first local administrator is required.
func (s *Service) SetupRequired(ctx context.Context) (bool, error) {
	if s.mode != kit.ModeLocal {
		return false, nil
	}

	return s.sessions.SetupRequired(ctx)
}

// Setup creates the first administrator and returns its session token.
func (s *Service) Setup(ctx context.Context, username, password string) (string, error) {
	if s.mode != kit.ModeLocal {
		return "", ErrSetupComplete
	}

	if !validUsername(username) || !validPassword(password) {
		return "", ErrInvalidSetup
	}

	passwordHash, err := local.HashPassword(password)
	if err != nil {
		return "", fmt.Errorf("hash password: %w", err)
	}

	token, err := randomToken()
	if err != nil {
		return "", fmt.Errorf("create session token: %w", err)
	}

	hash := sha256.Sum256([]byte(token))
	if err := s.sessions.Setup(ctx, username, passwordHash, string(s.mode), hash[:]); err != nil {
		return "", err
	}

	return token, nil
}

// Login verifies a local credential and returns a session token.
func (s *Service) Login(ctx context.Context, username, password string) (string, error) {
	if !s.LocalLoginEnabled() {
		return "", kit.ErrUnauthenticated
	}

	hash, err := s.sessions.PasswordHash(ctx, username)
	if err != nil {
		return "", err
	}
	if local.VerifyPassword(hash, password) != nil {
		return "", kit.ErrUnauthenticated
	}

	return s.CreateSession(ctx, username, hash, nil, false)
}

// AuthenticateSession resolves a session token to its effective role.
func (s *Service) AuthenticateSession(ctx context.Context, token string) (username, effectiveRole string, err error) {
	hash := sha256.Sum256([]byte(token))
	user, role, externalAdmin, err := s.sessions.Session(ctx, hash[:], string(s.mode), s.LocalLoginEnabled())
	if err != nil {
		return "", "", err
	}

	if externalAdmin {
		role = "admin"
	}

	return user, role, nil
}

// Logout revokes a session token.
func (s *Service) Logout(ctx context.Context, token string) error {
	hash := sha256.Sum256([]byte(token))
	return s.sessions.DeleteSession(ctx, hash[:])
}

// CreateSession verifies a credential or identity and returns a session token.
func (s *Service) CreateSession(
	ctx context.Context,
	user, passwordHash string,
	identity *Identity,
	externalAdmin bool,
) (string, error) {
	token, err := randomToken()
	if err != nil {
		return "", err
	}

	hash := sha256.Sum256([]byte(token))
	mode := string(s.mode)
	if identity == nil {
		mode = string(kit.ModeLocal)
	}

	if err := s.sessions.CreateSession(ctx, user, passwordHash, identity, externalAdmin, mode, hash[:]); err != nil {
		return "", err
	}

	return token, nil
}

// randomToken returns a cryptographically random URL-safe token.
func randomToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}

	return base64.RawURLEncoding.EncodeToString(b), nil
}
