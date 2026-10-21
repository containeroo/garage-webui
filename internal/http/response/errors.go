package response

import (
	"log/slog"
	"net/http"

	httpkitresponse "github.com/containeroo/httpkit/response"
)

// InternalServerError logs a private failure and writes a safe JSON error with a reference ID.
func InternalServerError(logger *slog.Logger, w http.ResponseWriter, err error) {
	reference := httpkitresponse.LogFailure(logger, w, err)
	if request, ok := w.(*httpkitresponse.RequestWriter); ok && request.Status != 0 {
		return
	}

	Problem(w, http.StatusInternalServerError, "the request could not be processed; reference: "+reference)
}
