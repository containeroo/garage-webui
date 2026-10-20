package objects

import (
	"strings"
	"testing"

	"github.com/containeroo/garage-webui/internal/objectaction"
	"github.com/stretchr/testify/assert"
)

// TestObjectSigningValidation rejects malformed and privilege-changing S3 requests.
func TestObjectSigningValidation(t *testing.T) {
	t.Parallel()

	t.Run("valid list prefix", func(t *testing.T) {
		t.Parallel()

		input := Request{
			BucketID: "bucket",
			Action:   objectaction.List,
			Prefix:   "folder/",
		}
		assert.True(t, validObjectRequest(input), "rejected: %+v", input)
	})

	t.Run("valid download", func(t *testing.T) {
		t.Parallel()

		input := Request{
			BucketID: "bucket",
			Action:   objectaction.Download,
			Key:      "folder/file.txt",
		}
		assert.True(t, validObjectRequest(input), "rejected: %+v", input)
	})

	t.Run("valid preview", func(t *testing.T) {
		t.Parallel()

		input := Request{
			BucketID: "bucket",
			Action:   objectaction.Preview,
			Key:      "folder/file.txt",
		}
		assert.True(t, validObjectRequest(input), "rejected: %+v", input)
	})

	t.Run("valid deletion", func(t *testing.T) {
		t.Parallel()

		input := Request{
			BucketID: "bucket",
			Action:   objectaction.Delete,
			Key:      "folder/file.txt",
		}
		assert.True(t, validObjectRequest(input), "rejected: %+v", input)
	})

	t.Run("valid multipart start", func(t *testing.T) {
		t.Parallel()

		input := Request{
			BucketID: "bucket",
			Action:   objectaction.MultipartStart,
			Key:      "folder/file.txt",
		}
		assert.True(t, validObjectRequest(input), "rejected: %+v", input)
	})

	t.Run("valid multipart part", func(t *testing.T) {
		t.Parallel()

		input := Request{
			BucketID:   "bucket",
			Action:     objectaction.MultipartPart,
			Key:        "folder/file.txt",
			UploadID:   "opaqueid",
			PartNumber: 1,
		}
		assert.True(t, validObjectRequest(input), "rejected: %+v", input)
	})

	t.Run("valid multipart completion", func(t *testing.T) {
		t.Parallel()

		input := Request{
			BucketID: "bucket",
			Action:   objectaction.MultipartComplete,
			Key:      "folder/file.txt",
			UploadID: "opaqueid",
		}
		assert.True(t, validObjectRequest(input), "rejected: %+v", input)
	})

	t.Run("valid copy", func(t *testing.T) {
		t.Parallel()

		input := Request{
			BucketID:  "bucket",
			Action:    objectaction.Copy,
			Key:       "new.txt",
			SourceKey: "folder/file.txt",
		}
		assert.True(t, validObjectRequest(input), "rejected: %+v", input)
	})

	t.Run("valid multipart copy start", func(t *testing.T) {
		t.Parallel()

		input := Request{
			BucketID:  "bucket",
			Action:    objectaction.MultipartCopyStart,
			Key:       "new.txt",
			SourceKey: "folder/file.txt",
		}
		assert.True(t, validObjectRequest(input), "rejected: %+v", input)
	})

	t.Run("valid multipart copy part", func(t *testing.T) {
		t.Parallel()

		input := Request{
			BucketID:   "bucket",
			Action:     objectaction.MultipartCopyPart,
			Key:        "new.txt",
			SourceKey:  "folder/file.txt",
			UploadID:   "opaqueid",
			PartNumber: 1,
			RangeStart: 0,
			RangeEnd:   16777215,
		}
		assert.True(t, validObjectRequest(input), "rejected: %+v", input)
	})

	t.Run("valid multipart copy completion", func(t *testing.T) {
		t.Parallel()

		input := Request{
			BucketID:  "bucket",
			Action:    objectaction.MultipartCopyComplete,
			Key:       "new.txt",
			SourceKey: "folder/file.txt",
			UploadID:  "opaqueid",
		}
		assert.True(t, validObjectRequest(input), "rejected: %+v", input)
	})

	t.Run("reject missing bucket", func(t *testing.T) {
		t.Parallel()

		input := Request{
			BucketID: "",
			Action:   objectaction.List,
		}
		assert.False(t, validObjectRequest(input), "accepted: %+v", input)
	})

	t.Run("reject prefix traversal", func(t *testing.T) {
		t.Parallel()

		input := Request{
			BucketID: "bucket",
			Action:   objectaction.List,
			Prefix:   "../secret",
		}
		assert.False(t, validObjectRequest(input), "accepted: %+v", input)
	})

	t.Run("reject upload ID on download", func(t *testing.T) {
		t.Parallel()

		input := Request{
			BucketID: "bucket",
			Action:   objectaction.Download,
			Key:      "file",
			UploadID: "illicit",
		}
		assert.False(t, validObjectRequest(input), "accepted: %+v", input)
	})

	t.Run("reject self copy", func(t *testing.T) {
		t.Parallel()

		input := Request{
			BucketID:  "bucket",
			Action:    objectaction.Copy,
			SourceKey: "file",
			Key:       "file",
		}
		assert.False(t, validObjectRequest(input), "accepted: %+v", input)
	})

	t.Run("reject missing upload ID", func(t *testing.T) {
		t.Parallel()

		input := Request{
			BucketID:   "bucket",
			Action:     objectaction.MultipartPart,
			Key:        "key",
			PartNumber: 1,
		}
		assert.False(t, validObjectRequest(input), "accepted: %+v", input)
	})

	t.Run("reject excessive part number", func(t *testing.T) {
		t.Parallel()

		input := Request{
			BucketID:   "bucket",
			Action:     objectaction.MultipartPart,
			Key:        "key",
			UploadID:   "id",
			PartNumber: 10001,
		}
		assert.False(t, validObjectRequest(input), "accepted: %+v", input)
	})

	t.Run("reject reversed range", func(t *testing.T) {
		t.Parallel()

		input := Request{
			BucketID:   "bucket",
			Action:     objectaction.MultipartCopyPart,
			Key:        "new",
			SourceKey:  "old",
			UploadID:   "id",
			PartNumber: 1,
			RangeStart: 2,
			RangeEnd:   1,
		}
		assert.False(t, validObjectRequest(input), "accepted: %+v", input)
	})

	t.Run("reject excessive range", func(t *testing.T) {
		t.Parallel()

		input := Request{
			BucketID:   "bucket",
			Action:     objectaction.MultipartCopyPart,
			Key:        "new",
			SourceKey:  "old",
			UploadID:   "id",
			PartNumber: 1,
			RangeEnd:   6 << 30,
		}
		assert.False(t, validObjectRequest(input), "accepted: %+v", input)
	})

	t.Run("reject upload ID on copy start", func(t *testing.T) {
		t.Parallel()

		input := Request{
			BucketID:  "bucket",
			Action:    objectaction.MultipartCopyStart,
			Key:       "new",
			SourceKey: "old",
			UploadID:  "unexpected",
		}
		assert.False(t, validObjectRequest(input), "accepted: %+v", input)
	})

	t.Run("reject unknown action", func(t *testing.T) {
		t.Parallel()

		input := Request{
			BucketID: "bucket",
			Action:   "unknown",
			Key:      "new",
		}
		assert.False(t, validObjectRequest(input), "accepted: %+v", input)
	})

	t.Run("reject oversized upload ID", func(t *testing.T) {
		t.Parallel()

		assert.False(t, validMultipartID(strings.Repeat("a", 2049)))
	})
}
