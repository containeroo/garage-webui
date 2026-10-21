package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"time"

	"github.com/containeroo/garage-webui/internal/credential"
	"github.com/containeroo/garage-webui/internal/storage"
)

// Authenticate loads an encrypted session and revalidates its Garage credential.
func (s *Service) Authenticate(ctx context.Context, hash []byte) (Identity, error) {
	if len(hash) != sha256.Size {
		return Identity{}, ErrUnauthenticated
	}

	encrypted, theme, err := s.store.LoadSession(ctx, hash)
	if errors.Is(err, storage.ErrSessionNotFound) {
		return Identity{}, ErrUnauthenticated
	}
	if err != nil {
		return Identity{}, err
	}

	if len(encrypted) < s.encryption.NonceSize() {
		return Identity{}, ErrUnauthenticated
	}

	plaintext, err := s.encryption.Open(nil, encrypted[:s.encryption.NonceSize()], encrypted[s.encryption.NonceSize():], hash)
	if err != nil {
		return Identity{}, ErrUnauthenticated
	}

	var credentials Credential
	if json.Unmarshal(plaintext, &credentials) != nil {
		return Identity{}, ErrUnauthenticated
	}

	id, err := s.Verify(ctx, credentials)
	id.Theme = theme

	return id, err
}

// Login verifies the credentials and atomically replaces a previous session when present.
// The returned opaque token is intended only for the HTTP adapter's secure cookie.
func (s *Service) Login(ctx context.Context, oldHash []byte, credentials Credential) (string, error) {
	if _, err := s.Verify(ctx, credentials); err != nil {
		return "", err
	}

	if credentials.Kind == credential.Admin {
		credentials.AccessKeyID = ""
		credentials.SecretAccessKey = ""
	} else {
		credentials.Token = ""
	}

	token := make([]byte, 32)
	if _, err := rand.Read(token); err != nil {
		return "", err
	}

	hash := sha256.Sum256(token)
	plaintext, err := json.Marshal(credentials)
	if err != nil {
		return "", err
	}

	nonce := make([]byte, s.encryption.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}

	encrypted := s.encryption.Seal(nonce, nonce, plaintext, hash[:])
	if err := s.store.SaveSession(ctx, hash[:], encrypted, time.Now().Add(SessionLifetime)); err != nil {
		return "", err
	}

	if len(oldHash) == sha256.Size {
		if err := s.store.DeleteSession(ctx, oldHash); err != nil {
			_ = s.store.DeleteSession(ctx, hash[:])
			return "", err
		}
	}

	return base64.RawURLEncoding.EncodeToString(token), nil
}

// Logout revokes a session when its opaque token was valid.
func (s *Service) Logout(ctx context.Context, hash []byte) error {
	if len(hash) != sha256.Size {
		return nil
	}

	return s.store.DeleteSession(ctx, hash)
}

// SetTheme persists the current session's selected theme.
func (s *Service) SetTheme(ctx context.Context, hash []byte, theme string) error {
	return s.store.SetSessionTheme(ctx, hash, theme)
}
