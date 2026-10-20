package server

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"

	"github.com/containeroo/garage-webui/internal/application/objects"
	"github.com/containeroo/garage-webui/internal/garage/admin"
	"github.com/containeroo/garage-webui/internal/http/auth"
	"github.com/containeroo/garage-webui/internal/http/middleware"
	httpresponse "github.com/containeroo/garage-webui/internal/http/response"
)

// GarageRequest forwards only approved operations for authenticated administrators.
func GarageRequest(authentication *auth.Service, objectServices ObjectServices, logger *slog.Logger) http.HandlerFunc {
	s3Buckets := S3BucketRequest(authentication)
	return func(w http.ResponseWriter, r *http.Request) {
		id, _ := middleware.RequestIdentity(r)
		op := r.PathValue("operation")
		if id.Credential.Kind == "s3" {
			s3Buckets.ServeHTTP(w, r)
			return
		}

		operation, exists := admin.ParseOperation(op)
		if !exists {
			httpresponse.Error(w, "not found", http.StatusNotFound)
			return
		}

		method, _ := operation.Method()
		if method != r.Method {
			httpresponse.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		if !id.Allows(op) {
			httpresponse.Error(w, "token does not allow this operation", http.StatusForbidden)
			return
		}

		query := r.URL.Query()
		var body []byte
		if r.Method == http.MethodPost {
			data, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 64<<10))
			if err != nil {
				httpresponse.Error(w, "request too large", http.StatusBadRequest)
				return
			}

			if len(data) > 0 && !json.Valid(data) {
				httpresponse.Error(w, "invalid JSON", http.StatusBadRequest)
				return
			}

			if len(data) > 0 {
				body = data
			}
		}

		if op == "DeleteBucket" {
			if !canCleanupBucket(id) {
				httpresponse.Error(w, "deleting a bucket also requires DeleteKey and GetBucketInfo scopes", http.StatusForbidden)
				return
			}

			status, result, err := objectServices.DeleteBucket(r.Context(), id, query.Get("id"))
			if err != nil {
				if errors.Is(err, objects.ErrForbidden) {
					httpresponse.Error(w, "insufficient Garage token scope", http.StatusForbidden)
					return
				}

				logger.Error("delete Garage bucket", "error", err)
				if status < 400 || status > 499 {
					status = http.StatusBadGateway
				}

				httpresponse.Error(w, "could not delete Garage bucket or managed keys", status)

				return
			}

			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(status)
			_, _ = w.Write(result)

			return
		}

		client, err := authentication.AdminClient(id)
		if err != nil {
			httpresponse.Error(w, "invalid Garage endpoint", http.StatusBadGateway)
			return
		}

		query.Del("showSecretKey")
		status, data, err := client.Proxy(r.Context(), operation, query, body)
		if err != nil {
			logger.Error("garage request failed", "operation", op, "error", err)

			httpresponse.Error(w, "garage is unavailable", http.StatusBadGateway)

			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write(data)
	}
}

// canCleanupBucket checks the additional Garage scopes required for key cleanup.
func canCleanupBucket(id auth.Identity) bool {
	return id.Allows("DeleteKey") && id.Allows("GetBucketInfo")
}
