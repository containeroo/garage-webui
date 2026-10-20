// Package server exposes the authenticated web interface and Garage operations.
package server

import (
	"bytes"
	"context"
	"html"
	"io/fs"
	"log/slog"
	"net/http"
	"strings"

	"github.com/containeroo/garage-webui/internal/application/objects"
	"github.com/containeroo/garage-webui/internal/http/auth"
	"github.com/containeroo/garage-webui/internal/http/middleware"
	"github.com/containeroo/garage-webui/themes"
	httpkitmiddleware "github.com/containeroo/httpkit/middleware"
)

// ObjectServices prepares credential-scoped S3 operations and bucket deletion.
type ObjectServices interface {
	// ForIdentity creates object services scoped to the authenticated identity.
	ForIdentity(auth.Identity) (*objects.Service, error)
	// DeleteBucket deletes the requested bucket.
	DeleteBucket(context.Context, auth.Identity, string) (int, []byte, error)
}

// Server combines authenticated routes and the static frontend.
type Server struct {
	// themes lists the available application themes.
	themes []themes.Theme
	// assets provides the embedded frontend files.
	assets fs.FS
	// s3Origin identifies the S3 origin allowed by browser security policies.
	s3Origin string
	// objects provides identity-scoped object operations.
	objects ObjectServices
	// auth manages authenticated application sessions.
	auth *auth.Service
	// publicURL specifies the browser-facing application URL.
	publicURL string
	// prefix specifies the application route prefix.
	prefix string
	// logger records application diagnostics.
	logger *slog.Logger
	// accessLog enables HTTP access logging.
	accessLog bool
	// loginLimiter enforces the login attempt budget.
	loginLimiter middleware.LoginLimiter
}

// New constructs the HTTP server.
func New(
	authentication *auth.Service,
	objectServices ObjectServices,
	loginLimiter middleware.LoginLimiter,
	s3Origin string,
	publicURL, prefix string,
	logger *slog.Logger,
	accessLog bool,
	assets fs.FS,
	themeCatalog []themes.Theme,
) *Server {
	return &Server{
		themes:       themeCatalog,
		assets:       assets,
		objects:      objectServices,
		loginLimiter: loginLimiter,
		s3Origin:     s3Origin,
		auth:         authentication,
		publicURL:    publicURL,
		prefix:       prefix,
		logger:       logger,
		accessLog:    accessLog,
	}
}

// Handler returns the application routes.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	policies := middleware.New(s.auth, s.s3Origin, s.logger, validMutation(s.publicURL), middleware.AllowLogin(s.loginLimiter, s.logger))

	mux.Handle("GET /api/session", Session(s.auth, s.logger))

	mux.Handle("POST /api/login",
		httpkitmiddleware.Chain(
			Login(s.auth, s.logger),
			policies.Mutation,
			policies.LimitLogin(true),
		))
	mux.Handle("POST /api/logout",
		httpkitmiddleware.Chain(
			Logout(s.auth),
			policies.Mutation,
		))
	mux.Handle("GET /api/profile",
		httpkitmiddleware.Chain(
			OwnProfile(),
			policies.Authenticate,
		))
	mux.Handle("GET /api/themes",
		httpkitmiddleware.Chain(
			ThemeCatalog(s.themes),
			policies.Authenticate,
		))
	mux.Handle("PUT /api/profile/theme",
		httpkitmiddleware.Chain(
			UpdateOwnTheme(s.auth, s.themes),
			policies.Authenticate,
			policies.Mutation,
		))
	mux.Handle("GET /api/garage/{operation}",
		httpkitmiddleware.Chain(
			GarageRequest(s.auth, s.objects, s.logger),
			policies.Authenticate,
			policies.Mutation,
		))
	mux.Handle("POST /api/garage/{operation}",
		httpkitmiddleware.Chain(
			GarageRequest(s.auth, s.objects, s.logger),
			policies.Authenticate,
			policies.Mutation,
		))
	mux.Handle("POST /api/uploads/presign",
		httpkitmiddleware.Chain(
			PresignUpload(s.objects, s.logger),
			policies.Authenticate,
			policies.Mutation,
		))
	mux.Handle("POST /api/objects/presign",
		httpkitmiddleware.Chain(
			PresignObject(s.objects, s.logger),
			policies.Authenticate,
			policies.Mutation,
		))
	mux.Handle("GET /", Frontend(s.assets, s.prefix))

	middlewares := []httpkitmiddleware.Middleware{
		policies.SecurityHeaders(),
		middleware.APIHeaders(),
		middleware.RequestContext(),
	}

	if s.accessLog {
		middlewares = append(middlewares, policies.AccessLogger())
	}

	return httpkitmiddleware.Chain(mux, middlewares...)
}

// Frontend serves the SPA with a deployment-relative base URL and static assets.
// AppKit strips the route prefix before reaching this handler, just like Kumbuka.
func Frontend(assetsFS fs.FS, prefix string) http.Handler {
	assets := http.FileServer(http.FS(assetsFS))
	index, err := fs.ReadFile(assetsFS, "index.html")
	if err != nil {
		// A missing embedded index is a build error; fail each request safely.
		return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "frontend unavailable", http.StatusInternalServerError)
		})
	}

	notFound, err := fs.ReadFile(assetsFS, "404.html")
	if err != nil {
		return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "frontend unavailable", http.StatusInternalServerError)
		})
	}

	base := strings.TrimRight(prefix, "/") + "/"
	index = replaceBase(index, base)
	notFound = replaceBase(notFound, base)

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" || r.URL.Path == "/index.html" || r.URL.Path == "/login" {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.Header().Set("Cache-Control", "no-store")
			_, _ = w.Write(index)

			return
		}

		if isMissingPage(r) {
			if _, err := fs.Stat(assetsFS, strings.TrimPrefix(r.URL.Path, "/")); err != nil {
				w.Header().Set("Content-Type", "text/html; charset=utf-8")
				w.Header().Set("Cache-Control", "no-store")
				w.WriteHeader(http.StatusNotFound)
				_, _ = w.Write(notFound)

				return
			}
		}

		assets.ServeHTTP(w, r)
	})
}

// replaceBase sets the deployment path in a built frontend document.
func replaceBase(document []byte, base string) []byte {
	replacement := []byte(`<base href="` + html.EscapeString(base) + `">`)
	document = bytes.Replace(document, []byte(`<base href="/" />`), replacement, 1)

	return bytes.Replace(document, []byte(`<base href="/">`), replacement, 1)
}

// isMissingPage identifies browser navigation without intercepting API requests.
func isMissingPage(r *http.Request) bool {
	return !strings.HasPrefix(r.URL.Path, "/api/") &&
		r.URL.Path != "/api" &&
		strings.Contains(r.Header.Get("Accept"), "text/html")
}
