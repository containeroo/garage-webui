package web

import (
	"io/fs"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestEmbeddedFrontend verifies the built entry point and its bundled assets.
func TestEmbedded(t *testing.T) {
	t.Parallel()
	index, err := fs.ReadFile(Assets, "index.html")

	t.Run("index.html", func(t *testing.T) {
		t.Parallel()

		require.NoError(t, err)
		assert.Regexp(t, regexp.MustCompile(`<base href="/"\s*/?>`), string(index))
		notFound, err := fs.ReadFile(Assets, "404.html")
		require.NoError(t, err)

		for _, name := range []string{"favicon-16x16.png", "favicon-32x32.png", "apple-touch-icon.png"} {
			assert.Contains(t, string(index), `href="./`+name+`"`)
			assert.Contains(t, string(notFound), `href="./`+name+`"`)
			content, err := fs.ReadFile(Assets, name)
			require.NoError(t, err, "embedded favicon %s", name)
			assert.NotEmpty(t, content, "embedded favicon %s", name)
		}
	})

	t.Run("assets", func(t *testing.T) {
		t.Parallel()

		references := regexp.MustCompile(`(?:src|href)="(\./assets/[^"]+)"`).FindAllStringSubmatch(string(index), -1)
		require.NotEmpty(t, references, "index must reference built assets")
		var javascript, stylesheet bool
		for _, reference := range references {
			name := strings.TrimPrefix(reference[1], "./")
			content, err := fs.ReadFile(Assets, name)
			require.NoError(t, err, "embedded asset %s", name)
			assert.NotEmpty(t, content, "embedded asset %s", name)
			javascript = javascript || strings.HasSuffix(name, ".js")
			stylesheet = stylesheet || strings.HasSuffix(name, ".css")
		}

		assert.True(t, javascript, "JavaScript bundle is missing")
		assert.True(t, stylesheet, "stylesheet bundle is missing")
	})
}
