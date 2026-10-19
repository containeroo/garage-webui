// Package main starts the Garage web interface.
package main

import (
	"context"
	"os"

	"github.com/containeroo/garage-webui/internal/app"
	"github.com/containeroo/garage-webui/themes"
	"github.com/containeroo/garage-webui/web"
)

var (
	version = "dev"
	commit  = "unknown"
)

// main delegates process setup to the application.
func main() {
	if err := app.Run(
		context.Background(),
		os.Args[1:],
		web.Assets,
		themes.Files,
		version,
		commit,
		os.Stdout,
		os.Stderr,
	); err != nil {
		os.Exit(1)
	}
}
