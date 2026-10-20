// Package credential defines the Garage credential kinds shared by authorization and object services.
package credential

// Kind identifies a supported Garage authentication mechanism without storing its secret.
type Kind string

const (
	// Admin authenticates a scoped Garage Admin API token.
	Admin Kind = "admin"
	// S3 authenticates an S3 access-key ID and secret.
	S3 Kind = "s3"
)

// Valid reports whether the kind can authenticate a Garage credential.
func (kind Kind) Valid() bool {
	return kind == Admin || kind == S3
}
