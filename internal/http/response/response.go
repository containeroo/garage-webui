// Package response provides shared HTTP response formats.
package response

import (
	"net/http"

	httpkitresponse "github.com/containeroo/httpkit/response"
)

// Error writes an API error in the same format used by Kumbuka.
func Error(w http.ResponseWriter, message string, status int) {
	httpkitresponse.WriteJSON(w, status, map[string]any{
		"error":    message,
		"problems": map[string]string{},
	})
}
