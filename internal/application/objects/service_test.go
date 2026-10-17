package objects

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type signerStub struct{ called string }

type bucketStub struct {
	calls int
	err   error
}

func (b *bucketStub) ResolveBucket(_ context.Context, id string) (string, string, error) {
	b.calls++
	return id, "alias", b.err
}

func (s *signerStub) ListPrefixURL(context.Context, string, string, string, string, time.Time) (string, error) {
	s.called = "list"
	return "url", nil
}
func (s *signerStub) DownloadURL(context.Context, string, string, string, time.Time) (string, error) {
	s.called = "download"
	return "url", nil
}
func (s *signerStub) PreviewURL(context.Context, string, string, string, time.Time) (string, error) {
	s.called = "preview"
	return "url", nil
}
func (s *signerStub) DeleteURL(context.Context, string, string, string, time.Time) (string, error) {
	s.called = "delete"
	return "url", nil
}
func (s *signerStub) UploadURL(context.Context, string, string, string, time.Time) (string, error) {
	s.called = "upload"
	return "url", nil
}
func (s *signerStub) CopyURL(context.Context, string, string, string, string, time.Time) (string, string, error) {
	s.called = "copy"
	return "url", "source", nil
}
func (s *signerStub) MultipartURL(_ context.Context, _, _, _, _, action string, _ int, _ time.Time) (string, error) {
	s.called = action
	return "url", nil
}
func (s *signerStub) CopyMultipartURL(_ context.Context, _, _, _, _, _, action string, _ int, _, _ int64, _ time.Time) (string, map[string]string, error) {
	s.called = action
	return "url", map[string]string{"copy": "value"}, nil
}

func TestPrepare(t *testing.T) {
	for _, tc := range []struct{ action, method string }{
		{"list", "GET"}, {"download", "GET"}, {"preview", "GET"},
		{"delete", "DELETE"}, {"copy", "PUT"},
		{"multipart-start", "POST"}, {"multipart-part", "PUT"},
		{"multipart-complete", "POST"}, {"multipart-abort", "DELETE"},
		{"multipart-copy-start", "POST"}, {"multipart-copy-part", "PUT"},
		{"multipart-copy-complete", "POST"}, {"multipart-copy-abort", "DELETE"},
	} {
		t.Run(tc.action, func(t *testing.T) {
			signer := &signerStub{}
			service := New(&bucketStub{}, signer)
			result, err := service.sign(context.Background(), "bucket", "alias", Request{Action: tc.action}, time.Time{})
			require.NoError(t, err)
			assert.Equal(t, tc.action, signer.called)
			assert.Equal(t, tc.method, result.Method)
			assert.Equal(t, "url", result.URL)
		})
	}

	_, err := New(&bucketStub{}, &signerStub{}).Prepare(context.Background(), "admin", Request{Action: "unknown"}, time.Time{})
	require.Error(t, err)
}

func TestPrepareChecksBeforeResolving(t *testing.T) {
	bucket := &bucketStub{}
	signer := &signerStub{}
	service := New(bucket, signer)
	ctx := context.Background()
	_, err := service.Prepare(ctx, "viewer", Request{
		BucketID: "bucket",
		Action:   "delete",
		Key:      "file",
	}, time.Time{})
	require.ErrorIs(t, err, ErrForbidden)
	_, err = service.Prepare(ctx, "admin", Request{
		BucketID: "bucket",
		Action:   "delete",
		Key:      "../file",
	}, time.Time{})
	require.ErrorIs(t, err, ErrInvalid)
	_, err = service.PrepareUpload(ctx, "viewer", "bucket", "file", 1, time.Time{})
	require.ErrorIs(t, err, ErrForbidden)
	assert.Zero(t, bucket.calls)
	assert.Empty(t, signer.called)
	_, err = service.Prepare(ctx, "viewer", Request{
		BucketID: "bucket",
		Action:   "download",
		Key:      "file",
	}, time.Time{})
	require.NoError(t, err)
	assert.Equal(t, 1, bucket.calls)
	assert.Equal(t, "download", signer.called)
	_, err = service.PrepareUpload(ctx, "editor", "bucket", "file", 1, time.Time{})
	require.NoError(t, err)
	assert.Equal(t, "upload", signer.called)
	bucket.err = errors.New("Garage unavailable")
	_, err = service.Prepare(ctx, "admin", Request{
		BucketID: "bucket",
		Action:   "list",
	}, time.Time{})
	require.ErrorIs(t, err, ErrBucket)
}
