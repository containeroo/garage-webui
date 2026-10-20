package s3

import (
	"context"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/containeroo/garage-webui/internal/objectaction"
	"github.com/containeroo/garage-webui/internal/objectpath"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestInvalidMultipartDoesNotProvision checks validation at the service boundary.
func TestInvalidMultipartDoesNotProvision(t *testing.T) {
	state := newFakeGarage()
	store := &memoryKeyStore{}
	manager := managerForTest(t, state, store)
	ctx, now := context.Background(), time.Now()

	for _, input := range []struct {
		action, uploadID string
		part             int
	}{
		{"unknown", "", 0},
		{"multipart-start", "unexpected", 0},
		{"multipart-part", "", 1},
		{"multipart-part", "upload", 10001},
		{"multipart-complete", "upload", 1},
	} {
		_, err := manager.MultipartURL(ctx, "bucket", "alias", "object", input.uploadID, objectaction.Action(input.action), input.part, now)
		require.Error(t, err)
	}

	for _, input := range []struct {
		action, uploadID string
		part             int
		start, end       int64
	}{
		{"unknown", "", 0, 0, 0},
		{"multipart-copy-start", "unexpected", 0, 0, 0},
		{"multipart-copy-part", "upload", 1, 5, 4},
		{"multipart-copy-part", "upload", 1, 0, MaxUploadSize},
		{"multipart-copy-complete", "upload", 2, 0, 0},
	} {
		_, _, err := manager.CopyMultipartURL(ctx, "bucket", "alias", "source", "target", input.uploadID, objectaction.Action(input.action), input.part, input.start, input.end, now)
		require.Error(t, err)
	}

	assert.Zero(t, state.createCount)
	assert.Zero(t, state.allowCount)
	assert.Empty(t, state.cors)
	assert.Empty(t, store.ids)
	assert.Empty(t, store.copyIDs)
}

// TestMultipartAndCopySigning checks S3 operation scoping and signed copy headers.
func TestMultipartAndCopySigning(t *testing.T) {
	signer, err := New("http://localhost:3900", "garage", "GKtest", "a-secret", "http://localhost:8080")
	if err != nil {
		t.Fatal(err)
	}

	now := time.Date(2026, 10, 17, 18, 0, 0, 0, time.UTC)
	begin, err := signer.BeginMultipartURL("bucket", "folder/archive.tar", now)
	if err != nil {
		t.Fatal(err)
	}

	u, err := url.Parse(begin)
	if err != nil {
		t.Fatal(err)
	}

	if _, ok := u.Query()["uploads"]; !ok {
		t.Fatal("missing multipart start parameter")
	}

	if u.Query().Get("X-Amz-Expires") != "300" {
		t.Fatal("wrong URL lifetime")
	}

	partURL, err := signer.PartURL("bucket", "folder/archive.tar", "opaque=+id", 10000, now)
	if err != nil {
		t.Fatal(err)
	}

	parsed, err := url.Parse(partURL)
	if err != nil {
		t.Fatal(err)
	}

	if parsed.Query().Get("partNumber") != "10000" || parsed.Query().Get("uploadId") != "opaque=+id" {
		t.Fatal("incorrect part scope")
	}

	if _, err := signer.PartURL("bucket", "archive", "id", 10001, now); err == nil {
		t.Fatal("accepted too many parts")
	}

	copyURL, header, err := signer.CopyURL("bucket", "folder/source #1.txt", "folder/target.txt", now)
	if err != nil {
		t.Fatal(err)
	}

	parsed, err = url.Parse(copyURL)
	if err != nil {
		t.Fatal(err)
	}

	if parsed.Query().Get("X-Amz-SignedHeaders") != "host;x-amz-copy-source" {
		t.Fatal("copy source header not signed")
	}

	if !strings.Contains(header, "source%20%231.txt") {
		t.Fatalf("copy source not escaped: %s", header)
	}

	partCopyURL, headers, err := signer.CopyPartURL("bucket", "old", "new", "upload", 1, 0, 16<<20-1, now)
	if err != nil {
		t.Fatal(err)
	}

	parsed, err = url.Parse(partCopyURL)
	if err != nil {
		t.Fatal(err)
	}

	if parsed.Query().Get("X-Amz-SignedHeaders") != "host;x-amz-copy-source;x-amz-copy-source-range" {
		t.Fatal("missing signed copy range")
	}

	if headers["x-amz-copy-source-range"] != "bytes=0-16777215" {
		t.Fatal("incorrect signed copy range")
	}

	if _, _, err := signer.CopyPartURL("bucket", "old", "new", "upload", 1, 2, 1, now); err == nil {
		t.Fatal("accepted reversed range")
	}
}

// TestSeparateCopyCredential checks that copies never reuse read-only or write-only keys.
func TestSeparateCopyCredential(t *testing.T) {
	manager := managerForTest(t, newFakeGarage(), &memoryKeyStore{})
	ctx, now := context.Background(), time.Now()
	read, err := manager.PreviewURL(ctx, "bucket-a", "alias", "item.txt", now)
	if err != nil {
		t.Fatal(err)
	}

	write, err := manager.UploadURL(ctx, "bucket-a", "alias", "item.txt", now)
	if err != nil {
		t.Fatal(err)
	}

	copyURL, _, err := manager.CopyURL(ctx, "bucket-a", "alias", "item.txt", "item-2.txt", now)
	if err != nil {
		t.Fatal(err)
	}

	credential := func(address string) string {
		parsed, err := url.Parse(address)
		if err != nil {
			t.Fatal(err)
		}

		return strings.Split(parsed.Query().Get("X-Amz-Credential"), "/")[0]
	}
	if credential(read) == credential(write) || credential(copyURL) == credential(read) || credential(copyURL) == credential(write) {
		t.Fatal("privileges are not isolated across the three per-bucket keys")
	}
}

// TestListPrefixValidation covers folder prefixes and traversal rejection.
func TestListPrefixValidation(t *testing.T) {
	for _, prefix := range []string{"", "pictures/", "pictures/cats", "météo/"} {
		if !objectpath.ValidPrefix(prefix) {
			t.Fatalf("rejected valid prefix %q", prefix)
		}
	}

	for _, prefix := range []string{"/root", "../escape", "foo/../bar", "foo\\bar", "bad\nkey"} {
		if objectpath.ValidPrefix(prefix) {
			t.Fatalf("accepted unsafe prefix %q", prefix)
		}
	}

	signer, err := New("http://localhost:3900", "garage", "key", "secret", "http://localhost:8080")
	if err != nil {
		t.Fatal(err)
	}

	result, err := signer.ListPrefixURL("bucket", "folder/", "", time.Now())
	if err != nil {
		t.Fatal(err)
	}

	address, err := url.Parse(result)
	if err != nil {
		t.Fatal(err)
	}

	if address.Query().Get("prefix") != "folder/" || address.Query().Get("delimiter") != "/" {
		t.Fatal("listing is not folder-scoped")
	}
}
