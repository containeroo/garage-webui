package credentials

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRuleAllowsMethod(t *testing.T) {
	t.Parallel()

	rule := corsRule{
		AllowedOrigin: []string{"http://localhost:8080"},
		AllowedMethod: []string{"PUT", "DELETE"},
		AllowedHeader: []string{"*"},
	}
	t.Run("allow PUT", func(t *testing.T) {
		t.Parallel()

		assert.True(t, ruleAllowsMethod(rule, "http://localhost:8080", http.MethodPut))
	})
	t.Run("allow DELETE", func(t *testing.T) {
		t.Parallel()

		assert.True(t, ruleAllowsMethod(rule, "http://localhost:8080", http.MethodDelete))
	})
	t.Run("reject other origin", func(t *testing.T) {
		t.Parallel()

		assert.False(t, ruleAllowsMethod(rule, "https://other.example", http.MethodPut))
	})
	t.Run("require allowed headers", func(t *testing.T) {
		t.Parallel()

		withoutHeaders := rule
		withoutHeaders.AllowedHeader = nil
		assert.False(t, ruleAllowsMethod(withoutHeaders, "http://localhost:8080", http.MethodPut))
	})
}

// TestAllOperationCORS rejects incomplete rules that break multipart or deletion.
func TestAllOperationCORS(t *testing.T) {
	t.Parallel()

	rule := corsRule{
		AllowedOrigin: []string{"http://localhost:8080"},
		AllowedMethod: []string{"GET", "PUT", "POST", "DELETE"},
		AllowedHeader: []string{"*"},
		ExposeHeader:  []string{"ETag"},
	}
	t.Run("complete rule", func(t *testing.T) {
		t.Parallel()

		assert.True(t, ruleAllowsAllObjectMethods(rule, "http://localhost:8080"))
	})
	t.Run("reject missing methods", func(t *testing.T) {
		t.Parallel()

		incomplete := rule
		incomplete.AllowedMethod = []string{"PUT"}
		assert.False(t, ruleAllowsAllObjectMethods(incomplete, "http://localhost:8080"))
	})
	t.Run("require ETag exposure", func(t *testing.T) {
		t.Parallel()

		withoutETag := rule
		withoutETag.ExposeHeader = nil
		assert.False(t, ruleAllowsAllObjectMethods(withoutETag, "http://localhost:8080"))
	})
}
