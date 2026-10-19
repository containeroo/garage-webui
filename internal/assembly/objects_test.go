package assembly

import (
	"testing"

	"github.com/containeroo/garage-webui/internal/http/auth"
)

// TestObjectProvisioningScopes rejects partial admin tokens even with cached keys.
func TestObjectProvisioningScopes(t *testing.T) {
	full := auth.Identity{Credential: auth.Credential{Kind: "admin"}, Scope: []string{"*"}}
	if !canProvisionObjects(full) {
		t.Fatal("full-scope admin denied")
	}
	limited := auth.Identity{Credential: auth.Credential{Kind: "admin"}, Scope: []string{
		"GetBucketInfo", "GetKeyInfo", "CreateKey", "AllowBucketKey", "DenyBucketKey",
	}}
	if canProvisionObjects(limited) {
		t.Fatal("token without UpdateBucket could create presigned URLs")
	}
	limited.Scope = append(limited.Scope, "UpdateBucket")
	if !canProvisionObjects(limited) {
		t.Fatal("sufficiently scoped admin denied")
	}
	limited.Credential.Kind = "s3"
	if canProvisionObjects(limited) {
		t.Fatal("S3 key was treated as an admin token")
	}
}
