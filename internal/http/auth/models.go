package auth

import "github.com/gi8lino/garage-webui/internal/application/authentication"

// Identity describes a provider identity linked to a local account.
type Identity = authentication.Identity

// GroupMapping connects an OIDC group to a local group.
type GroupMapping = authentication.GroupMapping

// ErrUserConflict indicates a conflicting account or identity.
var ErrUserConflict = authentication.ErrUserConflict

// ErrInvalidUser indicates invalid account data.
var ErrInvalidUser = authentication.ErrInvalidUser

// ErrProtectAccount prevents disabling or deleting the current account.
var ErrProtectAccount = authentication.ErrProtectAccount

// ErrUserNotFound indicates a missing account or identity.
var ErrUserNotFound = authentication.ErrUserNotFound
