package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/containeroo/garage-webui/internal/application/objects"
	"github.com/containeroo/garage-webui/internal/objectaction"
	"github.com/containeroo/garage-webui/internal/s3"
)

const (
	// Transfer metadata contains bounded object keys, a cursor and multipart identifiers, never file bytes.
	maxTransferMetadataBytes = 16 << 10
	// Ten thousand multipart receipts fit comfortably within this XML control-body bound.
	maxMultipartControlBytes int64 = 8 << 20
)

var errTransferBodyTooLarge = errors.New("request body too large")

// transferRequest separates operation metadata from the streamed HTTP body.
type transferRequest struct {
	objectRequest
	Size int64 `json:"size"`
}

// decodeTransferRequest requires exactly one bounded JSON object and validates its HTTP envelope.
func decodeTransferRequest(r *http.Request) (transferRequest, error) {
	var input transferRequest
	raw := r.URL.Query().Get("request")
	if len(raw) == 0 || len(raw) > maxTransferMetadataBytes {
		return input, fmt.Errorf("%w: invalid transfer metadata size", objects.ErrInvalid)
	}

	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		return input, fmt.Errorf("%w: invalid transfer metadata", objects.ErrInvalid)
	}

	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return input, fmt.Errorf("%w: trailing transfer metadata", objects.ErrInvalid)
	}

	return input, input.validateEnvelope(r)
}

// validateEnvelope rejects method mismatches and body sizes before provisioning any credentials.
func (input transferRequest) validateEnvelope(r *http.Request) error {
	expected, valid := input.Action.Method()

	if !valid || r.Method != expected {
		return fmt.Errorf("%w: invalid object operation method", objects.ErrInvalid)
	}

	if r.ContentLength > input.bodyLimit() {
		return errTransferBodyTooLarge
	}

	if input.isSingleUpload() && r.ContentLength != input.Size {
		return fmt.Errorf("%w: upload size does not match request", objects.ErrInvalid)
	}

	return nil
}

// isSingleUpload returns true if the request is a single-part upload.
func (input transferRequest) isSingleUpload() bool { return input.Action == objectaction.Upload }

// bodyLimit distinguishes streamed file bytes from bounded multipart control documents.
func (input transferRequest) bodyLimit() int64 {
	if input.isSingleUpload() || input.Action == objectaction.MultipartPart {
		return s3.MaxUploadSize
	}
	return maxMultipartControlBytes
}

// transferErrorStatus maps application failures without leaking upstream signed URLs.
func transferErrorStatus(err error) int {
	switch {
	case errors.Is(err, errTransferBodyTooLarge):
		return http.StatusRequestEntityTooLarge
	case errors.Is(err, objects.ErrInvalid), errors.Is(err, objects.ErrBucket):
		return http.StatusBadRequest
	case errors.Is(err, objects.ErrForbidden):
		return http.StatusForbidden
	default:
		return http.StatusBadGateway
	}
}
