package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/containeroo/garage-webui/internal/application/objects"
	"github.com/containeroo/garage-webui/internal/http/middleware"
	httpresponse "github.com/containeroo/garage-webui/internal/http/response"
	"github.com/containeroo/garage-webui/internal/s3"
)

// objectRequest contains the JSON representation of an object operation.
type objectRequest struct {
	BucketID     string `json:"bucketId"`
	Action       string `json:"action"`
	Key          string `json:"key"`
	Continuation string `json:"continuationToken"`
	Prefix       string `json:"prefix"`
	SourceKey    string `json:"sourceKey"`
	UploadID     string `json:"uploadId"`
	PartNumber   int    `json:"partNumber"`
	RangeStart   int64  `json:"rangeStart"`
	RangeEnd     int64  `json:"rangeEnd"`
}

// presignObject authorizes listing, downloading or deleting without proxying data.
func (s *Server) presignObject(w http.ResponseWriter, r *http.Request) {
	id, _ := middleware.RequestIdentity(r)
	kind := id.Credential.Kind

	service, err := s.objectService(id)
	if errors.Is(err, objects.ErrForbidden) {
		httpresponse.Error(w, "token does not allow object provisioning", http.StatusForbidden)
		return
	}

	if err != nil {
		httpresponse.Error(w, "s3 operations unavailable", http.StatusServiceUnavailable)
		return
	}

	var input objectRequest
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8192))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		httpresponse.Error(w, "invalid object request", http.StatusBadRequest)
		return
	}

	result, err := service.Prepare(r.Context(), kind, objects.Request(input), time.Now())
	if errors.Is(err, objects.ErrInvalid) {
		httpresponse.Error(w, "invalid object request", http.StatusBadRequest)
		return
	}

	if errors.Is(err, objects.ErrForbidden) {
		httpresponse.Error(w, "object credentials required", http.StatusForbidden)
		return
	}

	if errors.Is(err, objects.ErrBucket) {
		httpresponse.Error(w, "bucket not found or missing a global S3 alias", http.StatusBadRequest)
		return
	}

	if err != nil {
		if s.logger != nil {
			s.logger.Error("sign object operation", "action", input.Action, "bucket", input.BucketID, "error", err)
		}

		httpresponse.Error(w, "cannot prepare object operation", http.StatusBadGateway)

		return
	}

	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, map[string]any{
		"url":         result.URL,
		"method":      result.Method,
		"expiresIn":   int(s3.URLLifetime.Seconds()),
		"copySource":  result.CopySource,
		"copyHeaders": result.CopyHeaders,
	})
}
