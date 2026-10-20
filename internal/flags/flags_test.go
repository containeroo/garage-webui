package flags

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCredentialConfiguration(t *testing.T) {
	t.Setenv("GARAGE_WEBUI__DATABASE_URL", "postgres://localhost/test")
	t.Setenv("GARAGE_WEBUI__GARAGE_URL", "http://localhost:3903")
	t.Setenv("GARAGE_WEBUI__SESSION_SECRET", "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=")
	cfg, err := Parse(nil, "test")
	require.NoError(t, err)
	assert.NotEmpty(t, cfg.SessionSecret)
	t.Run("--auth-mode=local", func(t *testing.T) {
		flag := "--auth-mode=local"
		_, err := Parse([]string{flag}, "test")
		assert.Error(t, err)
	})

	t.Run("--garage-token=shared", func(t *testing.T) {
		flag := "--garage-token=shared"
		_, err := Parse([]string{flag}, "test")
		assert.Error(t, err)
	})

	t.Run("--local-login", func(t *testing.T) {
		flag := "--local-login"
		_, err := Parse([]string{flag}, "test")
		assert.Error(t, err)
	})

	t.Run("--oidc-issuer=https://issuer.example", func(t *testing.T) {
		flag := "--oidc-issuer=https://issuer.example"
		_, err := Parse([]string{flag}, "test")
		assert.Error(t, err)
	})

	t.Run("--allow-user-registration", func(t *testing.T) {
		flag := "--allow-user-registration"
		_, err := Parse([]string{flag}, "test")
		assert.Error(t, err)
	})
}

func TestSQLiteConfiguration(t *testing.T) {
	t.Setenv("GARAGE_WEBUI__DATABASE_URL", "sqlite:///data/console.db")
	t.Setenv("GARAGE_WEBUI__GARAGE_URL", "http://localhost:3903")
	t.Setenv("GARAGE_WEBUI__SESSION_SECRET", "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=")
	cfg, err := Parse(nil, "test")
	require.NoError(t, err)
	assert.Equal(t, "sqlite:///data/console.db", cfg.Database.URL)
	assert.Zero(t, cfg.Database.MaxConns)
	assert.Zero(t, cfg.Database.MinIdleConns)
}
