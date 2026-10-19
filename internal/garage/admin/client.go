// Package admin provides a typed client for the Garage v2.4.1 Admin API.
package admin

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/containeroo/garage-webui/internal/utils"
)

// Errors describe transport-independent failures returned by Garage.
var (
	ErrUnauthorized = errors.New("garage token is invalid")
	ErrForbidden    = errors.New("garage operation is forbidden")
	ErrNotFound     = errors.New("garage resource not found")
	ErrConflict     = errors.New("garage resource conflict")
)

// StatusError reports a rejected Admin API operation without exposing its body or credentials.
type StatusError struct {
	// Operation identifies the rejected Garage endpoint.
	Operation Operation
	// Status is the upstream HTTP status code.
	Status int
}

// Error explains which upstream operation failed.
func (e *StatusError) Error() string {
	return fmt.Sprintf("garage %s returned HTTP %d", e.Operation, e.Status)
}

// Is maps HTTP status codes into errors callers can classify with errors.Is.
func (e *StatusError) Is(target error) bool {
	switch e.Status {
	case http.StatusUnauthorized:
		return target == ErrUnauthorized
	case http.StatusForbidden:
		return target == ErrForbidden
	case http.StatusNotFound:
		return target == ErrNotFound
	case http.StatusConflict:
		return target == ErrConflict
	default:
		return false
	}
}

// Client sends requests to Garage using exactly one signed-in administrator's token.
type Client struct {
	base  *url.URL
	token string
	http  *http.Client
}

const maxResponseBytes = 4 << 20

// New validates the Admin API URL and constructs a client.
func New(rawURL, token string) (*Client, error) {
	u, err := url.Parse(rawURL)
	if err != nil || !validAdminURL(u) {
		return nil, errors.New("invalid Garage Admin API URL")
	}

	u.Path = strings.TrimRight(u.Path, "/") + "/v2/"

	return &Client{
		base:  u,
		token: token,
		http: &http.Client{
			Timeout:       10 * time.Second,
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		},
	}, nil
}

// validAdminURL reports whether a URL can address the Garage Admin API.
func validAdminURL(u *url.URL) bool {
	return u != nil && hasHTTPHost(u) && hasNoURLExtras(u)
}

// hasHTTPHost reports whether a URL has an HTTP scheme and host.
func hasHTTPHost(u *url.URL) bool {
	return utils.HasValidScheme(u) && u.Host != ""
}

// hasNoURLExtras rejects credentials, query and fragment components.
func hasNoURLExtras(u *url.URL) bool {
	return u.User == nil && u.RawQuery == "" && u.Fragment == "" && !u.ForceQuery
}

// request exchanges a known operation's JSON body and returns its response.
// Only this transport method handles HTTP status codes or JSON decoding.
func (c *Client) request(ctx context.Context, op Operation, query url.Values, input, output any) error {
	var body []byte
	if input != nil {
		var err error
		body, err = json.Marshal(input)
		if err != nil {
			return fmt.Errorf("encode Garage %s: %w", op, err)
		}
	}

	status, data, err := c.send(ctx, op, query, body)
	if err != nil {
		return err
	}

	if status < http.StatusOK || status >= http.StatusMultipleChoices {
		return &StatusError{Operation: op, Status: status}
	}

	if output == nil {
		return nil
	}

	if err := json.Unmarshal(data, output); err != nil {
		return fmt.Errorf("decode Garage %s: %w", op, err)
	}

	return nil
}

// Proxy forwards an already-validated browser operation, preserving Garage's status and JSON.
// It does not accept a method or arbitrary endpoint string from the caller.
func (c *Client) Proxy(ctx context.Context, op Operation, query url.Values, jsonBody []byte) (int, []byte, error) {
	if !validProxyBody(jsonBody) {
		return 0, nil, errors.New("invalid Garage proxy JSON body")
	}
	// Key secrets are never exposed through the generic browser proxy.
	if op == OpGetKeyInfo {
		query = maps.Clone(query)
		query.Del("showSecretKey")
	}

	return c.send(ctx, op, query, jsonBody)
}

// validProxyBody bounds requests and rejects malformed user-supplied JSON.
func validProxyBody(body []byte) bool {
	return len(body) <= 64<<10 && (len(body) == 0 || json.Valid(body))
}

// send is the only Admin API HTTP transport, shared by the typed client and proxy.
func (c *Client) send(ctx context.Context, op Operation, query url.Values, body []byte) (int, []byte, error) {
	method, allowed := op.Method()
	if !allowed {
		return 0, nil, fmt.Errorf("unsupported Garage operation %q", op)
	}

	u := *c.base
	u.Path += string(op)
	u.RawQuery = query.Encode()
	var content io.Reader
	if len(body) > 0 {
		content = bytes.NewReader(body)
	}

	req, err := http.NewRequestWithContext(ctx, method, u.String(), content)
	if err != nil {
		return 0, nil, err
	}

	req.Header.Set("Authorization", "Bearer "+c.token)
	if len(body) > 0 {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return 0, nil, fmt.Errorf("garage %s request failed: %w", op, err)
	}

	defer resp.Body.Close() // nolint:errcheck
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes+1))
	if err != nil {
		return 0, nil, fmt.Errorf("read Garage %s: %w", op, err)
	}

	if len(data) > maxResponseBytes {
		return 0, nil, fmt.Errorf("garage response exceeds %d bytes", maxResponseBytes)
	}

	return resp.StatusCode, data, nil
}
