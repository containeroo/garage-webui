package server

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/url"

	"github.com/gi8lino/garage-webui/internal/http/middleware"
	httpresponse "github.com/gi8lino/garage-webui/internal/http/response"
)

// garageRequest forwards only approved operations for authenticated administrators.
func (s *Server) garageRequest(w http.ResponseWriter, r *http.Request) {
	id, _ := middleware.RequestIdentity(r)
	role := id.Role
	op := r.PathValue("operation")
	query := r.URL.Query()
	var body io.Reader
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
			body = bytes.NewReader(data)
		}
	}

	status, data, err := s.garage.Request(r.Context(), role, r.Method, op, url.Values(query), body)
	if err != nil {
		s.logger.Error("Garage request failed", "operation", op, "error", err)
		httpresponse.Error(w, "Garage is unavailable", http.StatusBadGateway)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write(data)
}
