package s3

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/containeroo/garage-webui/internal/objectaction"
	"github.com/containeroo/garage-webui/internal/objectpath"
	"github.com/containeroo/garage-webui/internal/signing"
)

// CredentialProvider obtains purpose-limited bucket credentials.
type CredentialProvider interface {
	// Write provides credentials for writing to the target bucket.
	Write(context.Context, string, string) (signing.Credential, error)
	// Read provides credentials for reading from the target bucket.
	Read(context.Context, string) (signing.Credential, error)
	// Copy provides credentials for copying objects in the target bucket.
	Copy(context.Context, string) (signing.Credential, error)
}

// Manager signs validated S3 operations for server-side or optional direct transfers.
type Manager struct {
	// keys provides bucket-scoped signing credentials.
	keys CredentialProvider
	// public specifies the selected S3 endpoint.
	public string
	// region specifies the S3 signing region.
	region string
	// origin specifies the browser-facing application origin.
	origin string
}

// NewManager constructs a manager, or returns nil when the S3 URL is absent.
func NewManager(keys CredentialProvider, publicURL, region, origin string) (*Manager, error) {
	if publicURL == "" {
		return nil, nil
	}

	if _, err := New(publicURL, region, "validation", "validation", origin); err != nil {
		return nil, err
	}

	if keys == nil {
		return nil, errors.New("managed S3 operations require a credential provider")
	}

	return &Manager{
		keys:   keys,
		public: publicURL,
		region: region,
		origin: origin,
	}, nil
}

// Origin returns the browser-reachable S3 origin for Content Security Policy.
func (m *Manager) Origin() string { return strings.TrimSuffix(m.public, "/") }

// UploadURL signs an object-scoped PUT using the bucket's write-only key.
func (m *Manager) UploadURL(ctx context.Context, bucketID, alias, objectKey string, now time.Time) (string, error) {
	if !validUploadTarget(bucketID, alias, objectKey) {
		return "", errors.New("invalid bucket or object key")
	}

	signer, err := m.writeSigner(ctx, bucketID, http.MethodPut)
	if err != nil {
		return "", err
	}

	return signer.UploadURL(alias, objectKey, now)
}

// DeleteURL signs an object-scoped DELETE using the bucket's write-only key.
func (m *Manager) DeleteURL(ctx context.Context, bucketID, alias, objectKey string, now time.Time) (string, error) {
	if !validUploadTarget(bucketID, alias, objectKey) {
		return "", errors.New("invalid bucket or object key")
	}

	signer, err := m.writeSigner(ctx, bucketID, http.MethodDelete)
	if err != nil {
		return "", err
	}

	return signer.DeleteURL(alias, objectKey, now)
}

// DownloadURL signs an object-scoped attachment download with the read-only key.
func (m *Manager) DownloadURL(ctx context.Context, bucketID, alias, objectKey string, now time.Time) (string, error) {
	if !validUploadTarget(bucketID, alias, objectKey) {
		return "", errors.New("invalid bucket or object key")
	}

	signer, err := m.readSigner(ctx, bucketID)
	if err != nil {
		return "", err
	}

	return signer.DownloadURL(alias, objectKey, now)
}

// ListURL signs one page of a bucket's objects using its read-only key.
func (m *Manager) ListURL(ctx context.Context, bucketID, alias, continuation string, now time.Time) (string, error) {
	if bucketID == "" || !validBucket(alias) || !objectpath.ValidContinuationToken(continuation) {
		return "", errors.New("invalid bucket or continuation token")
	}

	signer, err := m.readSigner(ctx, bucketID)
	if err != nil {
		return "", err
	}

	return signer.ListURL(alias, continuation, now)
}

// PreviewURL signs inline viewing with the bucket-scoped read key.
func (m *Manager) PreviewURL(ctx context.Context, bucketID, alias, key string, now time.Time) (string, error) {
	if !validUploadTarget(bucketID, alias, key) {
		return "", errors.New("invalid preview target")
	}

	signer, err := m.readSigner(ctx, bucketID)
	if err != nil {
		return "", err
	}

	return signer.PreviewURL(alias, key, now)
}

// ListPrefixURL lists a folder or key prefix in an authorized bucket.
func (m *Manager) ListPrefixURL(
	ctx context.Context,
	bucketID, alias, prefix, continuation string,
	now time.Time,
) (string, error) {
	if bucketID == "" || !validBucket(alias) || !objectpath.ValidPrefix(prefix) || !objectpath.ValidContinuationToken(continuation) {
		return "", errors.New("invalid list target")
	}

	signer, err := m.readSigner(ctx, bucketID)
	if err != nil {
		return "", err
	}

	return signer.ListPrefixURL(alias, prefix, continuation, now)
}

// MultipartURL signs the next step of a multipart upload as it is needed.
func (m *Manager) MultipartURL(
	ctx context.Context,
	bucketID, alias, key, uploadID string,
	action objectaction.Action,
	part int,
	now time.Time,
) (string, error) {
	if !validUploadTarget(bucketID, alias, key) {
		return "", errors.New("invalid multipart target")
	}

	method, supported := action.Method()
	if !supported || !validMultipartUpload(action, uploadID, part) {
		return "", errors.New("invalid multipart operation")
	}

	signer, err := m.writeSigner(ctx, bucketID, method)
	if err != nil {
		return "", err
	}

	switch action {
	case objectaction.MultipartStart:
		return signer.BeginMultipartURL(alias, key, now)
	case objectaction.MultipartPart:
		return signer.PartURL(alias, key, uploadID, part, now)
	case objectaction.MultipartComplete:
		return signer.CompleteMultipartURL(alias, key, uploadID, now)
	case objectaction.MultipartAbort:
		return signer.AbortMultipartURL(alias, key, uploadID, now)
	default:
		return "", errors.New("invalid multipart operation")
	}
}

// validMultipartUpload validates upload parameters before any key or CORS changes.
func validMultipartUpload(action objectaction.Action, uploadID string, part int) bool {
	switch action {
	case objectaction.MultipartStart:
		return uploadID == "" && part == 0
	case objectaction.MultipartPart:
		return validUploadID(uploadID) && part >= 1 && part <= 10000
	case objectaction.MultipartComplete, objectaction.MultipartAbort:
		return validUploadID(uploadID) && part == 0
	default:
		return false
	}
}

// CopyURL uses a bucket-only read/write key to sign a server-side copy.
func (m *Manager) CopyURL(
	ctx context.Context,
	bucketID, alias, source, target string,
	now time.Time,
) (signedURL, copySource string, err error) {
	if !validUploadTarget(bucketID, alias, source) || !objectpath.ValidObjectKey(target) || source == target {
		return "", "", errors.New("invalid copy operation")
	}

	signer, err := m.copySigner(ctx, bucketID)
	if err != nil {
		return "", "", err
	}

	return signer.CopyURL(alias, source, target, now)
}

// CopyMultipartURL signs an operation for large same-bucket server-side copies.
func (m *Manager) CopyMultipartURL(
	ctx context.Context,
	bucketID, alias, source, target, uploadID string,
	action objectaction.Action,
	part int,
	start, end int64,
	now time.Time,
) (signedURL string, requiredHeaders map[string]string, err error) {
	if !validUploadTarget(bucketID, alias, source) || !objectpath.ValidObjectKey(target) || source == target {
		return "", nil, errors.New("invalid multipart copy target")
	}

	if !validMultipartCopy(action, uploadID, part, start, end) {
		return "", nil, errors.New("invalid multipart copy operation")
	}

	signer, err := m.copySigner(ctx, bucketID)
	if err != nil {
		return "", nil, err
	}

	switch action {
	case objectaction.MultipartCopyStart:
		url, err := signer.BeginMultipartURL(alias, target, now)
		return url, nil, err
	case objectaction.MultipartCopyPart:
		return signer.CopyPartURL(alias, source, target, uploadID, part, start, end, now)
	case objectaction.MultipartCopyComplete:
		url, err := signer.CompleteMultipartURL(alias, target, uploadID, now)
		return url, nil, err
	case objectaction.MultipartCopyAbort:
		url, err := signer.AbortMultipartURL(alias, target, uploadID, now)
		return url, nil, err
	default:
		return "", nil, errors.New("unsupported copy operation")
	}
}

// validMultipartCopy validates copy parameters before provisioning credentials.
func validMultipartCopy(action objectaction.Action, uploadID string, part int, start, end int64) bool {
	switch action {
	case objectaction.MultipartCopyStart:
		return uploadID == "" && part == 0 && start == 0 && end == 0
	case objectaction.MultipartCopyPart:
		return validUploadID(uploadID) && part >= 1 && part <= 10000 &&
			start >= 0 && end >= start && end-start < MaxUploadSize
	case objectaction.MultipartCopyComplete, objectaction.MultipartCopyAbort:
		return validUploadID(uploadID) && part == 0 && start == 0 && end == 0
	default:
		return false
	}
}

// validUploadTarget checks an authenticated bucket selection and target path.
func validUploadTarget(bucketID, alias, objectKey string) bool {
	return bucketID != "" && validBucket(alias) && objectpath.ValidObjectKey(objectKey)
}

// writeSigner obtains a write key and constructs an S3 signer.
func (m *Manager) writeSigner(ctx context.Context, bucketID, method string) (*Presigner, error) {
	key, err := m.keys.Write(ctx, bucketID, method)
	if err != nil {
		return nil, err
	}

	return m.presigner(key)
}

// readSigner obtains a read key and constructs an S3 signer.
func (m *Manager) readSigner(ctx context.Context, bucketID string) (*Presigner, error) {
	key, err := m.keys.Read(ctx, bucketID)
	if err != nil {
		return nil, err
	}

	return m.presigner(key)
}

// copySigner obtains a copy key and constructs an S3 signer.
func (m *Manager) copySigner(ctx context.Context, bucketID string) (*Presigner, error) {
	key, err := m.keys.Copy(ctx, bucketID)
	if err != nil {
		return nil, err
	}

	return m.presigner(key)
}

// presigner constructs SigV4 signing without exposing the key to the browser.
func (m *Manager) presigner(key signing.Credential) (*Presigner, error) {
	return New(
		m.public,
		m.region,
		key.AccessKeyID,
		key.SecretAccessKey,
		m.origin,
	)
}
