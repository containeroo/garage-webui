package server

import (
	"encoding/json"
	"errors"
	"net/http"

	kit "github.com/containeroo/appkit/auth"
	"github.com/gi8lino/garage-webui/internal/application/authentication"
	"github.com/gi8lino/garage-webui/internal/http/auth"
	"github.com/gi8lino/garage-webui/internal/http/middleware"
	httpresponse "github.com/gi8lino/garage-webui/internal/http/response"
)

// administrator retrieves the identity already checked by middleware.
func (s *Server) administrator(w http.ResponseWriter, r *http.Request) (username string, authorized bool) {
	id, ok := middleware.RequestIdentity(r)
	if !ok || id.Role != "admin" {
		httpresponse.Error(w, "authentication required", http.StatusUnauthorized)
		return "", false
	}
	return id.User, true
}

// updateUserProfile updates an account's profile and authorization role.
func (s *Server) updateUserProfile(w http.ResponseWriter, r *http.Request) {
	actor, ok := s.administrator(w, r)
	if !ok {
		return
	}

	var input struct {
		Role        string `json:"role"`
		Email       string `json:"email"`
		DisplayName string `json:"displayName"`
	}
	if !decodeUserRequest(w, r, &input) {
		return
	}

	s.userResult(w, s.auth.SetAccountProfile(r.Context(), actor, r.PathValue("username"), input.Role, input.Email, input.DisplayName))
}

// revertUserProfile restores provider-managed profile fields.
func (s *Server) revertUserProfile(w http.ResponseWriter, r *http.Request) {
	actor, ok := s.administrator(w, r)
	if !ok {
		return
	}

	var input struct {
		Email       bool `json:"email"`
		DisplayName bool `json:"displayName"`
	}
	if !decodeUserRequest(w, r, &input) {
		return
	}

	s.userResult(w, s.auth.RevertProfile(r.Context(), actor, r.PathValue("username"), input.Email, input.DisplayName))
}

// renameUser changes a local account name.
func (s *Server) renameUser(w http.ResponseWriter, r *http.Request) {
	actor, ok := s.administrator(w, r)
	if !ok {
		return
	}

	var input struct {
		Username string `json:"username"`
	}
	if !decodeUserRequest(w, r, &input) {
		return
	}

	s.userResult(w, s.auth.RenameAccount(r.Context(), actor, r.PathValue("username"), input.Username))
}

// updateUserCredential enables or disables local password authentication.
func (s *Server) updateUserCredential(w http.ResponseWriter, r *http.Request) {
	actor, ok := s.administrator(w, r)
	if !ok {
		return
	}

	var input struct {
		Enabled *bool `json:"enabled"`
	}
	if !decodeUserRequest(w, r, &input) {
		return
	}

	if input.Enabled == nil {
		httpresponse.Error(w, "enabled is required", http.StatusBadRequest)
		return
	}

	s.userResult(w, s.auth.SetLocalCredentialEnabled(r.Context(), actor, r.PathValue("username"), *input.Enabled))
}

// decodeUserRequest bounds and decodes account administration requests.
func decodeUserRequest(w http.ResponseWriter, r *http.Request, value any) bool {
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8192))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(value); err != nil {
		httpresponse.Error(w, "invalid request", http.StatusBadRequest)
		return false
	}

	return true
}

// userResult returns a safe response for account administration.
func (s *Server) userResult(w http.ResponseWriter, err error) {
	switch {
	case err == nil:
		w.WriteHeader(http.StatusNoContent)
	case errors.Is(err, auth.ErrInvalidUser):
		httpresponse.Error(w, err.Error(), http.StatusBadRequest)
	case errors.Is(err, auth.ErrUserConflict), errors.Is(err, auth.ErrProtectAccount):
		httpresponse.Error(w, err.Error(), http.StatusConflict)
	case errors.Is(err, auth.ErrUserNotFound):
		httpresponse.Error(w, err.Error(), http.StatusNotFound)
	case errors.Is(err, kit.ErrUnauthenticated):
		httpresponse.Error(w, "authentication required", http.StatusUnauthorized)
	default:
		if s.logger != nil {
			s.logger.Error("user management failed", "error", err)
		}
		httpresponse.Error(w, "user management failed", http.StatusInternalServerError)
	}
}

// listUsers returns local accounts and their identity bindings.
func (s *Server) listUsers(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.administrator(w, r); !ok {
		return
	}

	users, err := s.auth.Accounts(r.Context())
	if err != nil {
		s.userResult(w, err)
		return
	}

	writeJSON(w, http.StatusOK, users)
}

// createUser creates a local account.
func (s *Server) createUser(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.administrator(w, r); !ok {
		return
	}

	var input struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if !decodeUserRequest(w, r, &input) {
		return
	}

	s.userResult(w, s.auth.CreateAccount(r.Context(), input.Username, input.Password))
}

// updateUser updates an account and its optional local credential.
func (s *Server) updateUser(w http.ResponseWriter, r *http.Request) {
	actor, ok := s.administrator(w, r)
	if !ok {
		return
	}

	var input struct {
		Enabled  *bool  `json:"enabled"`
		Password string `json:"password"`
	}
	if !decodeUserRequest(w, r, &input) {
		return
	}

	if input.Enabled == nil {
		httpresponse.Error(w, "enabled is required", http.StatusBadRequest)
		return
	}

	s.userResult(w, s.auth.UpdateAccount(r.Context(), actor, r.PathValue("username"), *input.Enabled, input.Password))
}

// deleteUser deletes an account and invalidates its sessions.
func (s *Server) deleteUser(w http.ResponseWriter, r *http.Request) {
	actor, ok := s.administrator(w, r)
	if !ok {
		return
	}

	s.userResult(w, s.auth.DeleteAccount(r.Context(), actor, r.PathValue("username")))
}

// revokeUserSessions signs an account out of all browser sessions.
func (s *Server) revokeUserSessions(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.administrator(w, r); !ok {
		return
	}

	s.userResult(w, s.auth.RevokeAccountSessions(r.Context(), r.PathValue("username")))
}

// linkIdentity binds a trusted-proxy identity to an account.
func (s *Server) linkIdentity(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.administrator(w, r); !ok {
		return
	}

	var id auth.Identity
	if !decodeUserRequest(w, r, &id) {
		return
	}

	s.userResult(w, s.auth.LinkIdentity(r.Context(), r.PathValue("username"), id))
}

// linkPendingIdentity binds a verified pending OIDC identity to an account.
func (s *Server) linkPendingIdentity(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.administrator(w, r); !ok {
		return
	}
	var input struct {
		Username string        `json:"username"`
		Identity auth.Identity `json:"identity"`
	}
	if !decodeUserRequest(w, r, &input) {
		return
	}
	s.userResult(w, s.auth.LinkPendingIdentity(r.Context(), input.Username, input.Identity))
}

// unlinkIdentity retires a provider identity binding.
func (s *Server) unlinkIdentity(w http.ResponseWriter, r *http.Request) {
	actor, ok := s.administrator(w, r)
	if !ok {
		return
	}

	var id auth.Identity
	if !decodeUserRequest(w, r, &id) {
		return
	}

	if actor == r.PathValue("username") && s.auth.Mode() == id.Provider {
		httpresponse.Error(w, "cannot unlink your own active authentication method", http.StatusConflict)
		return
	}

	s.userResult(w, s.auth.UnlinkIdentity(r.Context(), r.PathValue("username"), id))
}

// relinkProxyIdentity changes the external key of a trusted-proxy identity.
func (s *Server) relinkProxyIdentity(w http.ResponseWriter, r *http.Request) {
	actor, ok := s.administrator(w, r)
	if !ok {
		return
	}

	if actor == r.PathValue("username") && s.auth.Mode() == "proxy" {
		httpresponse.Error(w, "cannot relink your own active authentication method", http.StatusConflict)
		return
	}

	var input struct {
		OldSubject string `json:"oldSubject"`
		NewSubject string `json:"newSubject"`
	}
	if !decodeUserRequest(w, r, &input) {
		return
	}

	s.userResult(w, s.auth.RelinkProxyIdentity(r.Context(), r.PathValue("username"), input.OldSubject, input.NewSubject))
}

// getRegistration returns external registration policy.
func (s *Server) getRegistration(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.administrator(w, r); !ok {
		return
	}

	enabled, err := s.auth.Registration(r.Context())
	if err != nil {
		s.userResult(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]bool{"enabled": enabled, "managed": s.auth.RegistrationManaged()})
}

// setRegistration updates external registration policy.
func (s *Server) setRegistration(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.administrator(w, r); !ok {
		return
	}

	var input struct {
		Enabled *bool `json:"enabled"`
	}
	if !decodeUserRequest(w, r, &input) {
		return
	}

	if input.Enabled == nil {
		httpresponse.Error(w, "enabled is required", http.StatusBadRequest)
		return
	}

	err := s.auth.SetRegistration(r.Context(), *input.Enabled)
	if errors.Is(err, authentication.ErrRegistrationManaged) {
		httpresponse.Error(w, err.Error(), http.StatusConflict)
		return
	}
	s.userResult(w, err)
}

// listPendingIdentities lists OIDC identities waiting for an account link.
func (s *Server) listPendingIdentities(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.administrator(w, r); !ok {
		return
	}

	identities, err := s.auth.PendingIdentities(r.Context())
	if err != nil {
		s.userResult(w, err)
		return
	}

	writeJSON(w, http.StatusOK, identities)
}

// rejectPendingIdentity prevents an OIDC identity from registering.
func (s *Server) rejectPendingIdentity(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.administrator(w, r); !ok {
		return
	}

	var identity auth.Identity
	if !decodeUserRequest(w, r, &identity) {
		return
	}

	s.userResult(w, s.auth.RejectPendingIdentity(r.Context(), identity))
}

// reopenPendingIdentity reopens a rejected OIDC identity.
func (s *Server) reopenPendingIdentity(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.administrator(w, r); !ok {
		return
	}

	var identity auth.Identity
	if !decodeUserRequest(w, r, &identity) {
		return
	}

	s.userResult(w, s.auth.ReopenPendingIdentity(r.Context(), identity))
}

// approvePendingIdentity creates an account for a pending identity.
func (s *Server) approvePendingIdentity(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.administrator(w, r); !ok {
		return
	}

	var input struct {
		Identity auth.Identity `json:"identity"`
		Username string        `json:"username"`
	}
	if !decodeUserRequest(w, r, &input) {
		return
	}

	s.userResult(w, s.auth.ApprovePendingIdentity(r.Context(), input.Identity, input.Username))
}
