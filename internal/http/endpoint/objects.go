package endpoint

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/containeroo/garage-webui/internal/application/objects"
	"github.com/containeroo/garage-webui/internal/http/middleware"
	httpresponse "github.com/containeroo/garage-webui/internal/http/response"
	"github.com/containeroo/garage-webui/internal/objectaction"
	"github.com/containeroo/garage-webui/internal/s3"
)

// objectRequest contains the JSON representation of an object operation.
type objectRequest struct {
	// BucketID identifies the target bucket.
	BucketID string `json:"bucketId"`
	// Action identifies the object operation to sign.
	Action objectaction.Action `json:"action"`
	// Key identifies the destination object.
	Key string `json:"key"`
	// Continuation holds the pagination token for the next object listing.
	Continuation string `json:"continuationToken"`
	// Prefix filters listed objects by key prefix.
	Prefix string `json:"prefix"`
	// SourceKey identifies the source object for a copy.
	SourceKey string `json:"sourceKey"`
	// UploadID identifies the active multipart upload.
	UploadID string `json:"uploadId"`
	// PartNumber identifies the one-based multipart part.
	PartNumber int `json:"partNumber"`
	// RangeStart sets the inclusive first byte of the source range.
	RangeStart int64 `json:"rangeStart"`
	// RangeEnd sets the inclusive last byte of the source range.
	RangeEnd int64 `json:"rangeEnd"`
}

// PresignObject authorizes listing, downloading or deleting without proxying data.
func PresignObject(objectServices ObjectServices, logger *slog.Logger) http.HandlerFunc {
	// presignObjectResponse describes the signed object request and required copy headers.
	type presignObjectResponse struct {
		// URL contains the signed request URL.
		URL string `json:"url"`
		// Method specifies the HTTP method for the signed request.
		Method string `json:"method"`
		// ExpiresIn specifies the signed request lifetime in seconds.
		ExpiresIn int `json:"expiresIn"`
		// CopySource contains the encoded copy source header value.
		CopySource string `json:"copySource"`
		// CopyHeaders contains the headers required by a multipart copy.
		CopyHeaders map[string]string `json:"copyHeaders"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		id, _ := middleware.RequestIdentity(r)
		kind := id.Credential.Kind

		service, err := objectServices.ForDirectIdentity(id)
		if errors.Is(err, objects.ErrForbidden) {
			httpresponse.Problem(w, http.StatusForbidden, "token does not allow object provisioning")
			return
		}

		if err != nil {
			httpresponse.Problem(w, http.StatusServiceUnavailable, "s3 operations unavailable")
			return
		}

		var input objectRequest
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8192))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&input); err != nil {
			httpresponse.Problem(w, http.StatusBadRequest, "invalid object request")
			return
		}

		result, err := service.Prepare(r.Context(), kind, objects.Request(input), time.Now())
		if errors.Is(err, objects.ErrInvalid) {
			httpresponse.Problem(w, http.StatusBadRequest, "invalid object request")
			return
		}

		if errors.Is(err, objects.ErrForbidden) {
			httpresponse.Problem(w, http.StatusForbidden, "object credentials required")
			return
		}

		if errors.Is(err, objects.ErrBucket) {
			httpresponse.Problem(w, http.StatusBadRequest, "bucket not found or missing a global S3 alias")
			return
		}

		if err != nil {
			logger.Error("sign object operation", "action", input.Action, "bucket", input.BucketID, "error", err)
			httpresponse.Problem(w, http.StatusBadGateway, "cannot prepare object operation")

			return
		}

		w.Header().Set("Cache-Control", "no-store")
		httpresponse.Respond(w, http.StatusOK, presignObjectResponse{
			URL:         result.URL,
			Method:      result.Method,
			ExpiresIn:   int(s3.URLLifetime.Seconds()),
			CopySource:  result.CopySource,
			CopyHeaders: result.CopyHeaders,
		})
	}
}
