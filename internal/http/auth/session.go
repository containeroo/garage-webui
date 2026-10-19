package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"
)

// sessionHash derives the database key from the opaque browser cookie.
func sessionHash(r *http.Request) []byte {
	cookie, err := r.Cookie(cookieName)
	if err != nil {
		return nil
	}

	token, err := base64.RawURLEncoding.DecodeString(cookie.Value)
	if err != nil || len(token) != 32 {
		return nil
	}

	hash := sha256.Sum256(token)

	return hash[:]
}

// Authenticate decrypts a live session and revalidates its Garage credential.
func (s *Service) Authenticate(r *http.Request) (Identity, error) {
	hash := sessionHash(r)
	if hash == nil {
		return Identity{}, ErrUnauthenticated
	}

	encrypted, theme, err := s.store.LoadSession(r.Context(), hash)
	if errors.Is(err, pgx.ErrNoRows) {
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

	var credential Credential
	if json.Unmarshal(plaintext, &credential) != nil {
		return Identity{}, ErrUnauthenticated
	}

	id, err := s.Verify(r.Context(), credential)
	id.Theme = theme

	return id, err
}

// Login verifies credentials and rotates the browser session.
func (s *Service) Login(w http.ResponseWriter, r *http.Request, c Credential) error {
	if _, err := s.Verify(r.Context(), c); err != nil {
		return err
	}
	// Store only the credential fields for the selected sign-in method.
	if c.Kind == "admin" {
		c.AccessKeyID = ""
		c.SecretAccessKey = ""
	} else {
		c.Token = ""
	}

	token := make([]byte, 32)
	if _, err := rand.Read(token); err != nil {
		return err
	}

	hash := sha256.Sum256(token)
	plaintext, err := json.Marshal(c)
	if err != nil {
		return err
	}

	nonce := make([]byte, s.encryption.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return err
	}

	encrypted := s.encryption.Seal(nonce, nonce, plaintext, hash[:])
	if err := s.store.SaveSession(r.Context(), hash[:], encrypted, time.Now().Add(sessionLifetime)); err != nil {
		return err
	}

	if old := sessionHash(r); old != nil {
		if err := s.store.DeleteSession(r.Context(), old); err != nil {
			_ = s.store.DeleteSession(r.Context(), hash[:])
			return err
		}
	}

	s.cookie(w, cookieName, base64.RawURLEncoding.EncodeToString(token), int(sessionLifetime.Seconds()))

	return nil
}

// Logout revokes the shared session and clears the browser cookie.
func (s *Service) Logout(w http.ResponseWriter, r *http.Request) error {
	if hash := sessionHash(r); hash != nil {
		if err := s.store.DeleteSession(r.Context(), hash); err != nil {
			return err
		}
	}

	s.cookie(w, cookieName, "", -1)

	return nil
}

// SetTheme stores the current browser session's appearance preference.
func (s *Service) SetTheme(r *http.Request, theme string) error {
	return s.store.SetSessionTheme(r.Context(), sessionHash(r), theme)
}
