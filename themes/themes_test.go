package themes

import (
	"io/fs"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEmbeddedThemes(t *testing.T) {
	entries, err := fs.ReadDir(Files, ".")
	require.NoError(t, err)
	assert.Greater(t, len(entries), 2)
	for _, name := range []string{"Light", "Dark", "Nord"} {
		data, err := fs.ReadFile(Files, name+".toml")
		require.NoError(t, err)
		assert.Contains(t, string(data), "[colors]")
	}
}

func TestLoadValidatesThemes(t *testing.T) {
	catalog, err := Load(Files)
	require.NoError(t, err)
	light, ok := Find(catalog, "light")
	require.True(t, ok)
	assert.Equal(t, "Light", light.Title)
	assert.NotEmpty(t, light.Colors.Background)

	_, err = Load(fstest.MapFS{"Light.toml": &fstest.MapFile{Data: []byte("color_scheme = 'dark'\n[colors]\nbackground = '#fff'\n")}})
	require.ErrorContains(t, err, "required")
	_, err = Load(fstest.MapFS{"Light.toml": &fstest.MapFile{Data: []byte("invalid = [")}})
	require.ErrorContains(t, err, "parse theme")
	_, err = Load(fstest.MapFS{})
	require.ErrorContains(t, err, "default theme")
}

func TestLoadRejectsInvalidThemeFields(t *testing.T) {
	light, err := fs.ReadFile(Files, "Light.toml")
	require.NoError(t, err)
	for _, tc := range []struct{ name, content, problem string }{
		{"invalid scheme", strings.Replace(string(light), `color_scheme = "light"`, `color_scheme = "other"`, 1), "color_scheme must be light or dark"},
		{"unknown field", string(light) + "\nunknown_color = '#ffffff'\n", "parse theme"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Load(fstest.MapFS{"Light.toml": &fstest.MapFile{Data: []byte(tc.content)}})
			require.ErrorContains(t, err, tc.problem)
		})
	}
}
