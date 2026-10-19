// Package authentication implements account and authentication use cases.
package authentication

import (
	"errors"

	kit "github.com/containeroo/appkit/auth"
)

// ErrSetupComplete indicates that an administrator already exists.
var ErrSetupComplete = errors.New("setup is already complete")

// ErrInvalidSetup indicates invalid administrator credentials.
var ErrInvalidSetup = errors.New("username is required and password must be between 12 and 72 bytes")

// ErrRegistrationManaged indicates that registration is controlled by deployment configuration.
var ErrRegistrationManaged = errors.New("registration is managed by deployment configuration")

// Repository provides persistence required by account use cases.
type Repository interface {
	accountRepository
	groupRepository
	sessionRepository
}

// Service coordinates account and identity use cases.
type Service struct {
	accounts             accountRepository
	groups               groupRepository
	sessions             Repository
	mode                 kit.Mode
	localLogin           bool
	registrationOverride *bool
}

// New constructs an account service.
func New(store Repository, mode kit.Mode, localLogin bool, registrationOverride *bool) *Service {
	return &Service{
		accounts:             store,
		groups:               store,
		sessions:             store,
		mode:                 mode,
		localLogin:           localLogin,
		registrationOverride: registrationOverride,
	}
}

// validPassword reports whether a local password has an acceptable length.
func validPassword(password string) bool {
	return len(password) >= 12 &&
		len(password) <= 72
}
