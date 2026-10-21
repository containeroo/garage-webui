package assembly

import (
	"context"
	"github.com/containeroo/garage-webui/internal/application/objects"
	"github.com/containeroo/garage-webui/internal/objectaction"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

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
	require.True(t, canProvisionObjects(full, true), "full-scope admin denied")
	limited := auth.Identity{Credential: auth.Credential{Kind: credential.Admin}, Scope: []string{
		string(admin.OpGetBucketInfo), string(admin.OpGetKeyInfo), string(admin.OpCreateKey), string(admin.OpAllowBucketKey), string(admin.OpDenyBucketKey),
	}}
	assert.False(t, canProvisionObjects(limited, true), "token without UpdateBucket could create presigned URLs")
	assert.True(t, canProvisionObjects(limited, false), "backend transfer should not require CORS privileges")
	limited.Scope = append(limited.Scope, string(admin.OpUpdateBucket))
	require.True(t, canProvisionObjects(limited, true), "sufficiently scoped admin denied")
	limited.Credential.Kind = credential.S3
	assert.False(t, canProvisionObjects(limited, true), "S3 key was treated as an admin token")
}

func TestBackendObjectsNeedNoPublicEndpoint(t *testing.T) {
	t.Parallel()
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`<ListAllMyBucketsResult><Buckets><Bucket><Name>allowed</Name></Bucket></Buckets></ListAllMyBucketsResult>`))
	}))
	defer upstream.Close()
	authentication := &auth.Service{S3URL: upstream.URL, Region: "garage"}
	factory := New(authentication, nil, "https://console.example")
	identity := auth.Identity{Credential: auth.Credential{Kind: credential.S3, AccessKeyID: "GKuser", SecretAccessKey: "secret"}}
	service, err := factory.ForIdentity(identity)
	require.NoError(t, err)
	prepared, err := service.Prepare(context.Background(), credential.S3, objects.Request{BucketID: "allowed", Action: objectaction.List}, time.Now())
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(prepared.URL, upstream.URL+"/"))
	_, err = factory.ForDirectIdentity(identity)
	require.Error(t, err, "direct transfers must require a public S3 endpoint")
}
