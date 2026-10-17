package flags

import (
	"testing"

	"github.com/containeroo/tinyflags"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestParseEnvironment checks AppKit's environment variable convention.
func TestParseEnvironment(t *testing.T) {
	t.Setenv("GARAGE_WEBUI__DATABASE_URL", "postgres://localhost/test")
	t.Setenv("GARAGE_WEBUI__GARAGE_URL", "http://localhost:3903")
	t.Setenv("GARAGE_WEBUI__GARAGE_TOKEN", "token")
	t.Setenv("GARAGE_WEBUI__AUTH_MODE", "proxy")
	cfg, err := Parse(nil, "test")
	require.NoError(t, err)
	assert.Equal(t, "http://localhost:3903", cfg.GarageURL)
	require.NotNil(t, cfg.Authentication.ModeOverride)
	assert.Equal(t, "proxy", string(*cfg.Authentication.ModeOverride))
}

// TestAuthenticationFlags checks explicit overrides and the removed encryption flag.
func TestAuthenticationFlags(t *testing.T) {
	t.Setenv("GARAGE_WEBUI__DATABASE_URL", "postgres://localhost/test")
	t.Setenv("GARAGE_WEBUI__GARAGE_URL", "http://localhost:3903")
	t.Setenv("GARAGE_WEBUI__GARAGE_TOKEN", "token")
	t.Setenv("GARAGE_WEBUI__ENCRYPTION_KEY", "not-a-valid-key")

	cfg, err := Parse([]string{"--auth-mode=oidc", "--allow-user-registration=false", "--local-login"}, "test")
	require.NoError(t, err)
	require.NotNil(t, cfg.Authentication.ModeOverride)
	assert.Equal(t, "oidc", string(*cfg.Authentication.ModeOverride))
	require.NotNil(t, cfg.Authentication.AllowRegistrationOverride)
	assert.False(t, *cfg.Authentication.AllowRegistrationOverride)
	assert.True(t, cfg.Authentication.LocalLogin)
	assert.Empty(t, cfg.Authentication.EncryptionKey)

	_, err = Parse([]string{"--encryption-key=unused"}, "test")
	require.Error(t, err)

	_, err = Parse([]string{"--help"}, "test")
	require.True(t, tinyflags.IsHelpRequested(err))
	help := err.Error()
	assert.NotContains(t, help, "--encryption-key")
	assert.Contains(t, help, "--auth-mode")
	assert.Contains(t, help, "--allow-user-registration")
	assert.Contains(t, help, "--local-login")
}

// TestAuthenticationDefaults preserves absent and explicit environment overrides.
func TestAuthenticationDefaults(t *testing.T) {
	t.Setenv("GARAGE_WEBUI__DATABASE_URL", "postgres://localhost/test")
	t.Setenv("GARAGE_WEBUI__GARAGE_URL", "http://localhost:3903")
	t.Setenv("GARAGE_WEBUI__GARAGE_TOKEN", "token")

	cfg, err := Parse(nil, "test")
	require.NoError(t, err)
	assert.Nil(t, cfg.Authentication.ModeOverride)
	assert.Nil(t, cfg.Authentication.AllowRegistrationOverride)
	assert.False(t, cfg.Authentication.LocalLogin)

	t.Setenv("GARAGE_WEBUI__ALLOW_USER_REGISTRATION", "true")
	t.Setenv("GARAGE_WEBUI__LOCAL_LOGIN", "true")
	cfg, err = Parse(nil, "test")
	require.NoError(t, err)
	require.NotNil(t, cfg.Authentication.AllowRegistrationOverride)
	assert.True(t, *cfg.Authentication.AllowRegistrationOverride)
	assert.True(t, cfg.Authentication.LocalLogin)
}
