// Package signing defines bucket-scoped signing credentials.
package signing

// Credential contains the secret material for one Garage S3 key.
type Credential struct {
	// AccessKeyID identifies the key.
	AccessKeyID string `json:"accessKeyId"`
	// SecretAccessKey signs requests and must remain private.
	SecretAccessKey string `json:"secretAccessKey"`
	// Expired indicates the key can no longer be used.
	Expired bool `json:"expired"`
}
