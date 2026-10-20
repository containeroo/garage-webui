package server

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/containeroo/garage-webui/internal/application/objects"
	"github.com/containeroo/garage-webui/internal/http/middleware"
	httpresponse "github.com/containeroo/garage-webui/internal/http/response"
	"github.com/containeroo/garage-webui/internal/s3"
)

// PresignUpload authorizes Garage credentials and issues a short-lived, object-scoped URL.
// Only metadata passes through the backend; file bytes go directly to the S3 API.
func PresignUpload(objectServices ObjectServices, logger *slog.Logger) http.HandlerFunc {
	// presignUploadResponse describes a signed object upload request.
	type presignUploadResponse struct {
		// URL contains the signed request URL.
		URL string `json:"url"`
		// Method specifies the HTTP method for the signed request.
		Method string `json:"method"`
		// ExpiresIn specifies the signed request lifetime in seconds.
		ExpiresIn int `json:"expiresIn"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		id, _ := middleware.RequestIdentity(r)
		kind := id.Credential.Kind

		service, err := objectServices.ForIdentity(id)
		if errors.Is(err, objects.ErrForbidden) {
			httpresponse.Error(w, "token does not allow object provisioning", http.StatusForbidden)
			return
		}

		if err != nil {
			httpresponse.Error(w, "s3 operations unavailable", http.StatusServiceUnavailable)
			return
		}

		// input identifies the upload target and declared object size.
		var input struct {
			// BucketID identifies the target bucket.
			BucketID string `json:"bucketId"`
			// Key identifies the destination object.
			Key string `json:"key"`
			// Size specifies the object size in bytes.
			Size int64 `json:"size"`
		}
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&input); err != nil {
			httpresponse.Error(w, "invalid bucket, object key or file size (1 byte to 5 GiB)", http.StatusBadRequest)
			return
		}

		result, err := service.PrepareUpload(r.Context(), kind, input.BucketID, input.Key, input.Size, time.Now())
		if errors.Is(err, objects.ErrForbidden) {
			httpresponse.Error(w, "upload permissions required", http.StatusForbidden)
			return
		}

		if errors.Is(err, objects.ErrInvalid) {
			httpresponse.Error(w, "invalid bucket, object key or file size (1 byte to 5 GiB)", http.StatusBadRequest)
			return
		}

		if errors.Is(err, objects.ErrBucket) {
			httpresponse.Error(w, "bucket not found or missing a global S3 alias", http.StatusBadRequest)
			return
		}

		if err != nil {
			logger.Error("prepare direct Garage upload", "error", err, "bucket", input.BucketID)
			httpresponse.Error(w, "cannot prepare Garage upload", http.StatusBadGateway)

			return
		}

		w.Header().Set("Cache-Control", "no-store")
		writeJSON(w, http.StatusOK, presignUploadResponse{
			URL:       result.URL,
			Method:    result.Method,
			ExpiresIn: int(s3.URLLifetime.Seconds()),
		})
	}
}
