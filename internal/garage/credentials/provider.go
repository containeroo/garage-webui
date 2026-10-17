// Package credentials provisions bucket-scoped Garage signing credentials.
package credentials

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/gi8lino/garage-webui/internal/garage"
	"github.com/gi8lino/garage-webui/internal/signing"
)

// Store serializes and persists bucket credential identifiers.
type Store interface {
	WithBucketSigningKey(context.Context, string, func(context.Context, string) (string, error)) error
	WithBucketReadKey(context.Context, string, func(context.Context, string) (string, error)) error
	WithBucketCopyKey(context.Context, string, func(context.Context, string) (string, error)) error
}

// Provider manages Garage keys, permissions and browser CORS.
type Provider struct {
	admin  *garage.Client
	store  Store
	origin string
}

// New constructs a provider for Garage signing credentials.
func New(admin *garage.Client, store Store, origin string) *Provider {
	return &Provider{
		admin:  admin,
		store:  store,
		origin: origin,
	}
}

// Copy obtains the separate bucket-scoped copy-only credential.
func (m *Provider) Copy(ctx context.Context, bucketID string) (signing.Credential, error) {
	var key signing.Credential
	err := m.store.WithBucketCopyKey(ctx, bucketID, func(ctx context.Context, id string) (string, error) {
		var err error
		key, err = m.managedKeyForPurpose(ctx, id, bucketID, "copy")
		if err != nil {
			return key.AccessKeyID, err
		}

		if err := m.changePermission(ctx, "AllowBucketKey", bucketID, key.AccessKeyID, map[string]bool{"read": true, "write": true}); err != nil {
			return key.AccessKeyID, err
		}

		if err := m.changePermission(ctx, "DenyBucketKey", bucketID, key.AccessKeyID, map[string]bool{"owner": true}); err != nil {
			return key.AccessKeyID, err
		}

		return key.AccessKeyID, m.ensureCORS(ctx, bucketID, http.MethodPut)
	})
	if err != nil {
		return signing.Credential{}, err
	}

	return key, nil
}

// Write obtains or creates the bucket-scoped write-only credential.
func (m *Provider) Write(ctx context.Context, bucketID, method string) (signing.Credential, error) {
	var key signing.Credential
	err := m.store.WithBucketSigningKey(ctx, bucketID, func(ctx context.Context, keyID string) (string, error) {
		var err error
		key, err = m.managedKey(ctx, keyID, bucketID, false)
		if err != nil {
			return key.AccessKeyID, err
		}

		if err := m.enforceWriteOnly(ctx, bucketID, key.AccessKeyID); err != nil {
			return key.AccessKeyID, err
		}

		return key.AccessKeyID, m.ensureCORS(ctx, bucketID, method)
	})
	if err != nil {
		return signing.Credential{}, err
	}

	return key, nil
}

// Read obtains or creates the bucket-scoped read-only credential.
func (m *Provider) Read(ctx context.Context, bucketID string) (signing.Credential, error) {
	var key signing.Credential
	err := m.store.WithBucketReadKey(ctx, bucketID, func(ctx context.Context, keyID string) (string, error) {
		var err error
		key, err = m.managedKey(ctx, keyID, bucketID, true)
		if err != nil {
			return key.AccessKeyID, err
		}

		if err := m.enforceReadOnly(ctx, bucketID, key.AccessKeyID); err != nil {
			return key.AccessKeyID, err
		}

		return key.AccessKeyID, m.ensureCORS(ctx, bucketID, http.MethodGet)
	})
	if err != nil {
		return signing.Credential{}, err
	}

	return key, nil
}

// signing.Credential retrieves an existing bucket-scoped key or creates a replacement.
func (m *Provider) managedKey(ctx context.Context, keyID, bucketID string, readOnly bool) (signing.Credential, error) {
	purpose := "upload"
	if readOnly {
		purpose = "read"
	}

	return m.managedKeyForPurpose(ctx, keyID, bucketID, purpose)
}

// CredentialForPurpose creates or loads a purpose-limited S3 credential.
func (m *Provider) managedKeyForPurpose(ctx context.Context, keyID, bucketID, purpose string) (signing.Credential, error) {
	if keyID != "" {
		key, missing, err := m.keyInfo(ctx, keyID)
		if err != nil {
			return signing.Credential{}, err
		}

		if !missing && usableKey(key) {
			return key, nil
		}
	}

	var key signing.Credential
	if err := m.adminJSON(ctx, http.MethodPost, "CreateKey", nil, map[string]string{
		"name": "garage-webui-" + purpose + "-" + bucketID,
	}, &key); err != nil {
		return signing.Credential{}, fmt.Errorf("create Garage S3 key: %w", err)
	}

	if key.AccessKeyID == "" {
		return signing.Credential{}, errors.New("garage returned an empty S3 key ID")
	}

	if key.SecretAccessKey == "" {
		createdID := key.AccessKeyID
		var missing bool
		var err error
		key, missing, err = m.keyInfo(ctx, createdID)
		if err != nil || missing || key.SecretAccessKey == "" {
			return signing.Credential{AccessKeyID: createdID}, errors.New("garage did not return a usable S3 secret")
		}
	}

	return key, nil
}

// usableKey checks that Garage returned a non-expired signing credential.
func usableKey(key signing.Credential) bool {
	return key.AccessKeyID != "" && key.SecretAccessKey != "" && !key.Expired
}

// keyInfo retrieves only the persisted bucket key's secret from the Admin API.
func (m *Provider) keyInfo(ctx context.Context, id string) (credential signing.Credential, missing bool, err error) {
	var key signing.Credential
	status, data, err := m.admin.Request(ctx, http.MethodGet, "GetKeyInfo", url.Values{
		"id":            {id},
		"showSecretKey": {"true"},
	}, nil)
	if err != nil {
		return key, false, err
	}

	if status == http.StatusNotFound {
		return key, true, nil
	}

	if status != http.StatusOK {
		return key, false, fmt.Errorf("GetKeyInfo returned HTTP %d", status)
	}

	if err := json.Unmarshal(data, &key); err != nil {
		return key, false, err
	}

	if key.AccessKeyID != id {
		return key, false, errors.New("garage returned a different key ID")
	}

	return key, false, nil
}

// enforceWriteOnly grants write permission and revokes read/owner permissions.
func (m *Provider) enforceWriteOnly(ctx context.Context, bucketID, keyID string) error {
	if err := m.changePermission(ctx, "AllowBucketKey", bucketID, keyID, map[string]bool{"write": true}); err != nil {
		return fmt.Errorf("grant object write permission: %w", err)
	}

	if err := m.changePermission(ctx, "DenyBucketKey", bucketID, keyID, map[string]bool{"read": true, "owner": true}); err != nil {
		return fmt.Errorf("remove non-write permissions: %w", err)
	}

	return nil
}

// enforceReadOnly grants read permission and revokes write/owner permissions.
func (m *Provider) enforceReadOnly(ctx context.Context, bucketID, keyID string) error {
	if err := m.changePermission(ctx, "AllowBucketKey", bucketID, keyID, map[string]bool{"read": true}); err != nil {
		return fmt.Errorf("grant object read permission: %w", err)
	}

	if err := m.changePermission(ctx, "DenyBucketKey", bucketID, keyID, map[string]bool{"write": true, "owner": true}); err != nil {
		return fmt.Errorf("remove non-read permissions: %w", err)
	}

	return nil
}

// changePermission changes only this bucket's specified key privileges.
func (m *Provider) changePermission(ctx context.Context, operation, bucketID, keyID string, permissions map[string]bool) error {
	return m.adminJSON(ctx, http.MethodPost, operation, nil, map[string]any{
		"bucketId": bucketID, "accessKeyId": keyID, "permissions": permissions,
	}, nil)
}

// adminJSON calls Garage's private Admin API without disclosing its token.
func (m *Provider) adminJSON(ctx context.Context, method, operation string, query url.Values, input, output any) error {
	var body io.Reader
	if input != nil {
		encoded, err := json.Marshal(input)
		if err != nil {
			return err
		}

		body = bytes.NewReader(encoded)
	}

	status, data, err := m.admin.Request(ctx, method, operation, query, body)
	if err != nil {
		return err
	}

	if status < 200 || status >= 300 {
		return fmt.Errorf("%s returned HTTP %d: %s", operation, status, strings.TrimSpace(string(data)))
	}

	if output != nil {
		return json.Unmarshal(data, output)
	}

	return nil
}
