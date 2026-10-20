package credentials

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"net/http"
	"slices"
	"strings"
)

// corsRule uses Garage Admin API's XML-style JSON field names.
type corsRule struct {
	// ID identifies the managed CORS rule.
	ID string `json:"ID,omitempty"`
	// AllowedOrigin lists browser origins permitted by this CORS rule.
	AllowedOrigin []string `json:"AllowedOrigin"`
	// AllowedMethod lists HTTP methods permitted by this CORS rule.
	AllowedMethod []string `json:"AllowedMethod"`
	// AllowedHeader lists request headers permitted by this CORS rule.
	AllowedHeader []string `json:"AllowedHeader,omitempty"`
	// ExposeHeader lists response headers browsers may read.
	ExposeHeader []string `json:"ExposeHeader,omitempty"`
	// MaxAgeSeconds sets the browser preflight cache lifetime in seconds.
	MaxAgeSeconds int `json:"MaxAgeSeconds,omitempty"`
}

// ensureCORS permits the operation's browser origin and retains all other rules.
// CORS is configured using the Admin API, never with object signing keys.
func (m *Provider) ensureCORS(ctx context.Context, bucketID string) error {
	bucket, err := m.admin.GetBucket(ctx, bucketID)
	if err != nil {
		return fmt.Errorf("read bucket CORS: %w", err)
	}

	for _, raw := range bucket.CORSRules {
		var rule corsRule
		if err := json.Unmarshal(raw, &rule); err != nil {
			return fmt.Errorf("decode existing bucket CORS: %w", err)
		}

		if ruleAllowsAllObjectMethods(rule, m.origin) {
			return nil
		}
	}

	if len(bucket.CORSRules) >= 100 {
		return errors.New("cannot add browser CORS: bucket already has 100 rules")
	}

	raw, err := json.Marshal(corsRule{
		ID:            "garage-webui-object-operations",
		AllowedOrigin: []string{m.origin},
		AllowedMethod: []string{http.MethodGet, http.MethodPut, http.MethodPost, http.MethodDelete},
		AllowedHeader: []string{"*"},
		ExposeHeader:  []string{"ETag"},
		MaxAgeSeconds: 300,
	})
	if err != nil {
		return err
	}
	// S3 evaluates matching CORS rules in order; keep the managed rule first
	// so a pre-existing rule without an exposed ETag cannot break multipart.
	bucket.CORSRules = append([]json.RawMessage{raw}, bucket.CORSRules...)
	if err := m.admin.UpdateBucketCORS(ctx, bucketID, bucket.CORSRules); err != nil {
		return fmt.Errorf("save bucket CORS: %w", err)
	}

	return nil
}

// ruleAllowsMethod checks whether an existing rule permits this browser request.
func ruleAllowsMethod(rule corsRule, origin, method string) bool {
	if !slices.Contains(rule.AllowedOrigin, origin) || !slices.Contains(rule.AllowedMethod, method) {
		return false
	}

	switch method {
	case http.MethodPut:
		return allowsHeader(rule.AllowedHeader, "content-type") &&
			allowsHeader(rule.AllowedHeader, "x-amz-copy-source") &&
			allowsHeader(rule.AllowedHeader, "x-amz-copy-source-range")
	case http.MethodPost:
		return allowsHeader(rule.AllowedHeader, "content-type")
	default:
		return true
	}
}

// allowsHeader checks exact or wildcard CORS request-header support.
func allowsHeader(headers []string, name string) bool {
	return slices.ContainsFunc(headers, func(header string) bool {
		return header == "*" || strings.EqualFold(header, name)
	})
}

// corsExposesETag ensures multipart part identifiers can be read by the browser.
func corsExposesETag(rule corsRule, method string) bool {
	if method != http.MethodPut {
		return true
	}

	return slices.ContainsFunc(rule.ExposeHeader, func(value string) bool {
		return value == "*" || strings.EqualFold(value, "etag")
	})
}

// ruleAllowsAllObjectMethods checks the full browser S3 workflow, including multipart.
func ruleAllowsAllObjectMethods(rule corsRule, origin string) bool {
	for _, method := range []string{http.MethodGet, http.MethodPut, http.MethodPost, http.MethodDelete} {
		if !ruleAllowsMethod(rule, origin, method) {
			return false
		}
	}

	return corsExposesETag(rule, http.MethodPut)
}
