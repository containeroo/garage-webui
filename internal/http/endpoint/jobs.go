package endpoint

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/containeroo/garage-webui/internal/application/jobs"
	"github.com/containeroo/garage-webui/internal/http/middleware"
	httpresponse "github.com/containeroo/garage-webui/internal/http/response"
)

// CreateDeletionJob accepts a confirmed selection and schedules durable backend work.
func CreateDeletionJob(service *jobs.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, _ := middleware.RequestIdentity(r)
		var input struct {
			// Bucket identifies the deletion target.
			Bucket string `json:"bucket"`
			jobs.Selection
		}
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 32<<10))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&input); err != nil {
			httpresponse.Problem(w, http.StatusBadRequest, "invalid deletion request")
			return
		}
		var extra any
		if err := decoder.Decode(&extra); err != io.EOF {
			httpresponse.Problem(w, http.StatusBadRequest, "unexpected deletion request data")
			return
		}
		job, err := service.Start(r.Context(), id, input.Bucket, input.Selection)
		if err != nil {
			httpresponse.Problem(w, http.StatusBadRequest, "cannot schedule deletion: "+safeDeletionStartError(err))
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		httpresponse.Respond(w, http.StatusAccepted, job)
	}
}

// safeDeletionStartError avoids exposing credentials or signed upstream URLs in API errors.
func safeDeletionStartError(err error) string {
	switch err.Error() {
	case "invalid bucket", "invalid folder prefix", "invalid object key", "select between 1 and 200 files or folders":
		return err.Error()
	}
	return "Garage permission or object service unavailable"
}

// ListDeletionJobs returns the latest persisted jobs visible to the signed-in Garage credential.
func ListDeletionJobs(service *jobs.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, _ := middleware.RequestIdentity(r)
		result, err := service.List(r.Context(), id)
		if err != nil {
			httpresponse.Problem(w, http.StatusServiceUnavailable, "cannot load background jobs")
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		httpresponse.Respond(w, http.StatusOK, result)
	}
}

// CancelDeletionJob requests cancellation and never allows cancelling another credential's work.
func CancelDeletionJob(service *jobs.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, _ := middleware.RequestIdentity(r)
		jobID := r.PathValue("id")
		if len(jobID) != 32 || strings.Trim(jobID, "0123456789abcdef") != "" {
			httpresponse.Problem(w, http.StatusNotFound, "job not found")
			return
		}
		found, err := service.Cancel(r.Context(), id, jobID)
		if err != nil {
			httpresponse.Problem(w, http.StatusServiceUnavailable, "cannot cancel deletion job")
			return
		}
		if !found {
			httpresponse.Problem(w, http.StatusNotFound, "running job not found")
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}
