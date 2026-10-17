package auth

import (
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestSameOrigin rejects requests without the configured browser origin.
func TestSameOrigin(t *testing.T) {
	for _, tc := range []struct {
		origin  string
		allowed bool
	}{
		{"https://garage.example", true}, {"http://garage.example", false},
		{"https://evil.example", false}, {"", false},
	} {
		r := httptest.NewRequest("POST", "https://garage.example/api/login", nil)
		r.Header.Set("Origin", tc.origin)
		assert.Equal(t, tc.allowed, SameOrigin(r, "https://garage.example"), tc.origin)
	}
}

// TestValidPublicOrigin checks the accepted public URL shape.
func TestValidPublicOrigin(t *testing.T) {
	for _, tc := range []struct {
		raw   string
		valid bool
	}{
		{"https://garage.example", true}, {"http://garage.example/", true},
		{"http://garage.example/path", false}, {"http://user@garage.example", false},
		{"http://garage.example?", false}, {"http://garage.example#fragment", false},
		{"ftp://garage.example", false}, {"/relative", false},
	} {
		u, err := url.Parse(tc.raw)
		assert.NoError(t, err)
		assert.Equal(t, tc.valid, validPublicOrigin(u), tc.raw)
	}

	assert.False(t, validPublicOrigin(nil))
}
