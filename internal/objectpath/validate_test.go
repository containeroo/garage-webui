package objectpath

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestValidDeleteObjectKey restricts marker deletion to a single safe trailing slash.
func TestValidDeleteObjectKey(t *testing.T) {
	t.Parallel()

	t.Run("ordinary key", func(t *testing.T) {
		t.Parallel()

		assert.True(t, ValidDeleteObjectKey("folder/file.txt"))
	})

	t.Run("folder marker", func(t *testing.T) {
		t.Parallel()

		assert.True(t, ValidDeleteObjectKey("folder/"))
	})

	t.Run("reject ambiguous folder marker", func(t *testing.T) {
		t.Parallel()

		assert.False(t, ValidDeleteObjectKey("folder//"))
	})

	t.Run("reject traversal", func(t *testing.T) {
		t.Parallel()

		assert.False(t, ValidDeleteObjectKey("folder/../"))
	})
}
