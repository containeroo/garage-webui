package objects

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestValidObjectRequest rejects bad scopes, unsupported actions and pagination abuse.
func TestValidObjectRequest(t *testing.T) {
	for _, tc := range []struct {
		request Request
		valid   bool
	}{
		{Request{
			BucketID: "bucket",
			Action:   "list",
		}, true},
		{Request{
			BucketID:     "bucket",
			Action:       "list",
			Continuation: "opaque+token/=",
		}, true},
		{Request{
			BucketID: "bucket",
			Action:   "download",
			Key:      "folder/file.txt",
		}, true},
		{Request{
			BucketID: "bucket",
			Action:   "delete",
			Key:      "folder/file.txt",
		}, true},
		{Request{
			BucketID: "",
			Action:   "list",
		}, false},
		{Request{
			BucketID: "bucket",
			Action:   "list",
			Key:      "other",
		}, false},
		{Request{
			BucketID:     "bucket",
			Action:       "list",
			Continuation: strings.Repeat("x", 4097),
		}, false},
		{Request{
			BucketID: "bucket",
			Action:   "download",
			Key:      "../file",
		}, false},
		{Request{
			BucketID: "bucket",
			Action:   "delete",
			Key:      "",
		}, false},
		{Request{
			BucketID:     "bucket",
			Action:       "delete",
			Key:          "file",
			Continuation: "other",
		}, false},
		{Request{
			BucketID: "bucket",
			Action:   "unknown",
			Key:      "file",
		}, false},
	} {
		assert.Equal(t, tc.valid, validObjectRequest(tc.request), "%+v", tc.request)
	}
}
