package server

import (
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"time"

	kit "github.com/containeroo/appkit/auth"
	"github.com/gi8lino/garage-webui/internal/http/auth"
	httpresponse "github.com/gi8lino/garage-webui/internal/http/response"
)

// session returns authentication state without exposing credentials.
func (s *Server) session(w http.ResponseWriter, r *http.Request) {
	required, err := s.auth.SetupRequired(r.Context())
	if err != nil {
		httpresponse.Error(w, "setup status unavailable", http.StatusServiceUnavailable)
		return
	}

	user, role, err := s.auth.Authenticate(r)
	if err != nil && !errors.Is(err, kit.ErrUnauthenticated) {
		if s.logger != nil {
			s.logger.Error("session authentication failed", "error", err)
		}
		httpresponse.Error(w, "authentication service unavailable", http.StatusServiceUnavailable)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"mode": s.auth.Mode(), "user": user, "role": role, "setupRequired": required, "localLogin": s.auth.LocalLoginEnabled(), "uploadsEnabled": s.objects != nil})
}

// setup creates the first administrator and starts their session.
func (s *Server) setup(w http.ResponseWriter, r *http.Request) {
	var credentials struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&credentials); err != nil {
		httpresponse.Error(w, "invalid request", http.StatusBadRequest)
		return
	}

	err := s.auth.Setup(w, r, credentials.Username, credentials.Password)
	switch {
	case errors.Is(err, auth.ErrSetupComplete):
		httpresponse.Error(w, "setup is already complete", http.StatusConflict)
	case errors.Is(err, auth.ErrInvalidSetup):
		httpresponse.Error(w, err.Error(), http.StatusBadRequest)
	case err != nil:
		s.logger.Error("setup failed", "error", err)
		httpresponse.Error(w, "setup failed", http.StatusInternalServerError)
	default:
		writeJSON(w, http.StatusCreated, map[string]string{"user": credentials.Username})
	}
}

// login authenticates a local administrator.
func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	var credentials struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&credentials); err != nil {
		httpresponse.Error(w, "invalid request", http.StatusBadRequest)
		return
	}

	if err := s.auth.Login(w, r, credentials.Username, credentials.Password); err != nil {
		if errors.Is(err, kit.ErrUnauthenticated) {
			httpresponse.Error(w, "invalid credentials", http.StatusUnauthorized)
		} else {
			if s.logger != nil {
				s.logger.Error("login failed", "error", err)
			}
			httpresponse.Error(w, "authentication service unavailable", http.StatusServiceUnavailable)
		}
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"user": credentials.Username})
}

// allowLogin bounds password attempts per immediate peer and per process.
func (s *Server) allowLogin(r *http.Request) bool {
	peer, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		peer = r.RemoteAddr
	}

	s.loginMu.Lock()
	defer s.loginMu.Unlock()
	if s.loginCounts == nil || time.Since(s.loginWindow) >= time.Minute {
		s.loginWindow = time.Now()
		s.loginCounts = make(map[string]int)
	}

	if len(s.loginCounts) >= 4096 && s.loginCounts[peer] == 0 {
		return false
	}

	s.loginCounts[peer]++
	return s.loginCounts[peer] <= 10
}

// logout revokes the active session.
func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	if err := s.auth.Logout(w, r); err != nil {
		if s.logger != nil {
			s.logger.Error("logout failed", "error", err)
		}
		httpresponse.Error(w, "could not sign out; please try again", http.StatusServiceUnavailable)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// oidcLogin redirects to the identity provider.
func (s *Server) oidcLogin(w http.ResponseWriter, r *http.Request) {
	if err := s.auth.BeginOIDC(w, r); err != nil {
		http.Error(w, "cannot start login", http.StatusBadRequest)
	}
}

// oidcCallback completes the identity-provider flow.
func (s *Server) oidcCallback(w http.ResponseWriter, r *http.Request) {
	if err := s.auth.Callback(w, r); err != nil {
		s.logger.Warn("OIDC login failed", "error", err)
		http.Error(w, "authentication failed", http.StatusUnauthorized)
		return
	}

	// The outer prefix middleware rewrites application-relative redirects.
	http.Redirect(w, r, "/", http.StatusSeeOther)
}
