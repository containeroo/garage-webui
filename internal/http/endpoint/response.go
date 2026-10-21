package endpoint

import (
	"encoding/json"
	"net/http"
)

// writeJSON writes an encoded response.
func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
