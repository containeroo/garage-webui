package auth

import (
	"slices"

	"github.com/containeroo/garage-webui/internal/credential"
	"github.com/containeroo/garage-webui/internal/garage/admin"
)

// Credential is encrypted at rest and is never included in session responses.
type Credential struct {
	// Kind identifies the credential mechanism.
	Kind credential.Kind `json:"kind"`
	// Token contains the Garage admin token.
	Token string `json:"token,omitempty"`
	// AccessKeyID identifies the S3 access key.
	AccessKeyID string `json:"accessKeyId,omitempty"`
	// SecretAccessKey contains the secret used to sign S3 requests.
	SecretAccessKey string `json:"secretAccessKey,omitempty"`
}

// Identity contains fresh Garage metadata and the session's credential.
type Identity struct {
	// Credential holds verified credentials for server-side use.
	Credential Credential
	// User contains the session display name.
	User string
	// Scope lists the allowed admin operations.
	Scope []string
	// Theme contains the selected theme identifier.
	Theme string
}

// Allows checks the current Garage admin-token scope.
func (id Identity) Allows(operation admin.Operation) bool {
	return id.Credential.Kind == credential.Admin && (slices.Contains(id.Scope, "*") || slices.Contains(id.Scope, string(operation)))
}
