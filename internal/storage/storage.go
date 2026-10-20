// Package storage defines the database-independent persistence contracts.
package storage

import (
	"context"
	"errors"
	"time"

	"github.com/containeroo/garage-webui/internal/signing"
)

// ErrSessionNotFound indicates a missing, expired or revoked browser session.
var ErrSessionNotFound = errors.New("session not found")

// Sessions persists encrypted Garage credentials and appearance preferences.
type Sessions interface {
	// SaveSession stores encrypted credentials under the session hash until expiry.
	SaveSession(context.Context, []byte, []byte, time.Time) error
	// LoadSession loads the session credentials and theme by session hash.
	LoadSession(context.Context, []byte) (credential []byte, theme string, err error)
	// DeleteSession removes the session identified by its hash.
	DeleteSession(context.Context, []byte) error
	// SetSessionTheme updates the theme of the session identified by its hash.
	SetSessionTheme(context.Context, []byte, string) error
}

// Store is the persistence required by the application, independently of SQL dialect.
type Store interface {
	Sessions
	signing.BucketLocker
	// AllowLogin records a login attempt and reports whether the peer remains within its budget.
	AllowLogin(context.Context, string) (bool, error)
	// CleanupExpiredAuth removes expired authentication sessions.
	CleanupExpiredAuth(context.Context) error
	// CleanupExpiredLoginAttempts removes expired login attempt counters.
	CleanupExpiredLoginAttempts(context.Context) error
	// Ping checks whether persistent storage is available.
	Ping(context.Context) error
	// Close releases the persistent storage resources.
	Close()
}
