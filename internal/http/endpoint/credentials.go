package endpoint

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
		if !ok {
			httpresponse.Problem(w, http.StatusUnauthorized, "authentication required")
			return
		}

		if id.Credential.Kind != credential.S3 {
			httpresponse.Problem(w, http.StatusForbidden, "s3 credentials required")
			return
		}

		op, ok := admin.ParseOperation(r.PathValue("operation"))
		if !ok {
			httpresponse.Problem(w, http.StatusNotFound, "operation not found")
			return
		}

		if op != admin.OpListBuckets && op != admin.OpGetBucketInfo {
			httpresponse.Problem(w, http.StatusForbidden, "operation is not available with s3 credentials")
			return
		}

		if r.Method != http.MethodGet {
			w.Header().Set("Allow", http.MethodGet)
			httpresponse.Problem(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}

		name := ""
		if op == admin.OpGetBucketInfo {
			name = r.URL.Query().Get("id")
			if name == "" {
				httpresponse.Problem(w, http.StatusBadRequest, "bucket id is required")
				return
			}
		}

		names, err := authentication.Buckets(r.Context(), id.Credential)
		if err != nil {
			httpresponse.Problem(w, http.StatusBadGateway, "cannot list accessible buckets")
			return
		}

		if op == admin.OpGetBucketInfo {
			if !slices.Contains(names, name) {
				httpresponse.Problem(w, http.StatusForbidden, "bucket is not accessible")
				return
			}

			httpresponse.Respond(w, http.StatusOK, bucketInfoResponse{
				ID:            name,
				GlobalAliases: []string{name},
				Keys:          []any{},
			})
			return
		}

		result := make([]bucketSummaryResponse, 0, len(names))
		for _, name := range names {
			result = append(result, bucketSummaryResponse{
				ID:            name,
				GlobalAliases: []string{name},
			})
		}

		httpresponse.Respond(w, http.StatusOK, result)
	}
}
