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
		httpresponse.Respond(w, http.StatusOK, profileResponse{Username: id.User, Theme: id.Theme})
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
			httpresponse.Problem(w, http.StatusBadRequest, "invalid theme")
			return
		}

		theme, found := themes.Find(catalog, input.Theme)
		if !found {
			httpresponse.Problem(w, http.StatusBadRequest, "unknown theme")
			return
		}

		if err := authentication.SetTheme(r, theme.Title); err != nil {
			httpresponse.InternalServerError(nil, w, err)
			return
		}

		w.WriteHeader(http.StatusNoContent)
	}
}

// ThemeCatalog exposes the installed themes for the UI.
func ThemeCatalog(catalog []themes.Theme) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		httpresponse.Respond(w, http.StatusOK, catalog)
	}
}
