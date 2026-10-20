package server

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/containeroo/garage-webui/internal/http/auth"
	httpresponse "github.com/containeroo/garage-webui/internal/http/response"
)

// Session returns public credential metadata without exposing its secret.
func Session(authentication *auth.Service, logger *slog.Logger) http.HandlerFunc {
	type sessionResponse struct {
		Mode           string   `json:"mode"`
		User           string   `json:"user"`
		Scope          []string `json:"scope"`
		UploadsEnabled bool     `json:"uploadsEnabled"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := authentication.Authenticate(r)
		if err != nil && !errors.Is(err, auth.ErrUnauthenticated) {
			logger.ErrorContext(r.Context(), "garage authentication failed", "error", err)
			httpresponse.Error(w, "garage authentication unavailable; check server logs", http.StatusServiceUnavailable)

			return
		}

		writeJSON(w, http.StatusOK, sessionResponse{
			Mode:           id.Credential.Kind,
			User:           id.User,
			Scope:          id.Scope,
			UploadsEnabled: authentication.S3URL != "",
		})
	}
}

// Login verifies a Garage credential and starts a revocable session.
func Login(authentication *auth.Service, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var credential auth.Credential
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096))
		decoder.DisallowUnknownFields()

		if err := decoder.Decode(&credential); err != nil {
			httpresponse.Error(w, "invalid sign-in request", http.StatusBadRequest)
			return
		}

		if err := authentication.Login(w, r, credential); err != nil {
			if errors.Is(err, auth.ErrUnauthenticated) {
				httpresponse.Error(w, "invalid Garage credentials", http.StatusUnauthorized)
			} else {
				logger.ErrorContext(r.Context(), "garage credential validation failed", "error", err)
				httpresponse.Error(w, "garage authentication unavailable; check Garage Admin API availability and server logs", http.StatusServiceUnavailable)
			}

			return
		}

		w.WriteHeader(http.StatusNoContent)
	}
}

// Logout revokes the active credential session.
func Logout(authentication *auth.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := authentication.Logout(w, r); err != nil {
			httpresponse.Error(w, "could not sign out", http.StatusServiceUnavailable)
			return
		}

		w.WriteHeader(http.StatusNoContent)
	}
}
