// Package endpoint implements HTTP handlers and the contracts they consume.
package endpoint

import (
	"context"
	"github.com/containeroo/garage-webui/internal/application/objects"
	"github.com/containeroo/garage-webui/internal/http/auth"
)

// ObjectServices prepares credential-scoped S3 operations and bucket deletion.
type ObjectServices interface {
	// ForIdentity creates object services scoped to the authenticated identity.
	ForIdentity(auth.Identity) (*objects.Service, error)
	// ForDirectIdentity prepares optional public signed transfers.
	ForDirectIdentity(auth.Identity) (*objects.Service, error)
	// DeleteBucket deletes the requested bucket.
	DeleteBucket(context.Context, auth.Identity, string) (int, []byte, error)
}
