package auth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestS3Endpoints(t *testing.T) {
	t.Parallel()
	for _, separate := range []bool{false, true} {
		name := "shared endpoint"
		if separate {
			name = "internal HTTP with HTTPS browser"
		}
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			calls := 0
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				assert.NotEmpty(t, r.URL.Query().Get("X-Amz-Signature"))
				_, _ = w.Write([]byte(`<ListAllMyBucketsResult><Buckets><Bucket><Name>filebrowser</Name></Bucket></Buckets></ListAllMyBucketsResult>`))
			}))
			defer upstream.Close()
			cfg := Config{
				PublicURL:     "http://console.example",
				GarageURL:     "http://garage.example:3903",
				S3PublicURL:   upstream.URL,
				S3URL:         upstream.URL,
				Region:        "garage",
				SessionSecret: "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=",
			}
			if separate {
				cfg.PublicURL = "https://console.example"
				cfg.S3URL = upstream.URL
				cfg.S3PublicURL = "https://browser-only.invalid"
			}
			service, err := New(&testStore{records: map[string]testRecord{}}, cfg)
			require.NoError(t, err)
			assert.Equal(t, upstream.URL, service.S3URL)
			assert.Equal(t, cfg.S3PublicURL, service.S3PublicURL)
			buckets, err := service.Buckets(context.Background(), Credential{Kind: "s3", AccessKeyID: "GKtest", SecretAccessKey: "secret"})
			require.NoError(t, err)
			assert.Equal(t, []string{"filebrowser"}, buckets)
			assert.Equal(t, 1, calls)
		})
	}
}
