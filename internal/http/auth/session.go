package auth

import (
	"context"
	"errors"
	"net/http"

	kit "github.com/containeroo/appkit/auth"
)

// Mode returns the configured authentication mode.
func (s *Service) Mode() string { return string(s.mode) }

// OIDCIssuer returns the configured OIDC issuer, if enabled.
func (s *Service) OIDCIssuer() string { return s.issuer }

// LocalLoginEnabled reports whether local sign-in is available.
func (s *Service) LocalLoginEnabled() bool { return s.mode == kit.ModeLocal || s.localLogin }

// SetupRequired reports whether local authentication needs its first administrator.
func (s *Service) SetupRequired(ctx context.Context) (bool, error) {
	return s.Service.SetupRequired(ctx)
}

// Setup atomically creates the first administrator and their browser session.
func (s *Service) Setup(w http.ResponseWriter, r *http.Request, username, password string) error {
	token, err := s.Service.Setup(r.Context(), username, password)
	if err != nil {
		return err
	}

	s.cookie(w, "garage_webui_session", token, 12*60*60)
	return nil
}

// User authenticates a proxy identity or a stored session.
func (s *Service) User(r *http.Request) (string, error) {
	username, _, err := s.Authenticate(r)
	return username, err
}

// Authenticate returns the local user and effective authorization role.
func (s *Service) Authenticate(r *http.Request) (username, effectiveRole string, err error) {
	user, role, err := s.authenticateSession(r)
	if err == nil {
		return user, role, nil
	}
	if !errors.Is(err, kit.ErrUnauthenticated) {
		return "", "", err
	}

	if s.mode == kit.ModeProxy {
		user, err := s.proxy.Authenticate(r)
		if err == nil {
			role, err := s.Role(r.Context(), user.username)
			if err != nil {
				return "", "", err
			}

			if user.admin {
				role = "admin"
			}

			return user.username, role, nil
		}
		if !errors.Is(err, kit.ErrUnauthenticated) {
			return "", "", err
		}
	}

	return "", "", kit.ErrUnauthenticated
}

// authenticateSession resolves a stored local or OIDC browser session.
func (s *Service) authenticateSession(r *http.Request) (username, effectiveRole string, err error) {
	cookie, err := r.Cookie("garage_webui_session")
	if err != nil {
		return "", "", kit.ErrUnauthenticated
	}

	return s.AuthenticateSession(r.Context(), cookie.Value)
}

// Login verifies local credentials and creates a session.
func (s *Service) Login(w http.ResponseWriter, r *http.Request, username, password string) error {
	token, err := s.Service.Login(r.Context(), username, password)
	if err != nil {
		return err
	}

	s.cookie(w, "garage_webui_session", token, 12*60*60)
	return nil
}

// Logout revokes the current session.
func (s *Service) Logout(w http.ResponseWriter, r *http.Request) error {
	if cookie, err := r.Cookie("garage_webui_session"); err == nil {
		if err := s.Service.Logout(r.Context(), cookie.Value); err != nil {
			return err
		}
	}

	s.cookie(w, "garage_webui_session", "", -1)
	return nil
}

// session creates a session only while its account and credential remain valid.
func (s *Service) session(w http.ResponseWriter, r *http.Request, user, passwordHash string, identity *Identity, externalAdmin bool) error {
	token, err := s.CreateSession(r.Context(), user, passwordHash, identity, externalAdmin)
	if err != nil {
		return err
	}

	s.cookie(w, "garage_webui_session", token, 12*60*60)
	return nil
}
