// Package objectpath validates S3 object paths and pagination parameters.
package objectpath

import "strings"

// ValidPrefix accepts folder paths and search prefixes without traversal components.
func ValidPrefix(prefix string) bool {
	if prefix == "" {
		return true
	}

	if len(prefix) > 1024 || strings.HasPrefix(prefix, "/") || strings.Contains(prefix, "\\") {
		return false
	}

	for _, component := range strings.Split(prefix, "/") {
		if component == "." || component == ".." {
			return false
		}
	}

	for _, r := range prefix {
		if r < 0x20 || r == 0x7f {
			return false
		}
	}

	return true
}

// ValidContinuationToken permits a bounded opaque S3 pagination token.
func ValidContinuationToken(token string) bool {
	if len(token) > 4096 {
		return false
	}

	return !strings.ContainsAny(token, "\x00\r\n")
}

// ValidObjectKey accepts an S3 key without control bytes, traversal or ambiguous slashes.

func ValidObjectKey(key string) bool {
	if !validObjectKeyShape(key) {
		return false
	}

	for part := range strings.SplitSeq(key, "/") {
		if part == "" || part == "." || part == ".." {
			return false
		}
	}

	for _, r := range key {
		if r < 0x20 || r == 0x7f {
			return false
		}
	}

	return true
}

// validObjectKeyShape rejects empty, oversized and ambiguous keys.
func validObjectKeyShape(key string) bool {
	return key != "" &&
		len(key) <= 1024 &&
		!strings.HasPrefix(key, "/") &&
		!strings.HasSuffix(key, "/") &&
		!strings.Contains(key, "\\")
}
