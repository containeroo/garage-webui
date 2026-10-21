// Package auth adapts core Garage authentication to browser sessions and cookies.
package auth

import (
	"errors"
	"net/url"

	core "github.com/containeroo/garage-webui/internal/auth"
	"github.com/containeroo/garage-webui/internal/storage"
	"github.com/containeroo/garage-webui/internal/utils"
)

// Credential describes a Garage secret used to authenticate a browser session.
type Credential = core.Credential

// Identity contains the current Garage identity and session theme.
type Identity = core.Identity

// ErrUnauthenticated reports rejected credentials or an invalid browser session.
var ErrUnauthenticated = core.ErrUnauthenticated

const cookieName = "garage_webui_session"

// Service exposes core authentication and HTTP session behavior.
type Service struct {
	// Service provides Garage credential verification and encrypted session persistence.
	*core.Service

	// prefix is the deployment-relative browser cookie path.
	prefix string
	// secure requires HTTPS-only session cookies.
	secure bool
}

// Config specifies Garage endpoints and browser-session settings.
type Config struct {
	// PublicURL specifies the externally visible web console origin.
	PublicURL string
	// RoutePrefix specifies the deployment URL path prefix.
	RoutePrefix string
	// GarageURL specifies the Garage Admin API endpoint.
	GarageURL string
	// S3URL specifies the server-side Garage S3 endpoint.
	S3URL string
	// S3PublicURL specifies the browser-reachable Garage S3 endpoint.
	S3PublicURL string
	// Region specifies the Garage S3 signing region.
	Region string
	// SessionSecret encrypts session credentials stored in the database.
	SessionSecret string
}

// New validates browser configuration and creates a core authentication service.
func New(store storage.Sessions, cfg Config) (*Service, error) {
	coreService, err := core.New(store, core.Config{
		GarageURL:     cfg.GarageURL,
		S3URL:         cfg.S3URL,
		S3PublicURL:   cfg.S3PublicURL,
		Region:        cfg.Region,
		SessionSecret: cfg.SessionSecret,
	})
	if err != nil {
		return nil, err
	}

	origin, err := url.Parse(cfg.PublicURL)
	if err != nil || !validPublicOrigin(origin) {
		return nil, errors.New("public-url must be an HTTP(S) origin")
	}

	return &Service{
		Service: coreService,
		prefix:  cfg.RoutePrefix,
		secure:  origin.Scheme == "https",
	}, nil
}

// validPublicOrigin accepts an HTTP(S) origin without credentials, paths or extras.
func validPublicOrigin(origin *url.URL) bool {
	return origin != nil && origin.Host != "" && utils.HasValidScheme(origin) &&
		origin.User == nil && origin.RawQuery == "" && !origin.ForceQuery && origin.Fragment == "" &&
		(origin.Path == "" || origin.Path == "/")
}
