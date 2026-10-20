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
	t.Parallel()

	entries, err := fs.ReadDir(Files, ".")
	require.NoError(t, err)
	assert.Greater(t, len(entries), 2)
	t.Run("Light", func(t *testing.T) {
		t.Parallel()

		name := "Light"
		data, err := fs.ReadFile(Files, name+".toml")
		require.NoError(t, err)
		assert.Contains(t, string(data), "[colors]")
	})

	t.Run("Dark", func(t *testing.T) {
		t.Parallel()

		name := "Dark"
		data, err := fs.ReadFile(Files, name+".toml")
		require.NoError(t, err)
		assert.Contains(t, string(data), "[colors]")
	})

	t.Run("Nord", func(t *testing.T) {
		t.Parallel()

		name := "Nord"
		data, err := fs.ReadFile(Files, name+".toml")
		require.NoError(t, err)
		assert.Contains(t, string(data), "[colors]")
	})
}

func TestLoadValidatesThemes(t *testing.T) {
	t.Parallel()

	t.Run("valid embedded catalog", func(t *testing.T) {
		t.Parallel()

		catalog, err := Load(Files)
		require.NoError(t, err)
		light, ok := Find(catalog, "light")
		require.True(t, ok)
		assert.Equal(t, "Light", light.Title)
		assert.NotEmpty(t, light.Colors.Background)
	})
	t.Run("missing required colors", func(t *testing.T) {
		t.Parallel()

		_, err := Load(fstest.MapFS{"Light.toml": {Data: []byte("color_scheme = 'dark'\n[colors]\nbackground = '#fff'\n")}})
		require.ErrorContains(t, err, "required")
	})
	t.Run("malformed theme", func(t *testing.T) {
		t.Parallel()

		_, err := Load(fstest.MapFS{"Light.toml": {Data: []byte("invalid = [")}})
		require.ErrorContains(t, err, "parse theme")
	})
	t.Run("missing default theme", func(t *testing.T) {
		t.Parallel()

		_, err := Load(fstest.MapFS{})
		require.ErrorContains(t, err, "default theme")
	})
}

func TestLoadRejectsInvalidThemeFields(t *testing.T) {
	t.Parallel()

	light, err := fs.ReadFile(Files, "Light.toml")
	require.NoError(t, err)
	t.Run("invalid scheme", func(t *testing.T) {
		t.Parallel()

		_, err := Load(fstest.MapFS{"Light.toml": &fstest.MapFile{Data: []byte(strings.Replace(string(light), `color_scheme = "light"`, `color_scheme = "other"`, 1))}})
		require.ErrorContains(t, err, "color_scheme must be light or dark")
	})

	t.Run("unknown field", func(t *testing.T) {
		t.Parallel()

		_, err := Load(fstest.MapFS{"Light.toml": &fstest.MapFile{Data: []byte(string(light) + "\nunknown_color = '#ffffff'\n")}})
		require.ErrorContains(t, err, "parse theme")
	})
}
