// Package auth verifies Garage credentials and persists encrypted, revocable sessions.
package auth

import (
	"crypto/aes"
	"crypto/cipher"
	"encoding/base64"
	"errors"
	"github.com/containeroo/garage-webui/internal/garage/admin"
	"github.com/containeroo/garage-webui/internal/storage"
	"time"
)

// ErrUnauthenticated means a credential was rejected or a session is invalid.
var ErrUnauthenticated = errors.New("invalid Garage credentials")

// SessionLifetime is the validity period of a newly created Garage browser session.
const SessionLifetime = 8 * time.Hour

// Service verifies Garage credentials and stores encrypted sessions without HTTP dependencies.
type Service struct {
	// store persists encrypted authentication sessions.
	store storage.Sessions
	// encryption encrypts and authenticates stored credentials.
	encryption cipher.AEAD
	// GarageURL specifies the Garage Admin API endpoint.
	GarageURL string
	// S3URL specifies the server-side S3 endpoint.
	S3URL string
	// S3PublicURL specifies the browser-accessible S3 endpoint.
	S3PublicURL string
	// Region specifies the S3 signing region.
	Region string
}

// Config describes deployment endpoints and the credential encryption key.
type Config struct {
	// GarageURL specifies the Garage Admin API endpoint.
	GarageURL string
	// S3URL specifies the server-side S3 endpoint.
	S3URL string
	// S3PublicURL specifies the browser-accessible S3 endpoint.
	S3PublicURL string
	// Region specifies the S3 signing region.
	Region string
	// SessionSecret provides the secret used to derive session encryption keys.
	SessionSecret string
}

// New validates deployment configuration and prepares session encryption.
func New(store storage.Sessions, cfg Config) (*Service, error) {
	if store == nil {
		return nil, errors.New("session storage is required")
	}

	key, err := base64.StdEncoding.DecodeString(cfg.SessionSecret)
	if err != nil || len(key) != 32 {
		return nil, errors.New("session-secret must be a base64-encoded 32-byte key")
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	if _, err := admin.New(cfg.GarageURL, ""); err != nil {
		return nil, err
	}

	return &Service{
		store:       store,
		encryption:  aead,
		GarageURL:   cfg.GarageURL,
		S3URL:       cfg.S3URL,
		S3PublicURL: cfg.S3PublicURL,
		Region:      cfg.Region,
	}, nil
}
