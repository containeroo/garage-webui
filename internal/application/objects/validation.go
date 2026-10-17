package objects

import (
	"strings"

	"github.com/gi8lino/garage-webui/internal/objectpath"
)

// validObjectRequest validates the selected operation before provisioning keys.
func validObjectRequest(input Request) bool {
	if !validBucketRequestID(input.BucketID) {
		return false
	}

	switch input.Action {
	case "list":
		return validListRequest(input)
	case "download", "preview", "delete", "multipart-start":
		return objectKeyOnly(input)
	case "copy":
		return validCopyRequest(input) && noMultipartParameters(input)
	case "multipart-part":
		return multipartRequest(input) && validPartNumber(input.PartNumber)
	case "multipart-complete", "multipart-abort":
		return multipartRequest(input) && input.PartNumber == 0
	case "multipart-copy-start":
		return validCopyRequest(input) && noMultipartParameters(input)
	case "multipart-copy-part":
		return validMultipartCopyPart(input)
	case "multipart-copy-complete", "multipart-copy-abort":
		return validCopyRequest(input) &&
			validMultipartID(input.UploadID) &&
			input.PartNumber == 0 &&
			input.RangeStart == 0 &&
			input.RangeEnd == 0
	default:
		return false
	}
}

// validBucketRequestID bounds the Admin API bucket identifier.
func validBucketRequestID(id string) bool { return id != "" && len(id) <= 256 }

// validListRequest permits only a prefix and an opaque continuation token.
func validListRequest(input Request) bool {
	if input.Key != "" || input.SourceKey != "" || !noMultipartParameters(input) {
		return false
	}

	return objectpath.ValidPrefix(input.Prefix) && objectpath.ValidContinuationToken(input.Continuation)
}

// objectKeyOnly permits a single object without multipart or listing parameters.
func objectKeyOnly(input Request) bool {
	return objectpath.ValidObjectKey(input.Key) &&
		input.SourceKey == "" &&
		input.Prefix == "" &&
		input.Continuation == "" &&
		noMultipartParameters(input)
}

// noMultipartParameters rejects unrelated part arguments on non-multipart actions.
func noMultipartParameters(input Request) bool {
	return input.UploadID == "" &&
		input.PartNumber == 0 &&
		input.RangeStart == 0 &&
		input.RangeEnd == 0
}

// multipartRequest requires a signed key and bounded upload ID.
func multipartRequest(input Request) bool {
	if !objectpath.ValidObjectKey(input.Key) ||
		input.Prefix != "" ||
		input.SourceKey != "" ||
		input.Continuation != "" {
		return false
	}

	return validMultipartID(input.UploadID) &&
		input.RangeStart == 0 &&
		input.RangeEnd == 0
}

// validCopyRequest rejects cross-bucket paths and identical copy targets.
func validCopyRequest(input Request) bool {
	if !objectpath.ValidObjectKey(input.SourceKey) || !objectpath.ValidObjectKey(input.Key) {
		return false
	}

	return input.SourceKey != input.Key &&
		input.Prefix == "" &&
		input.Continuation == ""
}

// validMultipartID checks Garage's opaque multipart identifier.
func validMultipartID(id string) bool {
	return id != "" && len(id) <= 2048 && !strings.ContainsAny(id, "\x00\r\n")
}

// validPartNumber respects S3's 10,000-part limit.
func validPartNumber(number int) bool { return number >= 1 && number <= 10000 }

// validMultipartCopyPart bounds the byte range and its signing parameters.
func validMultipartCopyPart(input Request) bool {
	if !validCopyRequest(input) || !validMultipartID(input.UploadID) {
		return false
	}

	if !validPartNumber(input.PartNumber) || input.RangeStart < 0 || input.RangeEnd < input.RangeStart {
		return false
	}

	return input.RangeEnd-input.RangeStart < 5<<30
}

// objectMutation identifies all operations requiring editor permissions.
func objectMutation(action string) bool {
	switch action {
	case "delete",
		"copy",
		"multipart-start",
		"multipart-part",
		"multipart-complete",
		"multipart-abort",
		"multipart-copy-start",
		"multipart-copy-part",
		"multipart-copy-complete",
		"multipart-copy-abort":
		return true
	}

	return false
}

// validUploadRequest verifies the size, bucket ID and object key.
func validUploadRequest(bucketID, key string, size int64) bool {
	return validBucketRequestID(bucketID) &&
		objectpath.ValidObjectKey(key) &&
		size >= 1 &&
		size <= maxUploadSize
}
