package server

import (
	"net/http"

	"github.com/gi8lino/garage-webui/internal/http/middleware"
	httpresponse "github.com/gi8lino/garage-webui/internal/http/response"
	"github.com/gi8lino/garage-webui/themes"
)

// profileUser retrieves the identity checked by middleware.
func (s *Server) profileUser(w http.ResponseWriter, r *http.Request) (username string, authenticated bool) {
	id, ok := middleware.RequestIdentity(r)
	if !ok {
		httpresponse.Error(w, "authentication required", http.StatusUnauthorized)
		return "", false
	}

	return id.User, true
}

// getOwnProfile returns the current user's profile.
func (s *Server) getOwnProfile(w http.ResponseWriter, r *http.Request) {
	user, ok := s.profileUser(w, r)
	if !ok {
		return
	}

	profile, err := s.auth.Profile(r.Context(), user)
	if err != nil {
		s.userResult(w, err)
		return
	}

	writeJSON(w, http.StatusOK, profile)
}

// updateOwnProfile updates the current user's profile.
func (s *Server) updateOwnProfile(w http.ResponseWriter, r *http.Request) {
	user, ok := s.profileUser(w, r)
	if !ok {
		return
	}

	var input struct {
		Email       string `json:"email"`
		DisplayName string `json:"displayName"`
	}
	if !decodeUserRequest(w, r, &input) {
		return
	}

	s.userResult(w, s.auth.UpdateOwnProfile(r.Context(), user, input.Email, input.DisplayName))
}

// changeOwnPassword verifies and replaces the current user's local password.
func (s *Server) changeOwnPassword(w http.ResponseWriter, r *http.Request) {
	user, ok := s.profileUser(w, r)
	if !ok {
		return
	}

	var input struct {
		Current  string `json:"current"`
		Password string `json:"password"`
	}
	if !decodeUserRequest(w, r, &input) {
		return
	}

	s.userResult(w, s.auth.ChangeOwnPassword(r.Context(), user, input.Current, input.Password))
}

// updateOwnTheme saves the caller's theme without changing their profile.
func (s *Server) updateOwnTheme(w http.ResponseWriter, r *http.Request) {
	user, ok := s.profileUser(w, r)
	if !ok {
		return
	}
	var input struct {
		Theme string `json:"theme"`
	}
	if !decodeUserRequest(w, r, &input) {
		return
	}
	theme, found := themes.Find(s.themes, input.Theme)
	if !found {
		httpresponse.Error(w, "unknown theme", http.StatusBadRequest)
		return
	}
	s.userResult(w, s.auth.UpdateOwnTheme(r.Context(), user, theme.Title))
}

// themeCatalog returns the validated built-in themes.
func (s *Server) themeCatalog(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.themes)
}
