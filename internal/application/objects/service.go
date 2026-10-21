// Package objects coordinates authorized object operations.
package objects

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/containeroo/garage-webui/internal/credential"
	"github.com/containeroo/garage-webui/internal/objectaction"
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
	// ResolveBucket resolves a bucket reference into its identifier and usable alias.
	ResolveBucket(context.Context, string) (bucketID, alias string, err error)
}

// Service authorizes and prepares object operations.
type Service struct {
	// buckets resolves and authorizes bucket references.
	buckets BucketResolver
	// signer creates signed requests for validated object operations.
	signer Signer
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
	// ListPrefixURL signs a paginated object listing for a prefix.
	ListPrefixURL(context.Context, string, string, string, string, time.Time) (string, error)
	// DownloadURL signs an object download.
	DownloadURL(context.Context, string, string, string, time.Time) (string, error)
	// PreviewURL signs an inline object preview.
	PreviewURL(context.Context, string, string, string, time.Time) (string, error)
	// DeleteURL signs an object deletion.
	DeleteURL(context.Context, string, string, string, time.Time) (string, error)
	// UploadURL signs an object upload.
	UploadURL(context.Context, string, string, string, time.Time) (string, error)
	// CopyURL signs an object copy and returns its source header.
	CopyURL(context.Context, string, string, string, string, time.Time) (signedURL, copySource string, err error)
	// MultipartURL signs a multipart upload operation.
	MultipartURL(context.Context, string, string, string, string, objectaction.Action, int, time.Time) (string, error)
	// CopyMultipartURL signs a multipart copy operation and returns its required headers.
	CopyMultipartURL(
		context.Context,
		string,
		string,
		string,
		string,
		string,
		objectaction.Action,
		int,
		int64,
		int64,
		time.Time,
	) (signedURL string, requiredHeaders map[string]string, err error)
}

// Request identifies an object operation and its parameters.
type Request struct {
	// BucketID identifies the selected Garage bucket.
	BucketID string
	// Action identifies the requested operation.
	Action objectaction.Action
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

// Result describes a validated S3 request for backend execution or explicit direct transfers.
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
	kind credential.Kind,
	input Request,
	now time.Time,
) (Result, error) {
	if !validObjectRequest(input) {
		return Result{}, ErrInvalid
	}

	if !kind.Valid() {
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
	kind credential.Kind,
	bucketID, key string,
	size int64,
	now time.Time,
) (Result, error) {
	if !kind.Valid() {
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

// sign dispatches a validated operation to its signer.
func (s *Service) sign(
	ctx context.Context,
	bucketID, alias string,
	input Request,
	now time.Time,
) (Result, error) {
	signer := s.signer
	method, valid := input.Action.Method()
	if !valid {
		return Result{}, errors.New("unsupported object operation")
	}
	result := Result{Method: method}
	var err error
	switch input.Action {
	case objectaction.List:
		result.URL, err = signer.ListPrefixURL(ctx, bucketID, alias, input.Prefix, input.Continuation, now)
	case objectaction.Download:
		result.URL, err = signer.DownloadURL(ctx, bucketID, alias, input.Key, now)
	case objectaction.Preview:
		result.URL, err = signer.PreviewURL(ctx, bucketID, alias, input.Key, now)
	case objectaction.Delete:
		result.URL, err = signer.DeleteURL(ctx, bucketID, alias, input.Key, now)
	case objectaction.Copy:
		result.URL, result.CopySource, err = signer.CopyURL(ctx, bucketID, alias, input.SourceKey, input.Key, now)
	case objectaction.MultipartStart, objectaction.MultipartPart, objectaction.MultipartComplete, objectaction.MultipartAbort:
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
	case objectaction.MultipartCopyStart,
		objectaction.MultipartCopyPart,
		objectaction.MultipartCopyComplete,
		objectaction.MultipartCopyAbort:
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
