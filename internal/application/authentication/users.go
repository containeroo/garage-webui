package authentication

import (
	"context"
	"errors"
	"strings"

	"github.com/containeroo/appkit/auth/local"
	"github.com/gi8lino/garage-webui/internal/accounts"
)

// ErrUserConflict indicates a conflicting account or identity.
var ErrUserConflict = accounts.ErrConflict

// ErrInvalidUser indicates an invalid account or identity.
var ErrInvalidUser = accounts.ErrInvalid

// ErrProtectAccount prevents disabling or deleting the current account.
var ErrProtectAccount = errors.New("cannot disable or delete your own account")

// ErrUserNotFound indicates a missing account or identity.
var ErrUserNotFound = accounts.ErrNotFound

// Account describes a local account without exposing its password.
type Account struct {
	// Username is the local account name.
	Username string `json:"username"`
	// Role controls the user's access.
	Role string `json:"role"`
	// Email is the account's email address.
	Email string `json:"email"`
	// DisplayName is the account's display name.
	DisplayName string `json:"displayName"`
	// EmailOverridden reports whether an administrator changed the provider email.
	EmailOverridden bool `json:"emailOverridden"`
	// DisplayNameOverridden reports whether the provider display name was changed.
	DisplayNameOverridden bool `json:"displayNameOverridden"`
	// Groups lists local group memberships.
	Groups []string `json:"groups"`
	// ExternalGroups lists groups assigned through OIDC mappings.
	ExternalGroups []string `json:"externalGroups"`
	// Enabled controls whether the account can sign in.
	Enabled bool `json:"enabled"`
	// HasPassword reports whether a local password is configured.
	HasPassword bool `json:"hasPassword"`
	// LocalCredentialEnabled controls local password sign-in.
	LocalCredentialEnabled bool `json:"localCredentialEnabled"`
	// Identities contains the account's external bindings.
	Identities []Identity `json:"identities"`
}

// Identity describes a provider identity linked to a local account.
type Identity struct {
	// Provider is proxy or oidc.
	Provider string `json:"provider"`
	// Issuer identifies the OIDC provider, or is empty for proxy identities.
	Issuer string `json:"issuer"`
	// Subject is the provider's stable identity.
	Subject string `json:"subject"`
}

// PendingIdentity describes an OIDC identity awaiting an administrator decision.
type PendingIdentity struct {
	// Identity is the verified provider identity.
	Identity
	// Username is the suggested account name.
	Username string `json:"username"`
	// Status is pending or rejected.
	Status string `json:"status"`
}

// validUsername reports whether an account name can be stored safely.
func validUsername(name string) bool {
	return name != "" &&
		name == strings.TrimSpace(name) &&
		len(name) <= 128 &&
		!strings.ContainsRune(name, '\x00')
}

// validOptionalPassword permits an absent password or a valid local password.
func validOptionalPassword(password string) bool {
	return password == "" || validPassword(password)
}

// validIdentity reports whether an identity has a supported, complete key.
func validIdentity(id Identity) bool {
	if !validIdentityParts(id) {
		return false
	}

	return id.Provider == "proxy" &&
		id.Issuer == "" ||
		id.Provider == "oidc" &&
			id.Issuer != ""
}

// validIdentityParts checks identity field lengths and subject formatting.
func validIdentityParts(id Identity) bool {
	return id.Subject != "" &&
		strings.TrimSpace(id.Subject) == id.Subject &&
		len(id.Subject) <= 1024 &&
		len(id.Issuer) <= 2048
}

// accountRepository persists account and identity operations.
type accountRepository interface {
	Accounts(context.Context) ([]Account, error)
	SetLocalCredentialEnabled(context.Context, string, string, bool) error
	Role(context.Context, string) (string, error)
	SetAccountProfile(context.Context, string, string, string, string, string) error
	Registration(context.Context) (bool, error)
	SetRegistration(context.Context, bool) error
	CreateAccount(context.Context, string, string) error
	UpdateAccount(context.Context, string, string, bool, string) error
	DeleteAccount(context.Context, string, string) error
	LinkIdentity(context.Context, string, Identity) error
	LinkPendingIdentity(context.Context, string, Identity) error
	PendingIdentities(context.Context) ([]PendingIdentity, error)
	RejectPendingIdentity(context.Context, Identity) error
	ReopenPendingIdentity(context.Context, Identity) error
	ApprovePendingIdentity(context.Context, Identity, string) error
	UnlinkIdentity(context.Context, string, Identity) error
	RevokeAccountSessions(context.Context, string) error
	RelinkProxyIdentity(context.Context, string, string, string) error
	ResolveExternal(context.Context, string, string, string, string, *bool) (string, error)
	Profile(context.Context, string) (Profile, error)
	UpdateOwnProfile(context.Context, string, string, string) error
	UpdateOwnTheme(context.Context, string, string) error
	ChangeOwnPassword(context.Context, string, string, string) error
	RenameAccount(context.Context, string, string, string) error
	RefreshExternalProfile(context.Context, string, Identity, string, string, string) error
	RevertProfile(context.Context, string, string, bool, bool) error
}

// CreateAccount validates and hashes the optional local credential.
func (s *Service) CreateAccount(ctx context.Context, username, password string) error {
	if !validUsername(username) || !validOptionalPassword(password) {
		return ErrInvalidUser
	}

	hash, err := hashOptionalPassword(password)
	if err != nil {
		return err
	}

	return s.accounts.CreateAccount(ctx, username, hash)
}

// UpdateAccount changes account status and optionally its local credential.
func (s *Service) UpdateAccount(ctx context.Context, actor, username string, enabled bool, password string) error {
	if !validUsername(username) || !validOptionalPassword(password) {
		return ErrInvalidUser
	}

	if actor == username && !enabled {
		return ErrProtectAccount
	}

	hash, err := hashOptionalPassword(password)
	if err != nil {
		return err
	}

	return s.accounts.UpdateAccount(ctx, actor, username, enabled, hash)
}

// hashOptionalPassword hashes a password when one is supplied.
func hashOptionalPassword(password string) (string, error) {
	if password == "" {
		return "", nil
	}

	return local.HashPassword(password)
}
