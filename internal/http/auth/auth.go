// Package auth adapts authentication to HTTP requests.
package auth

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"slices"
	"strings"

	kit "github.com/containeroo/appkit/auth"
	"github.com/containeroo/appkit/auth/proxy"
	appkitconfig "github.com/containeroo/appkit/config"
	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/gi8lino/garage-webui/internal/application/authentication"
	"golang.org/x/oauth2"
)

// Config contains the deployment settings needed by HTTP authentication.
type Config struct {
	// PublicURL is the browser-accessible origin.
	PublicURL string
	// RoutePrefix is the externally visible application path.
	RoutePrefix string
	// Authentication contains authentication overrides.
	Authentication appkitconfig.Authentication
	// Proxy configures trusted proxy identity.
	Proxy appkitconfig.Proxy
	// OIDC configures OpenID Connect.
	OIDC appkitconfig.OIDC
}

// Service owns authentication configuration and persistent sessions.
type Service struct {
	*authentication.Service
	sessions   sessionRepository
	mode       kit.Mode
	proxy      proxy.Authenticator[proxyUser]
	oauth      *oauth2.Config
	verifier   *oidc.IDTokenVerifier
	adminGroup string
	groupClaim string
	issuer     string
	localLogin bool
	secure     bool
	prefix     string
}

// proxyUser contains a validated proxy identity and its administrator claim.
type proxyUser struct {
	username string
	admin    bool
}

// ErrSetupComplete indicates that an administrator already exists.
var ErrSetupComplete = authentication.ErrSetupComplete

// ErrInvalidSetup indicates invalid administrator credentials.
var ErrInvalidSetup = authentication.ErrInvalidSetup

// New validates and constructs authentication.
func New(ctx context.Context, store repository, cfg Config) (*Service, error) {
	if store == nil {
		return nil, errors.New("authentication storage is required")
	}

	publicURL, err := url.Parse(cfg.PublicURL)
	if err != nil || !validPublicOrigin(publicURL) {
		return nil, errors.New("public URL must be an HTTP(S) origin without a path")
	}

	if cfg.Authentication.ModeOverride == nil {
		return nil, errors.New("--auth-mode must be local, proxy, or oidc")
	}

	s := &Service{
		Service:    authentication.New(store, *cfg.Authentication.ModeOverride, cfg.Authentication.LocalLogin, cfg.Authentication.AllowRegistrationOverride),
		sessions:   store,
		mode:       *cfg.Authentication.ModeOverride,
		secure:     strings.HasPrefix(cfg.PublicURL, "https://"),
		prefix:     cfg.RoutePrefix,
		localLogin: cfg.Authentication.LocalLogin,
	}
	switch s.mode {
	case kit.ModeLocal:
	case kit.ModeProxy:
		if cfg.Proxy.AdminGroup == "" {
			return nil, errors.New("trusted proxy requires --trusted-admin-group")
		}
		p := proxy.Config{
			TrustedCIDRs:       cfg.Proxy.TrustedCIDRs,
			UsernameHeaders:    cfg.Proxy.UsernameHeaders,
			EmailHeaders:       cfg.Proxy.EmailHeaders,
			DisplayNameHeaders: cfg.Proxy.DisplayNameHeaders,
			GroupHeaders:       cfg.Proxy.GroupHeaders,
		}
		s.proxy, err = proxy.New(p, "garage-webui", func(ctx context.Context, id kit.ExternalIdentity) (proxyUser, error) {
			username, err := s.ResolveExternal(ctx, "proxy", "", id.Username, id.Username)
			if err != nil {
				return proxyUser{}, err
			}

			if err := s.RefreshExternalProfile(ctx, username, Identity{
				Provider: "proxy",
				Subject:  id.Username,
			}, id.Username, id.Email, id.DisplayName); err != nil {
				return proxyUser{}, err
			}

			return proxyUser{
				username: username,
				admin:    slices.Contains(id.Groups, cfg.Proxy.AdminGroup),
			}, nil
		})
		if err != nil {
			return nil, err
		}
	case kit.ModeOIDC:
		if cfg.OIDC.Issuer == "" || cfg.OIDC.ClientID == "" || cfg.OIDC.ClientSecret == "" || cfg.OIDC.AdminGroup == "" {
			return nil, errors.New("OIDC requires issuer, client ID, client secret and admin group")
		}
		provider, err := oidc.NewProvider(ctx, cfg.OIDC.Issuer)
		if err != nil {
			return nil, fmt.Errorf("discover OIDC provider: %w", err)
		}
		s.verifier = provider.Verifier(&oidc.Config{ClientID: cfg.OIDC.ClientID})
		s.oauth = &oauth2.Config{
			ClientID:     cfg.OIDC.ClientID,
			ClientSecret: cfg.OIDC.ClientSecret,
			Endpoint:     provider.Endpoint(),
			RedirectURL:  strings.TrimRight(cfg.PublicURL, "/") + s.prefix + "/auth/callback",
			Scopes:       []string{oidc.ScopeOpenID, "profile", "email"},
		}
		s.adminGroup, s.groupClaim = cfg.OIDC.AdminGroup, cfg.OIDC.GroupClaim
		s.issuer = cfg.OIDC.Issuer
	default:
		return nil, errors.New("unsupported authentication mode; choose local, proxy, or oidc")
	}

	return s, nil
}

// validPublicOrigin reports whether a URL is an HTTP origin without credentials.
func validPublicOrigin(u *url.URL) bool {
	return u != nil && hasHTTPOrigin(u) && hasNoURLExtras(u) && (u.Path == "" || u.Path == "/")
}

// hasHTTPOrigin reports whether a URL has an HTTP scheme and host.
func hasHTTPOrigin(u *url.URL) bool {
	return (u.Scheme == "http" || u.Scheme == "https") && u.Host != ""
}

// hasNoURLExtras reports whether a URL excludes credentials, query and fragment.
func hasNoURLExtras(u *url.URL) bool {
	return u.User == nil && u.RawQuery == "" && u.Fragment == "" && !u.ForceQuery
}
