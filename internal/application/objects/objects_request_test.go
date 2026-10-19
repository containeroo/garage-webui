package objects

import (
	"strings"
	"testing"
)

// TestObjectSigningValidation rejects malformed and privilege-changing S3 requests.
func TestObjectSigningValidation(t *testing.T) {
	base := Request{
		BucketID: "bucket",
		Key:      "folder/file.txt",
	}
	good := []Request{
		{
			BucketID: "bucket",
			Action:   "list",
			Prefix:   "folder/",
		},
		{
			BucketID: "bucket",
			Action:   "download",
			Key:      base.Key,
		},
		{
			BucketID: "bucket",
			Action:   "preview",
			Key:      base.Key,
		},
		{
			BucketID: "bucket",
			Action:   "delete",
			Key:      base.Key,
		},
		{
			BucketID: "bucket",
			Action:   "multipart-start",
			Key:      base.Key,
		},
		{
			BucketID:   "bucket",
			Action:     "multipart-part",
			Key:        base.Key,
			UploadID:   "opaqueid",
			PartNumber: 1,
		},
		{
			BucketID: "bucket",
			Action:   "multipart-complete",
			Key:      base.Key,
			UploadID: "opaqueid",
		},
		{
			BucketID:  "bucket",
			Action:    "copy",
			Key:       "new.txt",
			SourceKey: base.Key,
		},
		{
			BucketID:  "bucket",
			Action:    "multipart-copy-start",
			Key:       "new.txt",
			SourceKey: base.Key,
		},
		{
			BucketID:   "bucket",
			Action:     "multipart-copy-part",
			Key:        "new.txt",
			SourceKey:  base.Key,
			UploadID:   "opaqueid",
			PartNumber: 1,
			RangeStart: 0,
			RangeEnd:   16777215,
		},
		{
			BucketID:  "bucket",
			Action:    "multipart-copy-complete",
			Key:       "new.txt",
			SourceKey: base.Key,
			UploadID:  "opaqueid",
		},
	}
	for _, input := range good {
		if !validObjectRequest(input) {
			t.Errorf("rejected: %+v", input)
		}
	}

	bad := []Request{
		{
			BucketID: "",
			Action:   "list",
		},
		{
			BucketID: "bucket",
			Action:   "list",
			Prefix:   "../secret",
		},
		{
			BucketID: "bucket",
			Action:   "download",
			Key:      "file",
			UploadID: "illicit",
		},
		{
			BucketID:  "bucket",
			Action:    "copy",
			SourceKey: "file",
			Key:       "file",
		},
		{
			BucketID:   "bucket",
			Action:     "multipart-part",
			Key:        "key",
			PartNumber: 1,
		},
		{
			BucketID:   "bucket",
			Action:     "multipart-part",
			Key:        "key",
			UploadID:   "id",
			PartNumber: 10001,
		},
		{
			BucketID:   "bucket",
			Action:     "multipart-copy-part",
			Key:        "new",
			SourceKey:  "old",
			UploadID:   "id",
			PartNumber: 1,
			RangeStart: 2,
			RangeEnd:   1,
		},
		{
			BucketID:   "bucket",
			Action:     "multipart-copy-part",
			Key:        "new",
			SourceKey:  "old",
			UploadID:   "id",
			PartNumber: 1,
			RangeEnd:   6 << 30,
		},
		{
			BucketID:  "bucket",
			Action:    "multipart-copy-start",
			Key:       "new",
			SourceKey: "old",
			UploadID:  "unexpected",
		},
		{
			BucketID: "bucket",
			Action:   "unknown",
			Key:      "new",
		},
	}
	for _, input := range bad {
		if validObjectRequest(input) {
			t.Errorf("accepted: %+v", input)
		}
	}

	if validMultipartID(strings.Repeat("a", 2049)) {
		t.Fatal("unbounded upload ID")
	}
}
