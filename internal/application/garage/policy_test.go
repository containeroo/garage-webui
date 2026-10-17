package garage

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestAccess verifies Garage authorization and the allowed operation list.
func TestAccess(t *testing.T) {
	for _, tc := range []struct {
		role, method string
		allowed      bool
	}{
		{"admin", "POST", true},
		{"editor", "POST", true},
		{"viewer", "GET", true},
		{"viewer", "POST", false},
		{"unknown", "GET", false},
	} {
		assert.Equal(t, tc.allowed, CanAccess(tc.role, tc.method))
	}

	assert.True(t, CanModify("admin"))
	assert.True(t, CanModify("editor"))
	assert.False(t, CanModify("viewer"))
	assert.False(t, CanModify("unknown"))
	method, ok := OperationMethod("ListBuckets")
	assert.True(t, ok)
	assert.Equal(t, "GET", method)
	_, ok = OperationMethod("Unlisted")
	assert.False(t, ok)
}
