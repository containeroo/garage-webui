package flags

import (
	"testing"
	"time"

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

func TestS3ServerEndpointConfiguration(t *testing.T) {
	t.Setenv("GARAGE_WEBUI__DATABASE_URL", "sqlite:///data/console.db")
	t.Setenv("GARAGE_WEBUI__GARAGE_URL", "http://garage:3903")
	t.Setenv("GARAGE_WEBUI__SESSION_SECRET", "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=")
	t.Setenv("GARAGE_WEBUI__S3_URL", "http://garage:3900")
	t.Setenv("GARAGE_WEBUI__S3_PUBLIC_URL", "https://s3.example.com")
	cfg, err := Parse(nil, "test")
	require.NoError(t, err)
	assert.Equal(t, "http://garage:3900", cfg.S3URL)
	assert.Equal(t, "https://s3.example.com", cfg.S3PublicURL)
	cfg, err = Parse([]string{"--s3-url=http://other:3900"}, "test")
	require.NoError(t, err)
	assert.Equal(t, "http://other:3900", cfg.S3URL)
}

func TestS3EndpointFallback(t *testing.T) {
	t.Setenv("GARAGE_WEBUI__DATABASE_URL", "sqlite:///data/console.db")
	t.Setenv("GARAGE_WEBUI__GARAGE_URL", "http://garage:3903")
	t.Setenv("GARAGE_WEBUI__SESSION_SECRET", "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=")
	t.Setenv("GARAGE_WEBUI__S3_URL", "")
	t.Setenv("GARAGE_WEBUI__S3_PUBLIC_URL", "https://s3.example.com")
	cfg, err := Parse(nil, "test")
	require.NoError(t, err)
	assert.Equal(t, cfg.S3PublicURL, cfg.S3URL)
	cfg, err = Parse([]string{"--s3-public-url=https://other.example.com"}, "test")
	require.NoError(t, err)
	assert.Equal(t, "https://other.example.com", cfg.S3URL)
	t.Setenv("GARAGE_WEBUI__S3_PUBLIC_URL", "")
	cfg, err = Parse(nil, "test")
	require.NoError(t, err)
	assert.Empty(t, cfg.S3URL)
}

func TestS3ResponseHeaderTimeout(t *testing.T) {
	t.Setenv("GARAGE_WEBUI__DATABASE_URL", "sqlite:///data/console.db")
	t.Setenv("GARAGE_WEBUI__GARAGE_URL", "http://garage:3903")
	t.Setenv("GARAGE_WEBUI__SESSION_SECRET", "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=")
	cfg, err := Parse(nil, "test")
	require.NoError(t, err)
	assert.Equal(t, DefaultS3ResponseHeaderTimeout, cfg.S3ResponseHeaderTimeout)
	t.Setenv("GARAGE_WEBUI__S3_RESPONSE_HEADER_TIMEOUT", "2m")
	cfg, err = Parse(nil, "test")
	require.NoError(t, err)
	assert.Equal(t, 2*time.Minute, cfg.S3ResponseHeaderTimeout)
	cfg, err = Parse([]string{"--s3-response-header-timeout=0"}, "test")
	require.NoError(t, err)
	assert.Zero(t, cfg.S3ResponseHeaderTimeout)
	_, err = Parse([]string{"--s3-response-header-timeout=-1s"}, "test")
	require.Error(t, err)
}
