package server

import (
	"net/http"
	"slices"

	"github.com/containeroo/garage-webui/internal/http/auth"
	"github.com/containeroo/garage-webui/internal/http/middleware"
	httpresponse "github.com/containeroo/garage-webui/internal/http/response"
)

// S3BucketRequest exposes only the buckets listed for the authenticated S3 key.
func S3BucketRequest(authentication *auth.Service) http.HandlerFunc {
	type bucketSummaryResponse struct {
		ID            string   `json:"id"`
		GlobalAliases []string `json:"globalAliases"`
	}
	type bucketInfoResponse struct {
		ID            string   `json:"id"`
		GlobalAliases []string `json:"globalAliases"`
		Keys          []any    `json:"keys"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := middleware.RequestIdentity(r)
		if !ok || id.Credential.Kind != "s3" {
			httpresponse.Error(w, "s3 credentials required", http.StatusForbidden)
			return
		}
		op := r.PathValue("operation")
		if r.Method != "GET" || (op != "ListBuckets" && op != "GetBucketInfo") {
			httpresponse.Error(w, "s3 credentials cannot access administration", http.StatusForbidden)
			return
		}

		names, err := authentication.Buckets(r.Context(), id.Credential)
		if err != nil {
			httpresponse.Error(w, "cannot list accessible buckets", http.StatusBadGateway)
			return
		}

		if op == "GetBucketInfo" {
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
