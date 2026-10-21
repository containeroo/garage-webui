package endpoint

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"

	"github.com/containeroo/garage-webui/internal/application/objects"
	"github.com/containeroo/garage-webui/internal/credential"
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
		if id.Credential.Kind == credential.S3 {
			s3Buckets.ServeHTTP(w, r)
			return
		}

		operation, exists := admin.ParseOperation(op)
		if !exists {
			httpresponse.Problem(w, http.StatusNotFound, "not found")
			return
		}

		method, _ := operation.Method()
		if method != r.Method {
			httpresponse.Problem(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}

		if !id.Allows(operation) {
			httpresponse.Problem(w, http.StatusForbidden, "token does not allow this operation")
			return
		}

		query := r.URL.Query()
		var body []byte
		if r.Method == http.MethodPost {
			data, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 64<<10))
			if err != nil {
				httpresponse.Problem(w, http.StatusBadRequest, "request too large")
				return
			}

			if len(data) > 0 && !json.Valid(data) {
				httpresponse.Problem(w, http.StatusBadRequest, "invalid JSON")
				return
			}

			if len(data) > 0 {
				body = data
			}
		}

		if op == "DeleteBucket" {
			status, result, err := objectServices.DeleteBucket(r.Context(), id, query.Get("id"))
			if err != nil {
				if errors.Is(err, objects.ErrForbidden) {
					httpresponse.Problem(w, http.StatusForbidden, "insufficient Garage token scope")
					return
				}

				logger.Error("delete Garage bucket", "error", err)
				if status < 400 || status > 499 {
					status = http.StatusBadGateway
				}

				httpresponse.Problem(w, status, "could not delete Garage bucket or managed keys")

				return
			}

			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(status)
			_, _ = w.Write(result)

			return
		}

		client, err := authentication.AdminClient(id)
		if err != nil {
			httpresponse.Problem(w, http.StatusBadGateway, "invalid Garage endpoint")
			return
		}

		query.Del("showSecretKey")
		status, data, err := client.Proxy(r.Context(), operation, query, body)
		if err != nil {
			logger.Error("garage request failed", "operation", op, "error", err)

			httpresponse.Problem(w, http.StatusBadGateway, "garage is unavailable")

			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write(data)
	}
}
