package server

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net"
	"net/http"

	"github.com/containeroo/garage-webui/internal/http/auth"
	httpresponse "github.com/containeroo/garage-webui/internal/http/response"
)

// session returns public credential metadata without exposing its secret.
func (s *Server) session(w http.ResponseWriter, r *http.Request) {
	id, err := s.auth.Authenticate(r)
	if err != nil && !errors.Is(err, auth.ErrUnauthenticated) {
		s.logger.Log(r.Context(), slog.LevelError, "garage session validation failed", "error", err)
		httpresponse.Error(w, "garage authentication unavailable; check server logs", http.StatusServiceUnavailable)

		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"mode": id.Credential.Kind, "user": id.User, "scope": id.Scope, "uploadsEnabled": s.auth.S3URL != ""})
}

// login verifies a Garage credential and starts a revocable session.
func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	var credential auth.Credential
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&credential); err != nil {
		httpresponse.Error(w, "invalid sign-in request", http.StatusBadRequest)
		return
	}

	if err := s.auth.Login(w, r, credential); err != nil {
		if errors.Is(err, auth.ErrUnauthenticated) {
			httpresponse.Error(w, "invalid Garage credentials", http.StatusUnauthorized)
		} else {
			s.logger.Log(r.Context(), slog.LevelError, "garage credential validation failed", "error", err)
			httpresponse.Error(w, "garage authentication unavailable; check Garage Admin API availability and server logs", http.StatusServiceUnavailable)
		}

		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// allowLogin throttles requests by the immediate TCP peer on every replica.
// Do not trust X-Forwarded-For without an explicitly trusted proxy boundary.
func (s *Server) allowLogin(r *http.Request) bool {
	peer, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		peer = r.RemoteAddr
	}

	permitted, err := s.loginLimiter.AllowLogin(r.Context(), peer)
	if err != nil {
		s.logger.Error("login rate limit unavailable", "error", err)
		return false // Fail closed when the database is unavailable.
	}

	return permitted
}

// logout revokes the active credential session.
func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	if err := s.auth.Logout(w, r); err != nil {
		httpresponse.Error(w, "could not sign out", http.StatusServiceUnavailable)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
