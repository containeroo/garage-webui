package authentication

import (
	"context"
	"strings"

	kit "github.com/containeroo/appkit/auth"
)

// Accounts lists local accounts and their bindings.
func (s *Service) Accounts(ctx context.Context) ([]Account, error) {
	return s.accounts.Accounts(ctx)
}

// Role returns an enabled account's role.
func (s *Service) Role(ctx context.Context, username string) (string, error) {
	return s.accounts.Role(ctx, username)
}

// Registration reports whether external registration is enabled.
func (s *Service) Registration(ctx context.Context) (bool, error) {
	if s.registrationOverride != nil {
		return *s.registrationOverride, nil
	}
	return s.accounts.Registration(ctx)
}

// RegistrationManaged reports whether deployment configuration controls registration.
func (s *Service) RegistrationManaged() bool { return s.registrationOverride != nil }

// SetRegistration changes the external registration policy.
func (s *Service) SetRegistration(ctx context.Context, enabled bool) error {
	if s.RegistrationManaged() {
		return ErrRegistrationManaged
	}
	return s.accounts.SetRegistration(ctx, enabled)
}

// PendingIdentities lists OIDC identities awaiting review.
func (s *Service) PendingIdentities(ctx context.Context) ([]PendingIdentity, error) {
	return s.accounts.PendingIdentities(ctx)
}

// RevokeAccountSessions signs an account out of browser sessions.
func (s *Service) RevokeAccountSessions(ctx context.Context, username string) error {
	return s.accounts.RevokeAccountSessions(ctx, username)
}

// Profile returns an enabled user's profile.
func (s *Service) Profile(ctx context.Context, username string) (Profile, error) {
	return s.accounts.Profile(ctx, username)
}

// SetLocalCredentialEnabled changes whether a local password can authenticate.
func (s *Service) SetLocalCredentialEnabled(ctx context.Context, actor, username string, enabled bool) error {
	if actor == username && !enabled {
		return ErrProtectAccount
	}

	return s.accounts.SetLocalCredentialEnabled(ctx, actor, username, enabled)
}

// SetAccountProfile validates and updates an account profile and role.
func (s *Service) SetAccountProfile(ctx context.Context, actor, username, role, email, displayName string) error {
	if !validRole(role) || !validProfileFields(email, displayName) {
		return ErrInvalidUser
	}

	if actor == username && role != "admin" {
		return ErrProtectAccount
	}

	return s.accounts.SetAccountProfile(ctx, actor, username, role, strings.TrimSpace(email), strings.TrimSpace(displayName))
}

// validRole reports whether an account role is supported.
func validRole(role string) bool { return role == "admin" || role == "editor" || role == "viewer" }

// validProfileFields checks supported profile lengths.
func validProfileFields(email, displayName string) bool {
	return len(email) <= 320 && len(displayName) <= 128
}

// DeleteAccount removes an account and retires its linked identities.
func (s *Service) DeleteAccount(ctx context.Context, actor, username string) error {
	if actor == username {
		return ErrProtectAccount
	}

	return s.accounts.DeleteAccount(ctx, actor, username)
}

// LinkIdentity associates an external identity with an existing account.
func (s *Service) LinkIdentity(ctx context.Context, username string, id Identity) error {
	if !validUsername(username) || id.Provider != "proxy" || !validIdentity(id) {
		return ErrInvalidUser
	}

	return s.accounts.LinkIdentity(ctx, username, id)
}

// LinkPendingIdentity links a verified pending OIDC identity to an account.
func (s *Service) LinkPendingIdentity(ctx context.Context, username string, id Identity) error {
	if !validUsername(username) || !validOIDCIdentity(id) {
		return ErrInvalidUser
	}
	return s.accounts.LinkPendingIdentity(ctx, username, id)
}

// RejectPendingIdentity retires an unlinked OIDC identity.
func (s *Service) RejectPendingIdentity(ctx context.Context, id Identity) error {
	if !validOIDCIdentity(id) {
		return ErrInvalidUser
	}

	return s.accounts.RejectPendingIdentity(ctx, id)
}

// ReopenPendingIdentity allows review of a rejected OIDC identity.
func (s *Service) ReopenPendingIdentity(ctx context.Context, id Identity) error {
	if !validOIDCIdentity(id) {
		return ErrInvalidUser
	}

	return s.accounts.ReopenPendingIdentity(ctx, id)
}

// ApprovePendingIdentity creates an account for a verified OIDC identity.
func (s *Service) ApprovePendingIdentity(ctx context.Context, id Identity, username string) error {
	if !validOIDCIdentity(id) || !validUsername(username) {
		return ErrInvalidUser
	}

	return s.accounts.ApprovePendingIdentity(ctx, id, username)
}

// validOIDCIdentity checks a complete OIDC binding.
func validOIDCIdentity(id Identity) bool { return id.Provider == "oidc" && validIdentity(id) }

// UnlinkIdentity removes a binding and prevents automatic re-registration.
func (s *Service) UnlinkIdentity(ctx context.Context, username string, id Identity) error {
	if !validIdentity(id) {
		return ErrInvalidUser
	}

	return s.accounts.UnlinkIdentity(ctx, username, id)
}

// RelinkProxyIdentity changes a trusted-proxy account binding.
func (s *Service) RelinkProxyIdentity(ctx context.Context, username, oldSubject, newSubject string) error {
	if oldSubject == newSubject ||
		!validIdentity(Identity{
			Provider: "proxy",
			Subject:  oldSubject,
		}) ||
		!validIdentity(Identity{
			Provider: "proxy",
			Subject:  newSubject,
		}) {
		return ErrInvalidUser
	}

	return s.accounts.RelinkProxyIdentity(ctx, username, oldSubject, newSubject)
}

// ResolveExternal resolves a verified provider identity.
func (s *Service) ResolveExternal(ctx context.Context, provider, issuer, subject, username string) (string, error) {
	if !validIdentity(Identity{
		provider,
		issuer,
		subject,
	}) ||
		!validUsername(username) {
		return "", kit.ErrUnauthenticated
	}

	return s.accounts.ResolveExternal(ctx, provider, issuer, subject, username, s.registrationOverride)
}

// UpdateOwnProfile changes a user's profile fields.
func (s *Service) UpdateOwnProfile(ctx context.Context, username, email, displayName string) error {
	if !validProfileFields(email, displayName) {
		return ErrInvalidUser
	}

	return s.accounts.UpdateOwnProfile(ctx, username, strings.TrimSpace(email), strings.TrimSpace(displayName))
}

// RenameAccount changes a local username.
func (s *Service) RenameAccount(ctx context.Context, actor, username, newUsername string) error {
	if !validUsername(username) || !validUsername(newUsername) {
		return ErrInvalidUser
	}

	if username == newUsername {
		return nil
	}

	return s.accounts.RenameAccount(ctx, actor, username, newUsername)
}

// RefreshExternalProfile synchronizes provider fields.
func (s *Service) RefreshExternalProfile(
	ctx context.Context,
	user string,
	id Identity,
	username, email, displayName string,
) error {
	username = strings.TrimSpace(username)
	email = strings.TrimSpace(email)
	displayName = strings.TrimSpace(displayName)
	if len(username) > 128 {
		username = ""
	}
	if len(email) > 320 {
		email = ""
	}
	if len(displayName) > 128 {
		displayName = ""
	}

	return s.accounts.RefreshExternalProfile(ctx, user, id, username, email, displayName)
}

// RevertProfile restores selected provider-owned fields.
func (s *Service) RevertProfile(ctx context.Context, actor, username string, email, displayName bool) error {
	if !email && !displayName {
		return ErrInvalidUser
	}

	return s.accounts.RevertProfile(ctx, actor, username, email, displayName)
}
