package server

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gi8lino/garage-webui/themes"
	"github.com/stretchr/testify/assert"
)

var testThemes, _ = themes.Load(themes.Files)

func TestThemeCatalogUsesInjectedThemes(t *testing.T) {
	s := &Server{themes: []themes.Theme{{Title: "Example", ColorScheme: themes.ColorSchemeLight}}}
	w := httptest.NewRecorder()
	s.themeCatalog(w, httptest.NewRequest(http.MethodGet, "/api/themes", nil))
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), `"title":"Example"`)
	assert.NotContains(t, w.Body.String(), `"title":"Nord"`)
}
