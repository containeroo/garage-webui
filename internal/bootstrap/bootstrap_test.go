package bootstrap

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/containeroo/garage-webui/internal/database"
	"github.com/containeroo/garage-webui/internal/flags"
	"github.com/containeroo/garage-webui/themes"
	"github.com/containeroo/garage-webui/web"
	"github.com/stretchr/testify/require"
)

func TestDatabaseOwnership(t *testing.T) {
	for _, scenario := range []struct {
		name      string
		configure func(*flags.Config)
		wantError bool
	}{
		{name: "shutdown"},
		{name: "authentication initialization fails", configure: func(cfg *flags.Config) { cfg.SessionSecret = "invalid" }, wantError: true},
		{name: "S3 validation fails", configure: func(cfg *flags.Config) { cfg.S3PublicURL = "invalid" }, wantError: true},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			cfg := flags.Config{
				Database:      database.Config{URL: "sqlite:" + filepath.Join(t.TempDir(), "console.db")},
				GarageURL:     "http://garage.example:3903",
				SessionSecret: "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=",
				S3Region:      "garage",
			}
			cfg.Server.PublicURL = "http://console.example"
			if scenario.configure != nil {
				scenario.configure(&cfg)
			}
			app, err := New(t.Context(), cfg, slog.Default(), web.Assets, themes.Files)
			if scenario.wantError {
				require.Error(t, err)
				require.Nil(t, app)
			} else {
				require.NoError(t, err)
				t.Cleanup(app.Close)
				for _, path := range []string{"/healthz", "/readyz"} {
					response := httptest.NewRecorder()
					app.Management().ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
					require.Equal(t, http.StatusOK, response.Code)
				}
				app.Close()
			}
			// Both failed startup and normal shutdown must release the SQLite process lock.
			reopened, err := database.Open(context.Background(), cfg.Database, slog.Default())
			require.NoError(t, err)
			reopened.Close()
		})
	}
}
