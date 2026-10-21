package objects

import (
	"strings"
	"testing"

	"github.com/containeroo/garage-webui/internal/objectaction"
	"github.com/stretchr/testify/assert"
)

// TestValidObjectRequest rejects bad scopes, unsupported actions and pagination abuse.
// TestFolderMarkerDeletion allows deleting a marker but never listing or uploading it.
func TestFolderMarkerDeletion(t *testing.T) {
	t.Parallel()

	t.Run("delete marker", func(t *testing.T) {
		t.Parallel()

		assert.True(t, validObjectRequest(Request{BucketID: "bucket", Action: objectaction.Delete, Key: "folder/"}))
	})

	t.Run("reject marker download", func(t *testing.T) {
		t.Parallel()

		assert.False(t, validObjectRequest(Request{BucketID: "bucket", Action: objectaction.Download, Key: "folder/"}))
	})

	t.Run("reject root marker", func(t *testing.T) {
		t.Parallel()

		assert.False(t, validObjectRequest(Request{BucketID: "bucket", Action: objectaction.Delete, Key: "/"}))
	})
}

func TestValidObjectRequest(t *testing.T) {
	t.Parallel()

	t.Run("list bucket", func(t *testing.T) {
		t.Parallel()

		input := Request{BucketID: "bucket", Action: objectaction.List}
		assert.True(t, validObjectRequest(input), "%+v", input)
	})

	t.Run("list with continuation", func(t *testing.T) {
		t.Parallel()

		input := Request{BucketID: "bucket", Action: objectaction.List, Continuation: "opaque+token/="}
		assert.True(t, validObjectRequest(input), "%+v", input)
	})

	t.Run("download object", func(t *testing.T) {
		t.Parallel()

		input := Request{BucketID: "bucket", Action: objectaction.Download, Key: "folder/file.txt"}
		assert.True(t, validObjectRequest(input), "%+v", input)
	})

	t.Run("delete object", func(t *testing.T) {
		t.Parallel()

		input := Request{BucketID: "bucket", Action: objectaction.Delete, Key: "folder/file.txt"}
		assert.True(t, validObjectRequest(input), "%+v", input)
	})

	t.Run("reject missing bucket", func(t *testing.T) {
		t.Parallel()

		input := Request{BucketID: "", Action: objectaction.List}
		assert.False(t, validObjectRequest(input), "%+v", input)
	})

	t.Run("reject object key in listing", func(t *testing.T) {
		t.Parallel()

		input := Request{BucketID: "bucket", Action: objectaction.List, Key: "other"}
		assert.False(t, validObjectRequest(input), "%+v", input)
	})

	t.Run("reject oversized continuation", func(t *testing.T) {
		t.Parallel()

		input := Request{BucketID: "bucket", Action: objectaction.List, Continuation: strings.Repeat("x", 4097)}
		assert.False(t, validObjectRequest(input), "%+v", input)
	})

	t.Run("reject traversal", func(t *testing.T) {
		t.Parallel()

		input := Request{BucketID: "bucket", Action: objectaction.Download, Key: "../file"}
		assert.False(t, validObjectRequest(input), "%+v", input)
	})

	t.Run("reject empty object key", func(t *testing.T) {
		t.Parallel()

		input := Request{BucketID: "bucket", Action: objectaction.Delete, Key: ""}
		assert.False(t, validObjectRequest(input), "%+v", input)
	})

	t.Run("reject continuation on delete", func(t *testing.T) {
		t.Parallel()

		input := Request{BucketID: "bucket", Action: objectaction.Delete, Key: "file", Continuation: "other"}
		assert.False(t, validObjectRequest(input), "%+v", input)
	})

	t.Run("reject unknown action", func(t *testing.T) {
		t.Parallel()

		input := Request{BucketID: "bucket", Action: "unknown", Key: "file"}
		assert.False(t, validObjectRequest(input), "%+v", input)
	})
}
