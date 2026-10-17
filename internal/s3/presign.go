// Package s3 signs short-lived, browser-direct Garage S3 operations.
// Signing keys stay server-side; the browser receives only a scoped URL.
package s3

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"path"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gi8lino/garage-webui/internal/objectpath"
)

const (
	// MaxUploadSize is the maximum single-request upload size (5 GiB).
	MaxUploadSize int64 = 5 << 30
	// URLLifetime is how long a newly issued object URL remains usable.
	URLLifetime = 5 * time.Minute
	// MaxMultipartSize is Garage's S3 multipart object limit.
	MaxMultipartSize int64 = 5 << 40
	// ObjectPageSize bounds one bucket listing request.
	ObjectPageSize = 100
)

// Presigner creates S3 SigV4 query-authenticated URLs.
type Presigner struct {
	endpoint *url.URL
	region   string
	keyID    string
	secret   string
}

// New constructs a presigner with an existing Garage S3 credential.
// The endpoint must be a browser-reachable HTTP(S) origin.
func New(rawURL, region, keyID, secret, publicURL string) (*Presigner, error) {
	if rawURL == "" || region == "" || keyID == "" || secret == "" {
		return nil, errors.New("S3 signing requires an endpoint, region and Garage-managed key")
	}

	endpoint, err := url.Parse(rawURL)
	if err != nil || !validS3Origin(endpoint) {
		return nil, errors.New("S3 public URL must be an HTTP(S) origin without a path, credentials, query or fragment")
	}

	if strings.HasPrefix(publicURL, "https://") && endpoint.Scheme != "https" {
		return nil, errors.New("HTTPS web UI requires an HTTPS S3 public URL")
	}

	if strings.ContainsAny(region, "/\r\n ") || strings.ContainsAny(keyID, "\r\n") {
		return nil, errors.New("invalid S3 signing configuration")
	}

	return &Presigner{
		endpoint: endpoint,
		region:   region,
		keyID:    keyID,
		secret:   secret,
	}, nil
}

// UploadURL signs a PUT for one object.
func (s *Presigner) UploadURL(bucket, key string, now time.Time) (string, error) {
	return s.objectURL("PUT", bucket, key, nil, now)
}

// DeleteURL signs a DELETE for one object; the caller must authorize mutations.
func (s *Presigner) DeleteURL(bucket, key string, now time.Time) (string, error) {
	return s.objectURL("DELETE", bucket, key, nil, now)
}

// DownloadURL signs a GET that returns the selected object as an attachment.
func (s *Presigner) DownloadURL(bucket, key string, now time.Time) (string, error) {
	name := path.Base(key)
	query := url.Values{"response-content-disposition": {"attachment; filename*=UTF-8''" + strings.ReplaceAll(url.PathEscape(name), "+", "%2B")}}
	return s.objectURL("GET", bucket, key, query, now)
}

// PreviewURL signs an inline GET for an object without a download override.
func (s *Presigner) PreviewURL(bucket, key string, now time.Time) (string, error) {
	return s.objectURL("GET", bucket, key, nil, now)
}

// BeginMultipartURL signs the create-multipart-upload request.
func (s *Presigner) BeginMultipartURL(bucket, key string, now time.Time) (string, error) {
	return s.objectURL("POST", bucket, key, url.Values{"uploads": {""}}, now)
}

// PartURL signs a single part; signing is repeated throughout a long upload.
func (s *Presigner) PartURL(bucket, key, uploadID string, part int, now time.Time) (string, error) {
	if !validUploadID(uploadID) || part < 1 || part > 10000 {
		return "", errors.New("invalid multipart upload ID or part number")
	}

	return s.objectURL("PUT", bucket, key, url.Values{
		"uploadId":   {uploadID},
		"partNumber": {strconv.Itoa(part)},
	}, now)
}

// CompleteMultipartURL signs completion for the selected upload.
func (s *Presigner) CompleteMultipartURL(bucket, key, uploadID string, now time.Time) (string, error) {
	return s.multipartURL("POST", bucket, key, uploadID, now)
}

// AbortMultipartURL signs cancellation of the selected upload.
func (s *Presigner) AbortMultipartURL(bucket, key, uploadID string, now time.Time) (string, error) {
	return s.multipartURL("DELETE", bucket, key, uploadID, now)
}

// multipartURL binds upload ID, bucket, and object key to the signature.
func (s *Presigner) multipartURL(method, bucket, key, uploadID string, now time.Time) (string, error) {
	if !validUploadID(uploadID) {
		return "", errors.New("invalid multipart upload ID")
	}

	return s.objectURL(method, bucket, key, url.Values{"uploadId": {uploadID}}, now)
}

// CopyURL signs a source-bound server-side copy within one bucket.
// The browser must supply the returned exact x-amz-copy-source header.
func (s *Presigner) CopyURL(bucket, source, target string, now time.Time) (signedURL, copySource string, err error) {
	if !validBucket(bucket) || !objectpath.ValidObjectKey(source) || !objectpath.ValidObjectKey(target) || source == target {
		return "", "", errors.New("invalid copy source or destination")
	}

	header := (&url.URL{Path: "/" + bucket + "/" + source}).EscapedPath()
	address, err := s.signedWithHeaders("PUT", "/"+bucket+"/"+target, nil, map[string]string{"x-amz-copy-source": header}, now)
	return address, header, err
}

// CopyPartURL signs a ranged copy part with both source and range bound to SigV4.
func (s *Presigner) CopyPartURL(bucket, source, target, uploadID string, part int, start, end int64, now time.Time) (signedURL string, requiredHeaders map[string]string, err error) {
	if !validBucket(bucket) || !objectpath.ValidObjectKey(source) || !objectpath.ValidObjectKey(target) || source == target {
		return "", nil, errors.New("invalid multipart copy source or target")
	}

	if !validUploadID(uploadID) || part < 1 || part > 10000 || start < 0 || end < start || end-start >= 5<<30 {
		return "", nil, errors.New("invalid multipart copy part")
	}

	headers := map[string]string{
		"x-amz-copy-source":       (&url.URL{Path: "/" + bucket + "/" + source}).EscapedPath(),
		"x-amz-copy-source-range": fmt.Sprintf("bytes=%d-%d", start, end),
	}
	query := url.Values{
		"uploadId":   {uploadID},
		"partNumber": {strconv.Itoa(part)},
	}
	address, err := s.signedWithHeaders("PUT", "/"+bucket+"/"+target, query, headers, now)
	return address, headers, err
}

// validUploadID bounds Garage's opaque multipart identifier.
func validUploadID(id string) bool {
	return id != "" && len(id) <= 2048 && !strings.ContainsAny(id, "\x00\r\n")
}

// ListURL signs a paginated ListObjectsV2 request for one bucket.
func (s *Presigner) ListURL(bucket, continuation string, now time.Time) (string, error) {
	return s.ListPrefixURL(bucket, "", continuation, now)
}

// ListPrefixURL supports folder navigation and prefix searching with delimiters.
func (s *Presigner) ListPrefixURL(bucket, prefix, continuation string, now time.Time) (string, error) {
	if !validBucket(bucket) || !objectpath.ValidContinuationToken(continuation) || !objectpath.ValidPrefix(prefix) {
		return "", errors.New("invalid bucket alias or continuation token")
	}

	query := url.Values{
		"list-type":     {"2"},
		"max-keys":      {fmt.Sprintf("%d", ObjectPageSize)},
		"encoding-type": {"url"},
		"delimiter":     {"/"},
	}
	if continuation != "" {
		query.Set("continuation-token", continuation)
	}

	if prefix != "" {
		query.Set("prefix", prefix)
	}

	return s.signedURL("GET", "/"+bucket, query, now)
}

// objectURL checks object scoping before signing a request.
func (s *Presigner) objectURL(method, bucket, key string, query url.Values, now time.Time) (string, error) {
	if !validBucket(bucket) || !objectpath.ValidObjectKey(key) {
		return "", errors.New("invalid bucket alias or object key")
	}

	return s.signedURL(method, "/"+bucket+"/"+key, query, now)
}

// signedURL signs an operation without custom request headers.
func (s *Presigner) signedURL(method, objectPath string, parameters url.Values, now time.Time) (string, error) {
	return s.signedWithHeaders(method, objectPath, parameters, nil, now)
}

// signedWithHeaders binds required S3 headers to the SigV4 signature.
func (s *Presigner) signedWithHeaders(method, objectPath string, parameters url.Values, headers map[string]string, now time.Time) (string, error) {
	if s == nil {
		return "", errors.New("S3 operations are not configured")
	}

	u := *s.endpoint
	u.Path = objectPath
	stamp := now.UTC().Format("20060102T150405Z")
	date := stamp[:8]
	scope := date + "/" + s.region + "/s3/aws4_request"
	query := url.Values{}
	for name, values := range parameters {
		query[name] = append([]string(nil), values...)
	}

	headerNames := make([]string, 0, len(headers))
	for name := range headers {
		headerNames = append(headerNames, name)
	}

	sort.Strings(headerNames)
	signedHeaders := "host"
	canonicalHeaders := "host:" + u.Host + "\n"
	for _, name := range headerNames {
		signedHeaders += ";" + name
		canonicalHeaders += name + ":" + headers[name] + "\n"
	}

	query.Set("X-Amz-Algorithm", "AWS4-HMAC-SHA256")
	query.Set("X-Amz-Credential", s.keyID+"/"+scope)
	query.Set("X-Amz-Date", stamp)
	query.Set("X-Amz-Expires", strconv.Itoa(int(URLLifetime.Seconds())))
	query.Set("X-Amz-SignedHeaders", signedHeaders)
	canonicalQuery := strings.ReplaceAll(query.Encode(), "+", "%20")
	canonicalRequest := strings.Join([]string{
		method, u.EscapedPath(), canonicalQuery,
		canonicalHeaders, signedHeaders, "UNSIGNED-PAYLOAD",
	}, "\n")
	digest := sha256.Sum256([]byte(canonicalRequest))
	stringToSign := strings.Join([]string{
		"AWS4-HMAC-SHA256", stamp, scope, hex.EncodeToString(digest[:]),
	}, "\n")
	dateKey := sign([]byte("AWS4"+s.secret), date)
	regionKey := sign(dateKey, s.region)
	serviceKey := sign(regionKey, "s3")
	signingKey := sign(serviceKey, "aws4_request")
	query.Set("X-Amz-Signature", hex.EncodeToString(sign(signingKey, stringToSign)))
	u.RawQuery = strings.ReplaceAll(query.Encode(), "+", "%20")
	return u.String(), nil
}

// validBucket accepts a Garage global alias usable as a path-style S3 bucket.
func validBucket(bucket string) bool {
	if bucket == "" || len(bucket) > 255 || strings.ContainsAny(bucket, "/\\?#\r\n") {
		return false
	}

	return bucket != "." && bucket != ".."
}

// sign computes one HMAC-SHA256 used in SigV4 key derivation.
func sign(key []byte, value string) []byte {
	h := hmac.New(sha256.New, key)
	_, _ = h.Write([]byte(value))
	return h.Sum(nil)
}

// validS3Origin rejects unsafe endpoint features and non-HTTP(S) URLs.
func validS3Origin(endpoint *url.URL) bool {
	if endpoint == nil || (endpoint.Scheme != "http" && endpoint.Scheme != "https") {
		return false
	}

	return endpoint.Host != "" &&
		endpoint.User == nil &&
		(endpoint.Path == "" || endpoint.Path == "/") &&
		endpoint.RawQuery == "" &&
		!endpoint.ForceQuery &&
		endpoint.Fragment == ""
}
