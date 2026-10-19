// Package objectpath validates S3 object paths and pagination parameters.
package objectpath

import "strings"

// ValidPrefix accepts folder paths and search prefixes without traversal components.
func ValidPrefix(prefix string) bool {
	return validPrefixShape(prefix) &&
		!hasTraversalComponent(prefix) &&
		!hasControlCharacters(prefix)
}

// ValidContinuationToken permits a bounded opaque S3 pagination token.
func ValidContinuationToken(token string) bool {
	return len(token) <= 4096 &&
		!strings.ContainsAny(token, "\x00\r\n")
}

// ValidObjectKey accepts an S3 key without control bytes, traversal or ambiguous slashes.
func ValidObjectKey(key string) bool {
	return validObjectKeyShape(key) &&
		validObjectKeyComponents(key) &&
		!hasControlCharacters(key)
}

// validPrefixShape rejects oversized prefixes, leading slashes and backslashes.
func validPrefixShape(prefix string) bool {
	return len(prefix) <= 1024 &&
		!strings.HasPrefix(prefix, "/") &&
		!strings.Contains(prefix, "\\")
}

// validObjectKeyShape rejects empty, oversized and ambiguous keys.
func validObjectKeyShape(key string) bool {
	return key != "" &&
		len(key) <= 1024 &&
		!strings.HasPrefix(key, "/") &&
		!strings.HasSuffix(key, "/") &&
		!strings.Contains(key, "\\")
}

// validObjectKeyComponents rejects empty and traversal components in object keys.
func validObjectKeyComponents(key string) bool {
	for component := range strings.SplitSeq(key, "/") {
		if component == "" || component == "." || component == ".." {
			return false
		}
	}

	return true
}

// hasTraversalComponent reports whether a path contains dot or parent-directory components.
func hasTraversalComponent(path string) bool {
	for component := range strings.SplitSeq(path, "/") {
		if component == "." || component == ".." {
			return true
		}
	}

	return false
}

// hasControlCharacters reports whether a string contains ASCII control characters.
func hasControlCharacters(value string) bool {
	for _, r := range value {
		if r < 0x20 || r == 0x7f {
			return true
		}
	}

	return false
}
