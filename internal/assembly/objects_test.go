package assembly

import (
	"testing"

	"github.com/containeroo/garage-webui/internal/credential"
	"github.com/containeroo/garage-webui/internal/garage/admin"
	"github.com/containeroo/garage-webui/internal/http/auth"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestObjectProvisioningScopes rejects partial admin tokens even with cached keys.
func TestObjectProvisioningScopes(t *testing.T) {
	t.Parallel()

	full := auth.Identity{Credential: auth.Credential{Kind: credential.Admin}, Scope: []string{"*"}}
	require.True(t, canProvisionObjects(full), "full-scope admin denied")
	limited := auth.Identity{Credential: auth.Credential{Kind: credential.Admin}, Scope: []string{
		string(admin.OpGetBucketInfo), string(admin.OpGetKeyInfo), string(admin.OpCreateKey), string(admin.OpAllowBucketKey), string(admin.OpDenyBucketKey),
	}}
	assert.False(t, canProvisionObjects(limited), "token without UpdateBucket could create presigned URLs")
	limited.Scope = append(limited.Scope, string(admin.OpUpdateBucket))
	require.True(t, canProvisionObjects(limited), "sufficiently scoped admin denied")
	limited.Credential.Kind = credential.S3
	assert.False(t, canProvisionObjects(limited), "S3 key was treated as an admin token")
}
