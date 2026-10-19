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
	for _, flag := range []string{"--auth-mode=local", "--garage-token=shared", "--local-login", "--oidc-issuer=https://issuer.example", "--allow-user-registration"} {
		_, err := Parse([]string{flag}, "test")
		assert.Error(t, err)
	}
}
