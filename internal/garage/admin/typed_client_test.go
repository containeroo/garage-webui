package admin

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

// TestTypedClientErrorMapping distinguishes token failures, resource absence and outages.
func TestTypedClientErrorMapping(t *testing.T) {
	for _, tc := range []struct {
		status int
		want   error
	}{
		{http.StatusUnauthorized, ErrUnauthorized},
		{http.StatusForbidden, ErrForbidden},
		{http.StatusNotFound, ErrNotFound},
		{http.StatusConflict, ErrConflict},
		{http.StatusBadGateway, nil},
	} {
		t.Run(http.StatusText(tc.status), func(t *testing.T) {
			api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tc.status)
			}))
			defer api.Close()
			client, err := New(api.URL, "secret")
			if err != nil {
				t.Fatal(err)
			}
			_, err = client.CurrentAdminToken(context.Background())
			if err == nil {
				t.Fatal("expected Garage rejection")
			}
			if tc.want != nil && !errors.Is(err, tc.want) {
				t.Fatalf("expected %v, got %v", tc.want, err)
			}
			if tc.want == nil && errors.Is(err, ErrUnauthorized) {
				t.Fatal("upstream failure was incorrectly classified as invalid credentials")
			}
			if strings.Contains(err.Error(), "secret") {
				t.Fatal("client error exposed the Garage token")
			}
		})
	}
}

// TestTypedClientOperations prevents arbitrary methods and protects key secrets.
func TestTypedClientOperations(t *testing.T) {
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer secret" {
			t.Error("admin token was not forwarded")
		}
		switch r.URL.Path {
		case "/v2/GetCurrentAdminTokenInfo":
			_, _ = w.Write([]byte(`{"name":"operator","scope":["ListBuckets"],"expired":false}`))
		case "/v2/GetKeyInfo":
			if r.URL.Query().Has("showSecretKey") {
				t.Error("proxy leaked secret-key query flag")
			}
			_, _ = w.Write([]byte(`{"accessKeyId":"key"}`))
		case "/v2/AllowBucketKey":
			if r.Method != http.MethodPost {
				t.Error("permission update used the wrong method")
			}
			_, _ = w.Write([]byte(`{}`))
		default:
			t.Errorf("unexpected endpoint %s", r.URL.Path)
		}
	}))
	defer api.Close()
	client, err := New(api.URL, "secret")
	if err != nil {
		t.Fatal(err)
	}
	info, err := client.CurrentAdminToken(context.Background())
	if err != nil || info.Name != "operator" {
		t.Fatalf("unexpected current token: %+v, %v", info, err)
	}
	query := url.Values{"id": {"key"}, "showSecretKey": {"true"}}
	status, _, err := client.Proxy(context.Background(), OpGetKeyInfo, query, nil)
	if err != nil || status != http.StatusOK || query.Get("showSecretKey") != "true" {
		t.Fatalf("proxy failed to isolate query: status=%d err=%v", status, err)
	}
	if err := client.AllowBucketKey(context.Background(), "bucket", "key", BucketPermissions{Read: true}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := client.Proxy(context.Background(), Operation("../../Metrics"), nil, nil); err == nil {
		t.Fatal("unknown upstream endpoint was permitted")
	}
}

// TestGarageErrorsStartLowercase keeps API errors consistent with Go conventions.
func TestGarageErrorsStartLowercase(t *testing.T) {
	for _, err := range []error{
		ErrUnauthorized,
		ErrForbidden,
		ErrNotFound,
		ErrConflict,
		&StatusError{Operation: OpGetBucketInfo, Status: http.StatusBadRequest},
	} {
		message := err.Error()
		if message == "" || message[0] < 'a' || message[0] > 'z' {
			t.Errorf("error message must start lowercase: %q", message)
		}
	}
}
