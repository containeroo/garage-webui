package authentication

import (
	"context"
	"strings"

	"github.com/gi8lino/garage-webui/internal/accounts"
)

// GroupMapping connects an OIDC group claim to a local group.
type GroupMapping = accounts.GroupMapping

// groupRepository persists groups and their memberships.
type groupRepository interface {
	GroupMappings(context.Context) ([]GroupMapping, error)
	SetGroupMapping(context.Context, GroupMapping, bool) error
	SyncOIDCGroups(context.Context, string, []string) error
	Groups(context.Context) ([]string, error)
	CreateGroup(context.Context, string) error
	DeleteGroup(context.Context, string) error
	SetAccountGroups(context.Context, string, string, []string) error
}

// GroupMappings lists configured OIDC group mappings.
func (s *Service) GroupMappings(ctx context.Context) ([]GroupMapping, error) {
	return s.groups.GroupMappings(ctx)
}

// SetGroupMapping creates or removes an OIDC group mapping.
func (s *Service) SetGroupMapping(ctx context.Context, mapping GroupMapping, remove bool) error {
	if !validGroup(mapping.ExternalGroup) || !validGroup(mapping.Group) {
		return ErrInvalidUser
	}

	return s.groups.SetGroupMapping(ctx, mapping, remove)
}

// SyncOIDCGroups replaces provider-derived memberships after a verified login.
func (s *Service) SyncOIDCGroups(ctx context.Context, username string, groups []string) error {
	return s.groups.SyncOIDCGroups(ctx, username, groups)
}

// Groups lists local group names.
func (s *Service) Groups(ctx context.Context) ([]string, error) {
	return s.groups.Groups(ctx)
}

// CreateGroup creates a local group.
func (s *Service) CreateGroup(ctx context.Context, name string) error {
	if !validGroup(name) {
		return ErrInvalidUser
	}

	return s.groups.CreateGroup(ctx, name)
}

// DeleteGroup removes a group and its memberships.
func (s *Service) DeleteGroup(ctx context.Context, name string) error {
	return s.groups.DeleteGroup(ctx, name)
}

// SetAccountGroups atomically replaces one account's local memberships.
func (s *Service) SetAccountGroups(ctx context.Context, actor, username string, groups []string) error {
	if !validGroupList(groups) {
		return ErrInvalidUser
	}

	return s.groups.SetAccountGroups(ctx, actor, username, groups)
}

// validGroupList rejects invalid and duplicate group names.
func validGroupList(groups []string) bool {
	seen := make(map[string]bool, len(groups))
	for _, group := range groups {
		if !validGroup(group) || seen[group] {
			return false
		}

		seen[group] = true
	}

	return true
}

// validGroup validates a group name.
func validGroup(name string) bool {
	return name != "" &&
		name == strings.TrimSpace(name) &&
		len(name) <= 128 &&
		!strings.ContainsRune(name, '\x00')
}
