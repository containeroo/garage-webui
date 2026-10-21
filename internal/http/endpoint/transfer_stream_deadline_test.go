package endpoint

import (
	"bytes"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// failingDeadlineWriter simulates a response writer that cannot extend its deadline.
type failingDeadlineWriter struct {
	// ResponseRecorder captures the response written after the deadline failure.
	*httptest.ResponseRecorder
}

// SetWriteDeadline reports an unexpected deadline-control failure.
func (w *failingDeadlineWriter) SetWriteDeadline(time.Time) error {
	return errors.New("private deadline failure")
}

// TestObjectStreamDeadlineFailure ensures transport failures are logged but not exposed.
func TestObjectStreamDeadlineFailure(t *testing.T) {
	t.Parallel()

	var logs bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logs, nil))
	handler := &objectStream{logger: logger}
	writer := &failingDeadlineWriter{ResponseRecorder: httptest.NewRecorder()}

	handler.ServeHTTP(writer, httptest.NewRequest(http.MethodGet, "/api/objects/transfer", nil))

	assert.Equal(t, http.StatusInternalServerError, writer.Code)
	require.Contains(t, writer.Header().Get("Content-Type"), "application/json")
	assert.Contains(t, writer.Body.String(), "the request could not be processed")
	assert.NotContains(t, writer.Body.String(), "private deadline failure")
	assert.Contains(t, logs.String(), "private deadline failure")
	assert.True(t, strings.Contains(writer.Body.String(), "reference:"))
}
