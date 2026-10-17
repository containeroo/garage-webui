package server

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/gi8lino/garage-webui/internal/http/auth"
	httpresponse "github.com/gi8lino/garage-webui/internal/http/response"
)

// validMutation rejects cross-origin and non-JSON writes.
func (s *Server) validMutation(w http.ResponseWriter, r *http.Request) bool {
	if !auth.SameOrigin(r, s.publicURL) || !strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
		httpresponse.Error(w, "invalid request origin or content type", http.StatusForbidden)
		return false
	}

	return true
}

// writeJSON writes an encoded response.
func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
