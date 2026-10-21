// Package response defines consistent HTTP response formats for the web interface.
package response

import (
	"net/http"

	httpkitresponse "github.com/containeroo/httpkit/response"
)

// FieldProblem describes a validation problem associated with a request field.
type FieldProblem struct {
	// Field identifies the invalid request field.
	Field string

	// Message explains the validation problem without exposing internal details.
	Message string
}

// NewFieldProblem creates a field-specific validation problem.
func NewFieldProblem(field, message string) FieldProblem {
	return FieldProblem{Field: field, Message: message}
}

// Respond writes a JSON response with the supplied status code.
func Respond(w http.ResponseWriter, status int, value any) {
	httpkitresponse.WriteJSON(w, status, value)
}

// Problem writes the application's JSON error contract with optional field problems.
func Problem(w http.ResponseWriter, status int, message string, problems ...FieldProblem) {
	fields := make(map[string]string, len(problems))
	for _, problem := range problems {
		fields[problem.Field] = problem.Message
	}

	Respond(w, status, map[string]any{
		"error":    message,
		"problems": fields,
	})
}
