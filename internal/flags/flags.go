// Package flags parses deployment configuration.
package flags

import (
	appkitconfig "github.com/containeroo/appkit/config"
	"github.com/containeroo/tinyflags"
)

// Config contains deployment settings.
type Config struct {
	// Server configures HTTP listeners.
	Server appkitconfig.Server
	// Database configures PostgreSQL.
	Database appkitconfig.Postgres
	// Authentication configures authentication overrides.
	Authentication appkitconfig.Authentication
	// Proxy configures trusted proxy authentication.
	Proxy appkitconfig.Proxy
	// OIDC configures OpenID Connect.
	OIDC appkitconfig.OIDC
	// Logging configures process logs.
	Logging appkitconfig.Logging
	// GarageURL is the Garage Admin API base URL.
	GarageURL string
	// GarageToken authorizes requests to Garage.
	GarageToken string
	// S3PublicURL is the browser-accessible S3 API endpoint used for direct uploads.
	S3PublicURL string
	// S3Region is the Garage S3 region used for SigV4 signing.
	S3Region string
	// Overrides contains explicitly configured flag values.
	Overrides tinyflags.Overrides
}

// Parse reads command-line flags and environment variables.
func Parse(args []string, version string) (Config, error) {
	var cfg Config
	tf := appkitconfig.NewFlagSet("garage-webui", "GARAGE_WEBUI", version)

	appkitconfig.RegisterPostgres(tf, &cfg.Database, appkitconfig.PostgresOptions{})
	finalizeServer := appkitconfig.RegisterServer(tf, &cfg.Server, appkitconfig.ServerOptions{})

	finalizeAuthentication := registerAuthentication(tf, &cfg.Authentication)
	appkitconfig.RegisterProxy(tf, &cfg.Proxy)
	appkitconfig.RegisterOIDC(tf, &cfg.OIDC)

	finalizeLogging := appkitconfig.RegisterLogging(tf, &cfg.Logging)

	registerGarage(tf, &cfg)
	registerS3(tf, &cfg)

	if err := tf.Parse(args); err != nil {
		return Config{}, err
	}

	finalizeServer()
	finalizeAuthentication()
	finalizeLogging()
	cfg.Overrides = tf.Overrides()

	return cfg, nil
}

// registerGarage registers Garage Admin API flags.
func registerGarage(tf *tinyflags.FlagSet, cfg *Config) {
	tf.StringVar(&cfg.GarageURL, "garage-url", "", "Garage Admin API URL").
		Required().
		Value()

	tf.StringVar(&cfg.GarageToken, "garage-token", "", "Garage Admin API token").
		Required().
		OverriddenValueMaskFn(tinyflags.MaskFirstLast).
		Value()
}

// registerS3 registers S3 upload flags.
func registerS3(tf *tinyflags.FlagSet, cfg *Config) {
	tf.StringVar(&cfg.S3PublicURL, "s3-public-url", "", "Browser-accessible Garage S3 API origin for direct uploads (optional).").
		Value()
	tf.StringVar(&cfg.S3Region, "s3-region", "garage", "Garage S3 region for signed upload URLs.").
		Value()
}

// registerAuthentication registers the authentication overrides used by the WebUI.
func registerAuthentication(tf *tinyflags.FlagSet, cfg *appkitconfig.Authentication) func() {
	finalizeMode := appkitconfig.RegisterAuthMode(tf, cfg)
	finalizeRegistration := appkitconfig.RegisterUserRegistration(tf, cfg)
	appkitconfig.RegisterLocalLogin(tf, cfg)

	return func() {
		finalizeMode()
		finalizeRegistration()
	}
}
