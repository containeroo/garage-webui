package endpoint

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTransferRequestValidation(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name          string
		method        string
		metadata      string
		body          string
		contentLength int64
		wantStatus    int
	}{
		{name: "listing", method: http.MethodGet, metadata: `{"bucketId":"bucket","action":"list"}`},
		{name: "single upload", method: http.MethodPut, metadata: `{"bucketId":"bucket","key":"file","action":"upload","size":4}`, body: "data", contentLength: 4},
		{name: "oversized metadata", method: http.MethodGet, metadata: strings.Repeat("x", maxTransferMetadataBytes+1), wantStatus: http.StatusBadRequest},
		{name: "unknown fields", method: http.MethodGet, metadata: `{"action":"list","unexpected":true}`, wantStatus: http.StatusBadRequest},
		{name: "trailing JSON", method: http.MethodGet, metadata: `{"action":"list"} {}`, wantStatus: http.StatusBadRequest},
		{name: "wrong method", method: http.MethodGet, metadata: `{"action":"delete"}`, wantStatus: http.StatusBadRequest},
		{name: "size mismatch", method: http.MethodPut, metadata: `{"action":"upload","size":5}`, body: "data", contentLength: 4, wantStatus: http.StatusBadRequest},
		{name: "oversized control body", method: http.MethodPost, metadata: `{"action":"multipart-complete"}`, contentLength: maxMultipartControlBytes + 1, wantStatus: http.StatusRequestEntityTooLarge},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			request := httptest.NewRequest(tc.method, "/api/objects/transfer?"+url.Values{"request": {tc.metadata}}.Encode(), strings.NewReader(tc.body))
			request.ContentLength = tc.contentLength
			_, err := decodeTransferRequest(request)
			if tc.wantStatus == 0 {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Equal(t, tc.wantStatus, transferErrorStatus(err))
		})
	}
}
