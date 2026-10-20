// Package app orchestrates the Garage web interface process.
package app

import (
	"context"
	"fmt"
	"io"
	"io/fs"

	"github.com/containeroo/appkit/httpserver"
	"github.com/containeroo/appkit/logging"
	"github.com/containeroo/garage-webui/internal/bootstrap"
	"github.com/containeroo/garage-webui/internal/flags"
	"github.com/containeroo/httpkit/grace"
	"github.com/containeroo/tinyflags"
)

// Run configures and serves the application.
func Run(
	ctx context.Context,
	args []string,
	assets, themes fs.FS,
	version, commit string,
	stdout, stderr io.Writer,
) error {
	cfg, err := flags.Parse(args, version)
	if err != nil {
		if tinyflags.IsHelpRequested(err) || tinyflags.IsVersionRequested(err) {
			fmt.Fprint(stdout, err.Error()) // nolint:errcheck
			return nil
		}
		fmt.Fprintln(stderr, err) // nolint:errcheck

		return err
	}

	logger := logging.Setup(cfg.Logging.Format, cfg.Logging.Debug, stderr)
	setupLogger := logger.With("component", "setup")
	setupLogger.Info(
		"starting Garage Web UI",
		"event", "app_starting",
		"version", version,
		"commit", commit,
	)
	if len(cfg.Overrides) > 0 {
		setupLogger.Info("CLI Overrides", "event", "cli_overrides", "overrides", cfg.Overrides.Values())
	}

	ctx, stop := grace.SignalContext(ctx)
	defer stop()

	application, err := bootstrap.New(ctx, cfg, logger, assets, themes)
	if err != nil {
		logger.Error("initialize application", "error", err)
		return err
	}

	defer application.Close()

	handlers := httpserver.New(
		httpserver.Config{
			Application: application.Handler(),
			Management:  application.Management(),
			RoutePrefix: cfg.Server.RoutePrefix,
		})
	if err := httpserver.Run(
		ctx,
		cfg.Server.ListenAddress,
		cfg.Server.ManagementListenAddress,
		handlers,
		logger,
		logger,
	); err != nil {
		setupLogger.Error("run server", "event", "server_run_failed", "error", err)
		return err
	}

	return err
}
