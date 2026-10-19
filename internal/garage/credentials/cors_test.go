package credentials

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRuleAllowsMethod(t *testing.T) {
	rule := corsRule{
		AllowedOrigin: []string{"http://localhost:8080"},
		AllowedMethod: []string{"PUT", "DELETE"},
		AllowedHeader: []string{"*"},
	}
	assert.True(t, ruleAllowsMethod(rule, "http://localhost:8080", http.MethodPut))
	assert.True(t, ruleAllowsMethod(rule, "http://localhost:8080", http.MethodDelete))
	assert.False(t, ruleAllowsMethod(rule, "https://other.example", http.MethodPut))
	rule.AllowedHeader = nil
	assert.False(t, ruleAllowsMethod(rule, "http://localhost:8080", http.MethodPut))
}

// TestAllOperationCORS rejects incomplete rules that break multipart or deletion.
func TestAllOperationCORS(t *testing.T) {
	rule := corsRule{
		AllowedOrigin: []string{"http://localhost:8080"},
		AllowedMethod: []string{"GET", "PUT", "POST", "DELETE"},
		AllowedHeader: []string{"*"},
		ExposeHeader:  []string{"ETag"},
	}
	assert.True(t, ruleAllowsAllObjectMethods(rule, "http://localhost:8080"))
	rule.AllowedMethod = []string{"PUT"}
	assert.False(t, ruleAllowsAllObjectMethods(rule, "http://localhost:8080"))
	rule.AllowedMethod = []string{"GET", "PUT", "POST", "DELETE"}
	rule.ExposeHeader = nil
	assert.False(t, ruleAllowsAllObjectMethods(rule, "http://localhost:8080"))
}
