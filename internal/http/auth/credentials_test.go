package auth

import (
	"context"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestAdminValidationErrors distinguishes invalid tokens from upstream failures.
func TestAdminValidationErrors(t *testing.T) {
	for _, tc := range []struct {
		name         string
		status       int
		response     string
		unauthorized bool
		errorPart    string
	}{
		{"unauthorized", 401, `{}`, true, ""},
		{"forbidden", 403, `{}`, true, ""},
		{"upstream failure", 502, `{}`, false, "HTTP 502"},
		{"unknown endpoint", 404, `{}`, false, "HTTP 404"},
		{"malformed response", 200, `{`, false, "decode Garage GetCurrentAdminTokenInfo"},
		{"missing fields", 200, `{}`, false, "incomplete admin token metadata"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/v2/GetCurrentAdminTokenInfo" || r.Header.Get("Authorization") != "Bearer secret" {
					t.Error("unexpected Garage token verification request")
				}
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.response))
			}))
			defer api.Close()
			svc, err := New(&testStore{records: map[string]testRecord{}}, Config{
				GarageURL:     api.URL,
				PublicURL:     "http://localhost:8080",
				SessionSecret: base64.StdEncoding.EncodeToString(make([]byte, 32)),
			})
			require.NoError(t, err)
			_, err = svc.Admin(context.Background(), "secret")
			require.Error(t, err)
			if tc.unauthorized {
				require.ErrorIs(t, err, ErrUnauthenticated)
			} else if !strings.Contains(err.Error(), tc.errorPart) {
				t.Errorf("expected %q in %v", tc.errorPart, err)
			}
		})
	}
}

// TestRejectMixedCredentials prevents secondary credential types from bypassing validation.
func TestRejectMixedCredentials(t *testing.T) {
	if validAdminCredential(Credential{Token: "a", AccessKeyID: "b"}) || validS3Credential(Credential{Token: "a", AccessKeyID: "b", SecretAccessKey: "secret"}) {
		t.Fatal("mixed Garage credential types accepted")
	}
}
