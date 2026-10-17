// Package main starts the Garage web interface.
package main

import (
	"context"
	"os"

	"github.com/gi8lino/garage-webui/internal/app"
	"github.com/gi8lino/garage-webui/themes"
	"github.com/gi8lino/garage-webui/web"
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
