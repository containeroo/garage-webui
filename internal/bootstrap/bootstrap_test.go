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
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDatabaseOwnership(t *testing.T) {
	t.Parallel()

	t.Run("shutdown", func(t *testing.T) {
		t.Parallel()

		cfg := flags.Config{
			Database:      database.Config{URL: "sqlite:" + filepath.Join(t.TempDir(), "console.db")},
			GarageURL:     "http://garage.example:3903",
			SessionSecret: "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=",
			S3Region:      "garage",
		}
		cfg.Server.PublicURL = "http://console.example"

		app, err := New(t.Context(), cfg, slog.Default(), web.Assets, themes.Files)

		require.NoError(t, err)
		t.Cleanup(app.Close)
		t.Run("/healthz", func(t *testing.T) {
			// Run before closing the shared application.

			path := "/healthz"
			response := httptest.NewRecorder()
			app.Management().ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
			assert.Equal(t, http.StatusOK, response.Code)
		})

		t.Run("/readyz", func(t *testing.T) {
			// Run before closing the shared application.

			path := "/readyz"
			response := httptest.NewRecorder()
			app.Management().ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
			assert.Equal(t, http.StatusOK, response.Code)
		})
		app.Close()

		// Both failed startup and normal shutdown must release the SQLite process lock.
		reopened, err := database.Open(context.Background(), cfg.Database, slog.Default())
		require.NoError(t, err)
		reopened.Close()
	})

	t.Run("authentication initialization fails", func(t *testing.T) {
		t.Parallel()

		cfg := flags.Config{
			Database:      database.Config{URL: "sqlite:" + filepath.Join(t.TempDir(), "console.db")},
			GarageURL:     "http://garage.example:3903",
			SessionSecret: "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=",
			S3Region:      "garage",
		}
		cfg.Server.PublicURL = "http://console.example"

		cfg.SessionSecret = "invalid"

		app, err := New(t.Context(), cfg, slog.Default(), web.Assets, themes.Files)

		require.Error(t, err)
		assert.Nil(t, app)

		// Both failed startup and normal shutdown must release the SQLite process lock.
		reopened, err := database.Open(context.Background(), cfg.Database, slog.Default())
		require.NoError(t, err)
		reopened.Close()
	})

	t.Run("S3 validation fails", func(t *testing.T) {
		t.Parallel()

		cfg := flags.Config{
			Database:      database.Config{URL: "sqlite:" + filepath.Join(t.TempDir(), "console.db")},
			GarageURL:     "http://garage.example:3903",
			SessionSecret: "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=",
			S3Region:      "garage",
		}
		cfg.Server.PublicURL = "http://console.example"

		cfg.S3PublicURL = "invalid"

		app, err := New(t.Context(), cfg, slog.Default(), web.Assets, themes.Files)

		require.Error(t, err)
		assert.Nil(t, app)

		// Both failed startup and normal shutdown must release the SQLite process lock.
		reopened, err := database.Open(context.Background(), cfg.Database, slog.Default())
		require.NoError(t, err)
		reopened.Close()
	})
}
