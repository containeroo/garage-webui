package authentication

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

// fakeGroups records calls across the group persistence boundary.
type fakeGroups struct{ called bool }

func (f *fakeGroups) GroupMappings(context.Context) ([]GroupMapping, error) {
	f.called = true
	return nil, nil
}
func (f *fakeGroups) SetGroupMapping(context.Context, GroupMapping, bool) error {
	f.called = true
	return nil
}
func (f *fakeGroups) SyncOIDCGroups(context.Context, string, []string) error {
	f.called = true
	return nil
}
func (f *fakeGroups) Groups(context.Context) ([]string, error)  { f.called = true; return nil, nil }
func (f *fakeGroups) CreateGroup(context.Context, string) error { f.called = true; return nil }
func (f *fakeGroups) DeleteGroup(context.Context, string) error { f.called = true; return nil }
func (f *fakeGroups) SetAccountGroups(context.Context, string, string, []string) error {
	f.called = true
	return nil
}

// TestGroupValidation verifies invalid inputs do not reach persistence.
func TestGroupValidation(t *testing.T) {
	repository := &fakeGroups{}
	service := &Service{groups: repository}
	ctx := context.Background()
	assert.ErrorIs(t, service.CreateGroup(ctx, " invalid "), ErrInvalidUser)
	assert.ErrorIs(t, service.SetGroupMapping(ctx, GroupMapping{
		ExternalGroup: "",
		Group:         "group",
	}, false), ErrInvalidUser)
	assert.ErrorIs(t, service.SetAccountGroups(ctx, "admin", "user", []string{"group", "group"}), ErrInvalidUser)
	assert.False(t, repository.called)
	assert.NoError(t, service.SetAccountGroups(ctx, "admin", "user", []string{"group"}))
	assert.True(t, repository.called)
}
