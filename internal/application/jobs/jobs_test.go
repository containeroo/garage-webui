package jobs

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestValidateSelection keeps recursive deletion confined to explicit, non-root prefixes.
func TestValidateSelection(t *testing.T) {
	t.Parallel()

	t.Run("valid selection", func(t *testing.T) {
		t.Parallel()
		require.NoError(t, ValidateSelection(Selection{Files: []string{"documents/file.txt"}, Folders: []string{"backups/", "logs/2025/"}}))
	})
	t.Run("reject empty selection", func(t *testing.T) {
		t.Parallel()
		require.Error(t, ValidateSelection(Selection{}))
	})
	t.Run("reject root folder", func(t *testing.T) {
		t.Parallel()
		require.Error(t, ValidateSelection(Selection{Folders: []string{""}}))
	})
	t.Run("reject path traversal", func(t *testing.T) {
		t.Parallel()
		require.Error(t, ValidateSelection(Selection{Folders: []string{"backups/../"}}))
	})
	t.Run("reject ambiguous folder", func(t *testing.T) {
		t.Parallel()
		require.Error(t, ValidateSelection(Selection{Folders: []string{"backups//"}}))
	})
	t.Run("reject selection beyond limit", func(t *testing.T) {
		t.Parallel()
		require.Error(t, ValidateSelection(Selection{Files: make([]string, 201)}))
	})
	t.Run("allow zero-byte folder markers", func(t *testing.T) {
		t.Parallel()
		assert.NoError(t, ValidateSelection(Selection{Files: []string{"empty-folder/"}}))
	})
}
