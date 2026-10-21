package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestSameOrigin checks strict origin comparison independent of authentication.
func TestSameOrigin(t *testing.T) {
	t.Parallel()

	t.Run("matching origin", func(t *testing.T) {
		t.Parallel()
		r := httptest.NewRequest(http.MethodPost, "/api/login", nil)
		r.Header.Set("Origin", "https://console.example")
		assert.True(t, sameOrigin(r, "https://console.example"))
	})

	t.Run("cross origin", func(t *testing.T) {
		t.Parallel()
		r := httptest.NewRequest(http.MethodPost, "/api/login", nil)
		r.Header.Set("Origin", "https://other.example")
		assert.False(t, sameOrigin(r, "https://console.example"))
	})

	t.Run("missing origin", func(t *testing.T) {
		t.Parallel()
		r := httptest.NewRequest(http.MethodPost, "/api/login", nil)
		assert.False(t, sameOrigin(r, "https://console.example"))
	})
}
