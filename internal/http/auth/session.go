package auth

import (
	"crypto/sha256"
	"encoding/base64"
	"net/http"

	core "github.com/containeroo/garage-webui/internal/auth"
)

// sessionHash derives the database lookup key from an opaque browser cookie.
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

// Authenticate validates the browser's session against the core authentication service.
func (s *Service) Authenticate(r *http.Request) (Identity, error) {
	return s.Service.Authenticate(r.Context(), sessionHash(r))
}

// Login starts a revocable Garage session and writes its browser cookie.
func (s *Service) Login(w http.ResponseWriter, r *http.Request, credentials Credential) error {
	token, err := s.Service.Login(r.Context(), sessionHash(r), credentials)
	if err != nil {
		return err
	}

	s.cookie(w, cookieName, token, int(core.SessionLifetime.Seconds()))
	return nil
}

// Logout revokes the browser session and expires its cookie.
func (s *Service) Logout(w http.ResponseWriter, r *http.Request) error {
	if err := s.Service.Logout(r.Context(), sessionHash(r)); err != nil {
		return err
	}

	s.cookie(w, cookieName, "", -1)
	return nil
}

// SetTheme updates the theme stored for the active browser session.
func (s *Service) SetTheme(r *http.Request, theme string) error {
	return s.Service.SetTheme(r.Context(), sessionHash(r), theme)
}
