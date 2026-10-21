package auth

import (
	"bytes"
	"encoding/base64"
	"testing"

	"github.com/containeroo/garage-webui/internal/credential"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestJobCredentialIsolation verifies job secrets cannot be read without the correct key or job ID.
func TestJobCredentialIsolation(t *testing.T) {
	t.Parallel()
	config := Config{GarageURL: "http://garage.example:3903", SessionSecret: base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{42}, 32))}
	service, err := New(&sessionStore{entries: map[string]sessionRecord{}}, config)
	require.NoError(t, err)
	original := Credential{Kind: credential.S3, AccessKeyID: "GKtest", SecretAccessKey: "secret"}
	owner, err := service.JobOwner(original)
	require.NoError(t, err)
	different, err := service.JobOwner(Credential{Kind: credential.S3, AccessKeyID: "GKother", SecretAccessKey: "secret"})
	require.NoError(t, err)
	assert.NotEqual(t, owner, different)
	encrypted, err := service.EncryptJobCredential("first", original)
	require.NoError(t, err)
	assert.NotContains(t, string(encrypted), original.SecretAccessKey)
	decrypted, err := service.DecryptJobCredential("first", encrypted)
	require.NoError(t, err)
	assert.Equal(t, original, decrypted)
	_, err = service.DecryptJobCredential("second", encrypted)
	require.Error(t, err)
	encrypted[len(encrypted)-1] ^= 0xff
	_, err = service.DecryptJobCredential("first", encrypted)
	require.Error(t, err)
}
