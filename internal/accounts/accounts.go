// Package accounts defines account-management domain types and errors.
package accounts

import "errors"

// ErrConflict indicates a conflicting account, group, or identity.
var ErrConflict = errors.New("account or identity already exists")

// ErrInvalid indicates an invalid account, group, or identity.
var ErrInvalid = errors.New("invalid account or identity")

// ErrNotFound indicates a missing account, group, or identity.
var ErrNotFound = errors.New("account or identity not found")

// GroupMapping connects an OIDC group claim to a local group.
type GroupMapping struct {
	// ExternalGroup is the OIDC group claim value.
	ExternalGroup string `json:"externalGroup"`
	// Group is the local group name.
	Group string `json:"group"`
}
