package s3

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestListBucketsClient decodes S3 XML while differentiating auth and transport failures.
func TestListBucketsClient(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		body   string
		want   []string
		denied bool
	}{
		{"valid", 200, `<ListAllMyBucketsResult><Buckets><Bucket><Name>first</Name></Bucket><Bucket><Name>second</Name></Bucket></Buckets></ListAllMyBucketsResult>`, []string{"first", "second"}, false},
		{"denied", 403, `<Error/>`, nil, true},
		{"outage", 502, `<Error/>`, nil, false},
		{"invalid XML", 200, `<ListAllMyBucketsResult`, nil, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Query().Get("X-Amz-Signature") == "" {
					t.Error("missing signed S3 URL")
				}
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer api.Close()
			client, err := NewClient(api.URL, "garage", "GKtest", "secret", "http://localhost:8080")
			if err != nil {
				t.Fatal(err)
			}
			result, err := client.ListBuckets(context.Background())
			if tc.want != nil {
				if err != nil || len(result) != len(tc.want) {
					t.Fatalf("unexpected S3 buckets: %v, %v", result, err)
				}
				return
			}
			if err == nil || errors.Is(err, ErrUnauthorized) != tc.denied {
				t.Fatalf("unexpected S3 error classification: %v", err)
			}
		})
	}
}
