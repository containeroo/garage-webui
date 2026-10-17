// Package garage communicates with the Garage v2.4.1 Admin API.
package garage

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Client sends authenticated requests to Garage.
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
		return nil, fmt.Errorf("invalid Garage Admin API URL")
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
	return (u.Scheme == "http" || u.Scheme == "https") && u.Host != ""
}

// hasNoURLExtras rejects credentials, query and fragment components.
func hasNoURLExtras(u *url.URL) bool {
	return u.User == nil && u.RawQuery == "" && u.Fragment == "" && !u.ForceQuery
}

// Request sends an API request without exposing credentials to the caller.
func (c *Client) Request(
	ctx context.Context,
	method, endpoint string,
	query url.Values,
	body io.Reader,
) (statusCode int, responseBody []byte, err error) {
	u := *c.base
	u.Path += endpoint
	u.RawQuery = query.Encode()
	req, err := http.NewRequestWithContext(ctx, method, u.String(), body)
	if err != nil {
		return 0, nil, err
	}

	req.Header.Set("Authorization", "Bearer "+c.token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return 0, nil, fmt.Errorf("garage Admin API request failed: %w", err)
	}

	defer resp.Body.Close() // nolint:errcheck
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes+1))
	if err != nil {
		return 0, nil, fmt.Errorf("read Garage response: %w", err)
	}
	if len(data) > maxResponseBytes {
		return 0, nil, fmt.Errorf("garage response exceeds %d bytes", maxResponseBytes)
	}

	return resp.StatusCode, data, nil
}
