package objects

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/containeroo/garage-webui/internal/credential"
	"github.com/containeroo/garage-webui/internal/objectaction"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type signerStub struct {
	called   objectaction.Action
	uploaded bool
}

type bucketStub struct {
	calls int
	err   error
}

func (b *bucketStub) ResolveBucket(_ context.Context, id string) (string, string, error) {
	b.calls++
	return id, "alias", b.err
}

func (s *signerStub) ListPrefixURL(context.Context, string, string, string, string, time.Time) (string, error) {
	s.called = objectaction.List
	return "url", nil
}
func (s *signerStub) DownloadURL(context.Context, string, string, string, time.Time) (string, error) {
	s.called = objectaction.Download
	return "url", nil
}
func (s *signerStub) PreviewURL(context.Context, string, string, string, time.Time) (string, error) {
	s.called = objectaction.Preview
	return "url", nil
}
func (s *signerStub) DeleteURL(context.Context, string, string, string, time.Time) (string, error) {
	s.called = objectaction.Delete
	return "url", nil
}
func (s *signerStub) UploadURL(context.Context, string, string, string, time.Time) (string, error) {
	s.uploaded = true
	return "url", nil
}
func (s *signerStub) CopyURL(context.Context, string, string, string, string, time.Time) (string, string, error) {
	s.called = objectaction.Copy
	return "url", "source", nil
}
func (s *signerStub) MultipartURL(_ context.Context, _, _, _, _ string, action objectaction.Action, _ int, _ time.Time) (string, error) {
	s.called = action
	return "url", nil
}
func (s *signerStub) CopyMultipartURL(_ context.Context, _, _, _, _, _ string, action objectaction.Action, _ int, _, _ int64, _ time.Time) (string, map[string]string, error) {
	s.called = action
	return "url", map[string]string{"copy": "value"}, nil
}

func TestPrepare(t *testing.T) {
	t.Parallel()

	t.Run("list", func(t *testing.T) {
		t.Parallel()

		signer := &signerStub{}
		service := New(&bucketStub{}, signer)
		result, err := service.sign(context.Background(), "bucket", "alias", Request{Action: objectaction.List}, time.Time{})
		require.NoError(t, err)
		assert.Equal(t, objectaction.List, signer.called)
		assert.Equal(t, "GET", result.Method)
		assert.Equal(t, "url", result.URL)
	})

	t.Run("download", func(t *testing.T) {
		t.Parallel()

		signer := &signerStub{}
		service := New(&bucketStub{}, signer)
		result, err := service.sign(context.Background(), "bucket", "alias", Request{Action: objectaction.Download}, time.Time{})
		require.NoError(t, err)
		assert.Equal(t, objectaction.Download, signer.called)
		assert.Equal(t, "GET", result.Method)
		assert.Equal(t, "url", result.URL)
	})

	t.Run("preview", func(t *testing.T) {
		t.Parallel()

		signer := &signerStub{}
		service := New(&bucketStub{}, signer)
		result, err := service.sign(context.Background(), "bucket", "alias", Request{Action: objectaction.Preview}, time.Time{})
		require.NoError(t, err)
		assert.Equal(t, objectaction.Preview, signer.called)
		assert.Equal(t, "GET", result.Method)
		assert.Equal(t, "url", result.URL)
	})

	t.Run("delete", func(t *testing.T) {
		t.Parallel()

		signer := &signerStub{}
		service := New(&bucketStub{}, signer)
		result, err := service.sign(context.Background(), "bucket", "alias", Request{Action: objectaction.Delete}, time.Time{})
		require.NoError(t, err)
		assert.Equal(t, objectaction.Delete, signer.called)
		assert.Equal(t, "DELETE", result.Method)
		assert.Equal(t, "url", result.URL)
	})

	t.Run("copy", func(t *testing.T) {
		t.Parallel()

		signer := &signerStub{}
		service := New(&bucketStub{}, signer)
		result, err := service.sign(context.Background(), "bucket", "alias", Request{Action: objectaction.Copy}, time.Time{})
		require.NoError(t, err)
		assert.Equal(t, objectaction.Copy, signer.called)
		assert.Equal(t, "PUT", result.Method)
		assert.Equal(t, "url", result.URL)
	})

	t.Run("multipart-start", func(t *testing.T) {
		t.Parallel()

		signer := &signerStub{}
		service := New(&bucketStub{}, signer)
		result, err := service.sign(context.Background(), "bucket", "alias", Request{Action: objectaction.MultipartStart}, time.Time{})
		require.NoError(t, err)
		assert.Equal(t, objectaction.MultipartStart, signer.called)
		assert.Equal(t, "POST", result.Method)
		assert.Equal(t, "url", result.URL)
	})

	t.Run("multipart-part", func(t *testing.T) {
		t.Parallel()

		signer := &signerStub{}
		service := New(&bucketStub{}, signer)
		result, err := service.sign(context.Background(), "bucket", "alias", Request{Action: objectaction.MultipartPart}, time.Time{})
		require.NoError(t, err)
		assert.Equal(t, objectaction.MultipartPart, signer.called)
		assert.Equal(t, "PUT", result.Method)
		assert.Equal(t, "url", result.URL)
	})

	t.Run("multipart-complete", func(t *testing.T) {
		t.Parallel()

		signer := &signerStub{}
		service := New(&bucketStub{}, signer)
		result, err := service.sign(context.Background(), "bucket", "alias", Request{Action: objectaction.MultipartComplete}, time.Time{})
		require.NoError(t, err)
		assert.Equal(t, objectaction.MultipartComplete, signer.called)
		assert.Equal(t, "POST", result.Method)
		assert.Equal(t, "url", result.URL)
	})

	t.Run("multipart-abort", func(t *testing.T) {
		t.Parallel()

		signer := &signerStub{}
		service := New(&bucketStub{}, signer)
		result, err := service.sign(context.Background(), "bucket", "alias", Request{Action: objectaction.MultipartAbort}, time.Time{})
		require.NoError(t, err)
		assert.Equal(t, objectaction.MultipartAbort, signer.called)
		assert.Equal(t, "DELETE", result.Method)
		assert.Equal(t, "url", result.URL)
	})

	t.Run("multipart-copy-start", func(t *testing.T) {
		t.Parallel()

		signer := &signerStub{}
		service := New(&bucketStub{}, signer)
		result, err := service.sign(context.Background(), "bucket", "alias", Request{Action: objectaction.MultipartCopyStart}, time.Time{})
		require.NoError(t, err)
		assert.Equal(t, objectaction.MultipartCopyStart, signer.called)
		assert.Equal(t, "POST", result.Method)
		assert.Equal(t, "url", result.URL)
	})

	t.Run("multipart-copy-part", func(t *testing.T) {
		t.Parallel()

		signer := &signerStub{}
		service := New(&bucketStub{}, signer)
		result, err := service.sign(context.Background(), "bucket", "alias", Request{Action: objectaction.MultipartCopyPart}, time.Time{})
		require.NoError(t, err)
		assert.Equal(t, objectaction.MultipartCopyPart, signer.called)
		assert.Equal(t, "PUT", result.Method)
		assert.Equal(t, "url", result.URL)
	})

	t.Run("multipart-copy-complete", func(t *testing.T) {
		t.Parallel()

		signer := &signerStub{}
		service := New(&bucketStub{}, signer)
		result, err := service.sign(context.Background(), "bucket", "alias", Request{Action: objectaction.MultipartCopyComplete}, time.Time{})
		require.NoError(t, err)
		assert.Equal(t, objectaction.MultipartCopyComplete, signer.called)
		assert.Equal(t, "POST", result.Method)
		assert.Equal(t, "url", result.URL)
	})

	t.Run("multipart-copy-abort", func(t *testing.T) {
		t.Parallel()

		signer := &signerStub{}
		service := New(&bucketStub{}, signer)
		result, err := service.sign(context.Background(), "bucket", "alias", Request{Action: objectaction.MultipartCopyAbort}, time.Time{})
		require.NoError(t, err)
		assert.Equal(t, objectaction.MultipartCopyAbort, signer.called)
		assert.Equal(t, "DELETE", result.Method)
		assert.Equal(t, "url", result.URL)
	})

	t.Run("reject unsupported preparation", func(t *testing.T) {
		t.Parallel()

		_, err := New(&bucketStub{}, &signerStub{}).Prepare(context.Background(), credential.Admin, Request{Action: "unknown"}, time.Time{})
		require.Error(t, err)
	})
}

func TestSignRejectsUnknownAction(t *testing.T) {
	t.Parallel()

	t.Run("empty action", func(t *testing.T) {
		t.Parallel()

		action := objectaction.Action("")
		signer := &signerStub{}
		_, err := New(&bucketStub{}, signer).sign(context.Background(), "bucket", "alias", Request{Action: action}, time.Time{})
		require.Error(t, err)
		assert.Empty(t, signer.called)
	})

	t.Run("unsupported", func(t *testing.T) {
		t.Parallel()

		action := objectaction.Action("unsupported")
		signer := &signerStub{}
		_, err := New(&bucketStub{}, signer).sign(context.Background(), "bucket", "alias", Request{Action: action}, time.Time{})
		require.Error(t, err)
		assert.Empty(t, signer.called)
	})

	t.Run("multipart-unknown", func(t *testing.T) {
		t.Parallel()

		action := objectaction.Action("multipart-unknown")
		signer := &signerStub{}
		_, err := New(&bucketStub{}, signer).sign(context.Background(), "bucket", "alias", Request{Action: action}, time.Time{})
		require.Error(t, err)
		assert.Empty(t, signer.called)
	})
}

func TestPrepareChecksBeforeResolving(t *testing.T) {
	t.Parallel()

	bucket := &bucketStub{}
	signer := &signerStub{}
	service := New(bucket, signer)
	ctx := context.Background()
	_, err := service.Prepare(ctx, "unknown", Request{
		BucketID: "bucket",
		Action:   objectaction.Delete,
		Key:      "file",
	}, time.Time{})
	require.ErrorIs(t, err, ErrForbidden)
	_, err = service.Prepare(ctx, credential.Admin, Request{
		BucketID: "bucket",
		Action:   objectaction.Delete,
		Key:      "../file",
	}, time.Time{})
	require.ErrorIs(t, err, ErrInvalid)
	_, err = service.PrepareUpload(ctx, "unknown", "bucket", "file", 1, time.Time{})
	require.ErrorIs(t, err, ErrForbidden)
	assert.Zero(t, bucket.calls)
	assert.Empty(t, signer.called)
	_, err = service.Prepare(ctx, credential.S3, Request{
		BucketID: "bucket",
		Action:   objectaction.Download,
		Key:      "file",
	}, time.Time{})
	require.NoError(t, err)
	assert.Equal(t, 1, bucket.calls)
	assert.Equal(t, objectaction.Download, signer.called)
	_, err = service.PrepareUpload(ctx, credential.S3, "bucket", "file", 1, time.Time{})
	require.NoError(t, err)
	assert.True(t, signer.uploaded)
	bucket.err = errors.New("garage unavailable")
	_, err = service.Prepare(ctx, credential.Admin, Request{
		BucketID: "bucket",
		Action:   objectaction.List,
	}, time.Time{})
	require.ErrorIs(t, err, ErrBucket)
}
