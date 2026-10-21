// Package flags parses deployment configuration.
package flags

import (
	"cmp"

	appkitconfig "github.com/containeroo/appkit/config"
	"github.com/containeroo/garage-webui/internal/database"
	"github.com/containeroo/tinyflags"
)

// Config contains deployment settings.
type Config struct {
	// Server configures HTTP listeners.
	Server appkitconfig.Server
	// Database selects SQLite or PostgreSQL.
	Database database.Config
	// Logging configures process logs.
	Logging appkitconfig.Logging
	// GarageURL is the Garage Admin API base URL.
	GarageURL string
	// SessionSecret encrypts stored Garage credentials.
	SessionSecret string
	// S3URL is the server-side S3 endpoint; empty uses S3PublicURL.
	S3URL string
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

	registerDatabase(tf, &cfg.Database)
	finalizeServer := appkitconfig.RegisterServer(tf, &cfg.Server, appkitconfig.ServerOptions{})

	finalizeLogging := appkitconfig.RegisterLogging(tf, &cfg.Logging)

	registerGarage(tf, &cfg)
	finalizeS3 := registerS3(tf, &cfg)

	if err := tf.Parse(args); err != nil {
		return Config{}, err
	}

	finalizeServer()
	finalizeLogging()
	finalizeS3()

	cfg.Overrides = tf.Overrides()

	return cfg, nil
}

// registerGarage registers Garage Admin API flags.
func registerGarage(tf *tinyflags.FlagSet, cfg *Config) {
	tf.StringVar(&cfg.GarageURL, "garage-url", "", "Garage Admin API URL").
		Required().
		Value()

	tf.StringVar(&cfg.SessionSecret, "session-secret", "", "Base64-encoded 32-byte key encrypting Garage credential sessions").
		Required().
		OverriddenValueMaskFn(tinyflags.MaskFirstLast).
		Value()
}

// registerS3 registers S3 flags and returns the endpoint-default finalizer.
func registerS3(tf *tinyflags.FlagSet, cfg *Config) func() {
	tf.StringVar(&cfg.S3URL, "s3-url", "", "Server-side Garage S3 API origin (defaults to s3-public-url).").
		Value()
	tf.StringVar(&cfg.S3PublicURL, "s3-public-url", "", "Browser-accessible Garage S3 API origin for direct uploads (optional).").
		Value()

	tf.StringVar(&cfg.S3Region, "s3-region", "garage", "Garage S3 region for signed upload URLs.").
		Value()

	return func() {
		cfg.S3URL = cmp.Or(cfg.S3URL, cfg.S3PublicURL)
	}
}

// registerDatabase exposes the database URL and PostgreSQL-only pool options.
func registerDatabase(tf *tinyflags.FlagSet, cfg *database.Config) {
	tf.StringVar(&cfg.URL, "database-url", "", "PostgreSQL URL or SQLite file URL (sqlite:///absolute/path or sqlite:relative/path)").
		Section(appkitconfig.SectionDatabase).
		Placeholder("URL").
		Required().
		OverriddenValueMaskFn(tinyflags.MaskPostgresURL).
		Value()

	tf.Int32Var(&cfg.MaxConns, "database-max-conns", 0, "Maximum PostgreSQL connections; 0 uses driver defaults").
		Section(appkitconfig.SectionDatabase).
		Validate(tinyflags.NonNegative[int32]()).
		Value()

	tf.Int32Var(&cfg.MinIdleConns, "database-min-idle-conns", 0, "Minimum idle PostgreSQL connections; 0 uses driver defaults").
		Section(appkitconfig.SectionDatabase).
		Validate(tinyflags.NonNegative[int32]()).
		Value()
}
