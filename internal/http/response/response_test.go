package response

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestError(t *testing.T) {
	w := httptest.NewRecorder()
	Error(w, "invalid request", http.StatusBadRequest)
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Header().Get("Content-Type"), "application/json")
	assert.JSONEq(t, `{"error":"invalid request","problems":{}}`, w.Body.String())
}
