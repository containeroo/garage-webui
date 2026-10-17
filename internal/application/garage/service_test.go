package garage

import (
	"context"
	"errors"
	"io"
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testRequester records forwarded requests.
type testRequester struct {
	calls int
	query url.Values
	body  string
	err   error
}

// Request records an operation and returns a test response.
func (r *testRequester) Request(_ context.Context, _, _ string, query url.Values, body io.Reader) (int, []byte, error) {
	r.calls++
	r.query = query
	if body != nil {
		data, _ := io.ReadAll(body)
		r.body = string(data)
	}

	return 202, []byte("response"), r.err
}

// TestRequest checks authorization and forwarding at the application boundary.
func TestRequest(t *testing.T) {
	client := &testRequester{}
	service := New(client)
	ctx := context.Background()
	for _, tc := range []struct {
		role, method, operation string
		want                    error
	}{
		{"viewer", "POST", "CreateBucket", ErrForbidden},
		{"editor", "POST", "UpdateClusterLayout", ErrForbidden},
		{"viewer", "POST", "ConnectClusterNodes", ErrForbidden},
		{"admin", "GET", "Unknown", ErrUnknownOperation},
		{"admin", "GET", "CreateBucket", ErrMethod},
	} {
		_, _, err := service.Request(ctx, tc.role, tc.method, tc.operation, nil, nil)
		require.ErrorIs(t, err, tc.want)
	}

	assert.Zero(t, client.calls)

	query := url.Values{
		"id":            {"bucket"},
		"showSecretKey": {"true"},
	}
	status, data, err := service.Request(ctx, "editor", "POST", "CreateBucket", query, strings.NewReader(`{}`))
	require.NoError(t, err)
	assert.Equal(t, 202, status)
	assert.Equal(t, []byte("response"), data)
	assert.Equal(t, url.Values{"id": {"bucket"}}, client.query)
	assert.Equal(t, "true", query.Get("showSecretKey"))
	assert.Equal(t, `{}`, client.body)
	assert.Equal(t, 1, client.calls)

	client.err = errors.New("unavailable")
	_, _, err = service.Request(ctx, "viewer", "GET", "ListBuckets", nil, nil)
	assert.ErrorIs(t, err, client.err)
}

func TestClusterOperations(t *testing.T) {
	service := New(&testRequester{})
	for _, op := range []string{"ConnectClusterNodes", "UpdateClusterLayout", "PreviewClusterLayoutChanges", "ApplyClusterLayout", "RevertClusterLayout"} {
		require.NoError(t, service.Validate("admin", "POST", op))
		require.ErrorIs(t, service.Validate("editor", "POST", op), ErrForbidden)
		require.ErrorIs(t, service.Validate("viewer", "POST", op), ErrForbidden)
	}
	require.NoError(t, service.Validate("viewer", "GET", "GetClusterStatus"))
}
