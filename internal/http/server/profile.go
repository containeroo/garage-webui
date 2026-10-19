package server

import (
	"encoding/json"
	"net/http"

	"github.com/containeroo/garage-webui/internal/http/middleware"
	httpresponse "github.com/containeroo/garage-webui/internal/http/response"
	"github.com/containeroo/garage-webui/themes"
)

// getOwnProfile returns the current Garage credential label and theme.
func (s *Server) getOwnProfile(w http.ResponseWriter, r *http.Request) {
	id, _ := middleware.RequestIdentity(r)
	writeJSON(w, http.StatusOK, map[string]string{"username": id.User, "theme": id.Theme})
}

// updateOwnTheme validates and stores a theme for the active session.
func (s *Server) updateOwnTheme(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Theme string `json:"theme"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&input); err != nil {
		httpresponse.Error(w, "invalid theme", http.StatusBadRequest)
		return
	}

	theme, found := themes.Find(s.themes, input.Theme)
	if !found {
		httpresponse.Error(w, "unknown theme", http.StatusBadRequest)
		return
	}

	if err := s.auth.SetTheme(r, theme.Title); err != nil {
		httpresponse.Error(w, "cannot save theme", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// themeCatalog exposes the installed themes for the UI.
func (s *Server) themeCatalog(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.themes)
}
