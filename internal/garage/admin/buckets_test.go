package admin

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBucketByID(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		status     int
		valid      bool
	}{
		{"valid", `{"id":"bucket","globalAliases":["name"]}`, http.StatusOK, true},
		{"wrong id", `{"id":"other","globalAliases":["name"]}`, http.StatusOK, false},
		{"no alias", `{"id":"bucket","globalAliases":[]}`, http.StatusOK, false},
		{"invalid JSON", `{`, http.StatusOK, false},
		{"not found", `{}`, http.StatusNotFound, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, "/v2/GetBucketInfo", r.URL.Path)
				assert.Equal(t, "bucket", r.URL.Query().Get("id"))
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer server.Close()
			client, err := New(server.URL, "token")
			require.NoError(t, err)
			bucket, err := client.BucketByID(context.Background(), "bucket")
			if !tc.valid {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, []string{"name"}, bucket.GlobalAliases)
		})
	}
}
