package endpoint

import (
	"encoding/json"
	"net/http"

	"github.com/containeroo/garage-webui/internal/http/auth"
	"github.com/containeroo/garage-webui/internal/http/middleware"
	httpresponse "github.com/containeroo/garage-webui/internal/http/response"
	"github.com/containeroo/garage-webui/themes"
)

// OwnProfile returns the current Garage credential label and theme.
func OwnProfile() http.HandlerFunc {
	// profileResponse describes the current session profile.
	type profileResponse struct {
		// Username contains the authenticated display name.
		Username string `json:"username"`
		// Theme contains the selected theme identifier.
		Theme string `json:"theme"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		id, _ := middleware.RequestIdentity(r)
		writeJSON(w, http.StatusOK, profileResponse{Username: id.User, Theme: id.Theme})
	}
}

// UpdateOwnTheme validates and stores a theme for the active session.
func UpdateOwnTheme(authentication *auth.Service, catalog []themes.Theme) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// input contains the requested session theme change.
		var input struct {
			// Theme contains the selected theme identifier.
			Theme string `json:"theme"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&input); err != nil {
			httpresponse.Error(w, "invalid theme", http.StatusBadRequest)
			return
		}

		theme, found := themes.Find(catalog, input.Theme)
		if !found {
			httpresponse.Error(w, "unknown theme", http.StatusBadRequest)
			return
		}

		if err := authentication.SetTheme(r, theme.Title); err != nil {
			httpresponse.Error(w, "cannot save theme", http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusNoContent)
	}
}

// ThemeCatalog exposes the installed themes for the UI.
func ThemeCatalog(catalog []themes.Theme) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, catalog)
	}
}
