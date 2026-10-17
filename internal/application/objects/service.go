// Package objects coordinates authorized object operations.
package objects

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/gi8lino/garage-webui/internal/application/garage"
)

const maxUploadSize int64 = 5 << 30

// Errors returned by object operations.
var (
	ErrInvalid   = errors.New("invalid object request")
	ErrForbidden = errors.New("object operation forbidden")
	ErrBucket    = errors.New("bucket not found or missing a global S3 alias")
)

// BucketResolver resolves a trusted bucket ID and global alias.
type BucketResolver interface {
	ResolveBucket(context.Context, string) (bucketID, alias string, err error)
}

// Service authorizes and prepares object operations.
type Service struct {
	buckets BucketResolver
	signer  Signer
}

// New creates the object operation service.
func New(buckets BucketResolver, signer Signer) *Service {
	return &Service{
		buckets: buckets,
		signer:  signer,
	}
}

// Signer prepares bucket-scoped object operations.
type Signer interface {
	ListPrefixURL(context.Context, string, string, string, string, time.Time) (string, error)
	DownloadURL(context.Context, string, string, string, time.Time) (string, error)
	PreviewURL(context.Context, string, string, string, time.Time) (string, error)
	DeleteURL(context.Context, string, string, string, time.Time) (string, error)
	UploadURL(context.Context, string, string, string, time.Time) (string, error)
	CopyURL(context.Context, string, string, string, string, time.Time) (signedURL, copySource string, err error)
	MultipartURL(context.Context, string, string, string, string, string, int, time.Time) (string, error)
	CopyMultipartURL(context.Context, string, string, string, string, string, string, int, int64, int64, time.Time) (signedURL string, requiredHeaders map[string]string, err error)
}

// Request identifies an object operation and its parameters.
type Request struct {
	// BucketID identifies the selected Garage bucket.
	BucketID string
	// Action identifies the requested operation.
	Action string
	// Key identifies the target object.
	Key string
	// Continuation is the listing cursor.
	Continuation string
	// Prefix filters a listing.
	Prefix string
	// SourceKey identifies the source of a copy.
	SourceKey string
	// UploadID identifies a multipart upload.
	UploadID string
	// PartNumber identifies a multipart part.
	PartNumber int
	// RangeStart is the first byte of a copied part.
	RangeStart int64
	// RangeEnd is the inclusive last byte of a copied part.
	RangeEnd int64
}

// Result describes a signed browser request.
type Result struct {
	// URL is the signed request URL.
	URL string
	// Method is the HTTP method to use.
	Method string
	// CopySource is the signed copy-source header.
	CopySource string
	// CopyHeaders are the signed multipart-copy headers.
	CopyHeaders map[string]string
}

// Prepare authorizes and signs an object operation.
func (s *Service) Prepare(
	ctx context.Context,
	role string,
	input Request,
	now time.Time,
) (Result, error) {
	if !validObjectRequest(input) {
		return Result{}, ErrInvalid
	}

	if !canAccess(role, input.Action) {
		return Result{}, ErrForbidden
	}

	bucketID, alias, err := s.buckets.ResolveBucket(ctx, input.BucketID)
	if err != nil {
		return Result{}, fmt.Errorf("%w: %v", ErrBucket, err)
	}

	return s.sign(ctx, bucketID, alias, input, now)
}

// PrepareUpload authorizes and signs a single-request upload.
func (s *Service) PrepareUpload(
	ctx context.Context,
	role, bucketID, key string,
	size int64,
	now time.Time,
) (Result, error) {
	if !garage.CanModify(role) {
		return Result{}, ErrForbidden
	}

	if !validUploadRequest(bucketID, key, size) {
		return Result{}, ErrInvalid
	}

	id, alias, err := s.buckets.ResolveBucket(ctx, bucketID)
	if err != nil {
		return Result{}, fmt.Errorf("%w: %v", ErrBucket, err)
	}

	address, err := s.signer.UploadURL(ctx, id, alias, key, now)
	return Result{
		URL:    address,
		Method: http.MethodPut,
	}, err
}

// canAccess requires a recognized role and editor access for mutations.
func canAccess(role, action string) bool {
	if garage.CanModify(role) {
		return true
	}

	return role == "viewer" && !objectMutation(action)
}

// sign dispatches a validated operation to its signer.
func (s *Service) sign(
	ctx context.Context,
	bucketID, alias string,
	input Request,
	now time.Time,
) (Result, error) {
	signer := s.signer
	var result Result
	var err error
	switch input.Action {
	case "list":
		result.Method = http.MethodGet
		result.URL, err = signer.ListPrefixURL(ctx, bucketID, alias, input.Prefix, input.Continuation, now)
	case "download":
		result.Method = http.MethodGet
		result.URL, err = signer.DownloadURL(ctx, bucketID, alias, input.Key, now)
	case "preview":
		result.Method = http.MethodGet
		result.URL, err = signer.PreviewURL(ctx, bucketID, alias, input.Key, now)
	case "delete":
		result.Method = http.MethodDelete
		result.URL, err = signer.DeleteURL(ctx, bucketID, alias, input.Key, now)
	case "copy":
		result.Method = http.MethodPut
		result.URL, result.CopySource, err = signer.CopyURL(ctx, bucketID, alias, input.SourceKey, input.Key, now)
	case "multipart-start", "multipart-part", "multipart-complete", "multipart-abort":
		result.Method = multipartMethod(input.Action)
		result.URL, err = signer.MultipartURL(
			ctx,
			bucketID,
			alias,
			input.Key,
			input.UploadID,
			input.Action,
			input.PartNumber,
			now,
		)
	case "multipart-copy-start", "multipart-copy-part", "multipart-copy-complete", "multipart-copy-abort":
		result.Method = multipartMethod(input.Action)
		result.URL, result.CopyHeaders, err = signer.CopyMultipartURL(
			ctx,
			bucketID,
			alias,
			input.SourceKey,
			input.Key,
			input.UploadID,
			input.Action,
			input.PartNumber,
			input.RangeStart,
			input.RangeEnd,
			now,
		)
	default:
		return Result{}, errors.New("unsupported object operation")
	}

	return result, err
}

// multipartMethod maps multipart steps to their S3 methods.
func multipartMethod(action string) string {
	switch action {
	case "multipart-part", "multipart-copy-part":
		return http.MethodPut
	case "multipart-abort", "multipart-copy-abort":
		return http.MethodDelete
	default:
		return http.MethodPost
	}
}
