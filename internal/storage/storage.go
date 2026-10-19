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
	SaveSession(context.Context, []byte, []byte, time.Time) error
	LoadSession(context.Context, []byte) (credential []byte, theme string, err error)
	DeleteSession(context.Context, []byte) error
	SetSessionTheme(context.Context, []byte, string) error
}

// Store is the persistence required by the application, independently of SQL dialect.
type Store interface {
	Sessions
	signing.BucketLocker
	AllowLogin(context.Context, string) (bool, error)
	CleanupExpiredAuth(context.Context) error
	CleanupExpiredLoginAttempts(context.Context) error
	Ping(context.Context) error
	Close()
}
