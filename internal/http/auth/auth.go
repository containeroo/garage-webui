// Package auth authenticates Garage credentials and manages revocable browser sessions.
package auth

import (
	"crypto/aes"
	"crypto/cipher"
	"encoding/base64"
	"errors"
	"net/url"
	"slices"
	"time"

	"github.com/containeroo/garage-webui/internal/garage/admin"
	"github.com/containeroo/garage-webui/internal/storage"
	"github.com/containeroo/garage-webui/internal/utils"
)

// ErrUnauthenticated means Garage rejected the credential or the session is invalid.
var ErrUnauthenticated = errors.New("invalid Garage credentials")

const (
	cookieName      = "garage_webui_session"
	sessionLifetime = 8 * time.Hour
)

// Credential is encrypted at rest and is never included in session responses.
type Credential struct {
	Kind            string `json:"kind"`
	Token           string `json:"token,omitempty"`
	AccessKeyID     string `json:"accessKeyId,omitempty"`
	SecretAccessKey string `json:"secretAccessKey,omitempty"`
}

// Identity contains fresh Garage metadata and the session's credential.
type Identity struct {
	Credential Credential
	User       string
	Scope      []string
	Theme      string
}

// Allows checks the current Garage admin-token scope.
func (id Identity) Allows(operation string) bool {
	return id.Credential.Kind == "admin" && (slices.Contains(id.Scope, "*") || slices.Contains(id.Scope, operation))
}

// Service owns only deployment endpoints and session encryption, never a shared Garage credential.
type Service struct {
	store      storage.Sessions
	encryption cipher.AEAD
	GarageURL  string
	S3URL      string
	Region     string
	publicURL  string
	prefix     string
	secure     bool
}

// Config describes deployment endpoints and the credential encryption key.
type Config struct {
	PublicURL     string
	RoutePrefix   string
	GarageURL     string
	S3URL         string
	Region        string
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

	origin, err := url.Parse(cfg.PublicURL)
	if err != nil || !validPublicOrigin(origin) {
		return nil, errors.New("public-url must be an HTTP(S) origin")
	}

	return &Service{
		store:      store,
		encryption: aead,
		GarageURL:  cfg.GarageURL,
		S3URL:      cfg.S3URL,
		Region:     cfg.Region,
		publicURL:  cfg.PublicURL,
		prefix:     cfg.RoutePrefix,
		secure:     origin.Scheme == "https",
	}, nil
}

// hasNoCredentials prüft, ob keine Benutzerdaten (User/Password) hinterlegt sind.
func hasNoCredentials(origin *url.URL) bool {
	return origin.User == nil
}

// hasNoURLExtras prüft, ob die URL frei von Query-Parametern und Fragmenten ist.
func hasNoURLExtras(origin *url.URL) bool {
	return origin.RawQuery == "" && !origin.ForceQuery && origin.Fragment == ""
}

// hasNoPathComponents prüft, ob der Pfad leer oder lediglich der Root-Pfad "/" ist.
func hasNoPathComponents(origin *url.URL) bool {
	return origin.Path == "" || origin.Path == "/"
}

// validPublicOrigin akzeptiert einen Origin ohne Credentials, einen Pfad oder URL-Extras.
func validPublicOrigin(origin *url.URL) bool {
	if origin == nil || origin.Host == "" {
		return false
	}

	return utils.HasValidScheme(origin) &&
		hasNoCredentials(origin) &&
		hasNoURLExtras(origin) &&
		hasNoPathComponents(origin)
}
