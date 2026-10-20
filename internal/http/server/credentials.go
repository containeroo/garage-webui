package server

import (
	"net/http"
	"slices"

	"github.com/containeroo/garage-webui/internal/credential"
	"github.com/containeroo/garage-webui/internal/garage/admin"
	"github.com/containeroo/garage-webui/internal/http/auth"
	"github.com/containeroo/garage-webui/internal/http/middleware"
	httpresponse "github.com/containeroo/garage-webui/internal/http/response"
)

// S3BucketRequest exposes only the buckets listed for the authenticated S3 key.
func S3BucketRequest(authentication *auth.Service) http.HandlerFunc {
	// bucketSummaryResponse describes a bucket visible to an S3 session.
	type bucketSummaryResponse struct {
		// ID identifies the bucket.
		ID string `json:"id"`
		// GlobalAliases lists globally usable bucket names.
		GlobalAliases []string `json:"globalAliases"`
	}
	// bucketInfoResponse describes bucket metadata without exposing access key details.
	type bucketInfoResponse struct {
		// ID identifies the bucket.
		ID string `json:"id"`
		// GlobalAliases lists globally usable bucket names.
		GlobalAliases []string `json:"globalAliases"`
		// Keys is an empty list that avoids exposing access key details.
		Keys []any `json:"keys"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := middleware.RequestIdentity(r)
		if !ok || id.Credential.Kind != credential.S3 {
			httpresponse.Error(w, "s3 credentials required", http.StatusForbidden)
			return
		}
		op := admin.Operation(r.PathValue("operation"))
		if r.Method != http.MethodGet || (op != admin.OpListBuckets && op != admin.OpGetBucketInfo) {
			httpresponse.Error(w, "s3 credentials cannot access administration", http.StatusForbidden)
			return
		}

		names, err := authentication.Buckets(r.Context(), id.Credential)
		if err != nil {
			httpresponse.Error(w, "cannot list accessible buckets", http.StatusBadGateway)
			return
		}

		if op == admin.OpGetBucketInfo {
			name := r.URL.Query().Get("id")
			if !slices.Contains(names, name) {
				httpresponse.Error(w, "bucket not accessible", http.StatusForbidden)
				return
			}

			writeJSON(w, http.StatusOK, bucketInfoResponse{
				ID:            name,
				GlobalAliases: []string{name},
				Keys:          []any{},
			})

			return
		}

		result := []bucketSummaryResponse{}
		for _, name := range names {
			result = append(result, bucketSummaryResponse{
				ID:            name,
				GlobalAliases: []string{name},
			})
		}

		writeJSON(w, http.StatusOK, result)
	}
}
