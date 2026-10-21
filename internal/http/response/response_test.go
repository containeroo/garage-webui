package response

import (
	"bytes"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	httpkitresponse "github.com/containeroo/httpkit/response"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRespond checks the JSON representation of successful responses.
func TestRespond(t *testing.T) {
	t.Parallel()

	recorder := httptest.NewRecorder()
	Respond(recorder, http.StatusCreated, map[string]string{"id": "bucket"})
	assert.Equal(t, http.StatusCreated, recorder.Code)
	assert.Contains(t, recorder.Header().Get("Content-Type"), "application/json")
	assert.JSONEq(t, `{"id":"bucket"}`, recorder.Body.String())
}

// TestProblem checks the shared error contract with and without field validation.
func TestProblem(t *testing.T) {
	t.Parallel()

	t.Run("without field problems", func(t *testing.T) {
		t.Parallel()

		recorder := httptest.NewRecorder()
		Problem(recorder, http.StatusBadRequest, "invalid request")
		assert.Equal(t, http.StatusBadRequest, recorder.Code)
		assert.Contains(t, recorder.Header().Get("Content-Type"), "application/json")
		assert.JSONEq(t, `{"error":"invalid request","problems":{}}`, recorder.Body.String())
	})

	t.Run("with field problems", func(t *testing.T) {
		t.Parallel()

		recorder := httptest.NewRecorder()
		Problem(recorder, http.StatusUnprocessableEntity, "invalid request",
			NewFieldProblem("bucket", "bucket is required"),
			NewFieldProblem("key", "invalid object key"),
		)
		assert.Equal(t, http.StatusUnprocessableEntity, recorder.Code)
		assert.JSONEq(t, `{"error":"invalid request","problems":{"bucket":"bucket is required","key":"invalid object key"}}`, recorder.Body.String())
	})
}

// TestInternalServerError ensures private diagnostics never leak into an HTTP response.
func TestInternalServerError(t *testing.T) {
	t.Parallel()

	t.Run("writes safe problem and logs the private cause", func(t *testing.T) {
		t.Parallel()

		var logs bytes.Buffer
		logger := slog.New(slog.NewTextHandler(&logs, nil))
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodGet, "/api/private?token=hidden", nil)
		writer := httpkitresponse.NewRequestWriter(recorder, request)
		InternalServerError(logger, writer, errors.New("private database credential"))

		assert.Equal(t, http.StatusInternalServerError, recorder.Code)
		assert.NotEmpty(t, writer.ErrorReference)
		assert.Contains(t, recorder.Body.String(), writer.ErrorReference)
		assert.NotContains(t, recorder.Body.String(), "private database credential")
		assert.Contains(t, logs.String(), "private database credential")
		assert.NotContains(t, logs.String(), "token=hidden")
	})

	t.Run("does not write after a response is committed", func(t *testing.T) {
		t.Parallel()

		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodGet, "/api/private", nil)
		writer := httpkitresponse.NewRequestWriter(recorder, request)
		writer.WriteHeader(http.StatusAccepted)
		InternalServerError(slog.New(slog.NewTextHandler(&strings.Builder{}, nil)), writer, errors.New("private cause"))

		require.Equal(t, http.StatusAccepted, recorder.Code)
		assert.Empty(t, recorder.Body.String())
		assert.NotEmpty(t, writer.ErrorReference)
	})
}
