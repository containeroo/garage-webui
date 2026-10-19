// Package server exposes the authenticated web interface and Garage operations.
package server

import (
	"bytes"
	"html"
	"io/fs"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	httpkitmiddleware "github.com/containeroo/httpkit/middleware"
	garage "github.com/gi8lino/garage-webui/internal/application/garage"
	"github.com/gi8lino/garage-webui/internal/application/objects"
	"github.com/gi8lino/garage-webui/internal/http/auth"
	"github.com/gi8lino/garage-webui/internal/http/middleware"
	"github.com/gi8lino/garage-webui/themes"
)

// Server combines authenticated routes and the static frontend.
type Server struct {
	themes      []themes.Theme
	assets      fs.FS
	garage      *garage.Service
	s3Origin    string
	objects     *objects.Service
	auth        *auth.Service
	publicURL   string
	prefix      string
	logger      *slog.Logger
	accessLog   bool
	loginMu     sync.Mutex
	loginWindow time.Time
	loginCounts map[string]int
}

// New constructs the HTTP server.
func New(
	garageService *garage.Service,
	authentication *auth.Service,
	objectService *objects.Service,
	s3Origin string,
	publicURL, prefix string,
	logger *slog.Logger,
	accessLog bool,
	assets fs.FS,
	themeCatalog []themes.Theme,
) *Server {
	return &Server{
		themes:    themeCatalog,
		assets:    assets,
		garage:    garageService,
		objects:   objectService,
		s3Origin:  s3Origin,
		auth:      authentication,
		publicURL: publicURL,
		prefix:    prefix,
		logger:    logger,
		accessLog: accessLog,
	}
}

// Handler returns the application routes.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	policies := middleware.New(s.garage, s.auth, s.s3Origin, s.logger, s.validMutation, s.allowLogin)

	mux.HandleFunc("GET /api/session", s.session)
	mux.Handle("POST /api/setup",
		httpkitmiddleware.Chain(
			http.HandlerFunc(s.setup),
			policies.Mutation,
			policies.LimitLogin(true)),
	)
	mux.Handle("POST /api/login",
		httpkitmiddleware.Chain(
			http.HandlerFunc(s.login),
			policies.Mutation,
			policies.LimitLogin(true)),
	)
	mux.Handle("POST /api/logout",
		httpkitmiddleware.Chain(
			http.HandlerFunc(s.logout),
			policies.Mutation),
	)
	mux.Handle("GET /api/profile",
		httpkitmiddleware.Chain(
			http.HandlerFunc(s.getOwnProfile),
			policies.Authenticate,
			policies.Mutation),
	)
	mux.Handle("PUT /api/profile",
		httpkitmiddleware.Chain(
			http.HandlerFunc(s.updateOwnProfile),
			policies.Authenticate,
			policies.Mutation),
	)
	mux.Handle("GET /api/themes",
		httpkitmiddleware.Chain(http.HandlerFunc(s.themeCatalog), policies.Authenticate),
	)
	mux.Handle("PUT /api/profile/theme",
		httpkitmiddleware.Chain(http.HandlerFunc(s.updateOwnTheme), policies.Authenticate, policies.Mutation),
	)
	mux.Handle("POST /api/profile/password",
		httpkitmiddleware.Chain(
			http.HandlerFunc(s.changeOwnPassword),
			policies.Authenticate,
			policies.Mutation,
			policies.LimitLogin(false)),
	)
	mux.HandleFunc("GET /auth/login", s.oidcLogin)
	mux.HandleFunc("GET /auth/callback", s.oidcCallback)
	mux.Handle("GET /api/users",
		httpkitmiddleware.Chain(
			http.HandlerFunc(s.listUsers),
			policies.Authenticate,
			policies.RequireAdmin,
			policies.Mutation),
	)
	mux.Handle("POST /api/users",
		httpkitmiddleware.Chain(
			http.HandlerFunc(s.createUser),
			policies.Authenticate,
			policies.RequireAdmin,
			policies.Mutation),
	)
	mux.Handle("PUT /api/users/{username}",
		httpkitmiddleware.Chain(
			http.HandlerFunc(s.updateUser),
			policies.Authenticate,
			policies.RequireAdmin,
			policies.Mutation),
	)
	mux.Handle("PUT /api/users/{username}/profile",
		httpkitmiddleware.Chain(
			http.HandlerFunc(s.updateUserProfile),
			policies.Authenticate,
			policies.RequireAdmin,
			policies.Mutation),
	)
	mux.Handle("POST /api/users/{username}/profile/revert",
		httpkitmiddleware.Chain(
			http.HandlerFunc(s.revertUserProfile),
			policies.Authenticate,
			policies.RequireAdmin,
			policies.Mutation),
	)
	mux.Handle("PUT /api/users/{username}/username",
		httpkitmiddleware.Chain(
			http.HandlerFunc(s.renameUser),
			policies.Authenticate,
			policies.RequireAdmin,
			policies.Mutation),
	)
	mux.Handle("PUT /api/users/{username}/groups",
		httpkitmiddleware.Chain(
			http.HandlerFunc(s.updateUserGroups),
			policies.Authenticate,
			policies.RequireAdmin,
			policies.Mutation),
	)
	mux.Handle("PUT /api/users/{username}/credential",
		httpkitmiddleware.Chain(
			http.HandlerFunc(s.updateUserCredential),
			policies.Authenticate,
			policies.RequireAdmin,
			policies.Mutation),
	)
	mux.Handle("POST /api/users/{username}/sessions/revoke",
		httpkitmiddleware.Chain(
			http.HandlerFunc(s.revokeUserSessions),
			policies.Authenticate,
			policies.RequireAdmin,
			policies.Mutation),
	)
	mux.Handle("GET /api/groups",
		httpkitmiddleware.Chain(
			http.HandlerFunc(s.listGroups),
			policies.Authenticate,
			policies.RequireAdmin,
			policies.Mutation),
	)
	mux.Handle("POST /api/groups",
		httpkitmiddleware.Chain(
			http.HandlerFunc(s.createGroup),
			policies.Authenticate,
			policies.RequireAdmin,
			policies.Mutation),
	)
	mux.Handle("DELETE /api/groups/{group}",
		httpkitmiddleware.Chain(
			http.HandlerFunc(s.deleteGroup),
			policies.Authenticate,
			policies.RequireAdmin,
			policies.Mutation),
	)
	mux.Handle("GET /api/group-mappings",
		httpkitmiddleware.Chain(
			http.HandlerFunc(s.listGroupMappings),
			policies.Authenticate,
			policies.RequireAdmin,
			policies.Mutation),
	)
	mux.Handle("POST /api/group-mappings",
		httpkitmiddleware.Chain(
			http.HandlerFunc(s.createGroupMapping),
			policies.Authenticate,
			policies.RequireAdmin,
			policies.Mutation),
	)
	mux.Handle("DELETE /api/group-mappings",
		httpkitmiddleware.Chain(
			http.HandlerFunc(s.deleteGroupMapping),
			policies.Authenticate,
			policies.RequireAdmin,
			policies.Mutation),
	)
	mux.Handle("DELETE /api/users/{username}",
		httpkitmiddleware.Chain(
			http.HandlerFunc(s.deleteUser),
			policies.Authenticate,
			policies.RequireAdmin,
			policies.Mutation),
	)
	mux.Handle("POST /api/users/{username}/identities",
		httpkitmiddleware.Chain(
			http.HandlerFunc(s.linkIdentity),
			policies.Authenticate,
			policies.RequireAdmin,
			policies.Mutation,
		))
	mux.Handle("DELETE /api/users/{username}/identities",
		httpkitmiddleware.Chain(
			http.HandlerFunc(s.unlinkIdentity),
			policies.Authenticate,
			policies.RequireAdmin,
			policies.Mutation,
		))
	mux.Handle("POST /api/users/{username}/identities/proxy/relink",
		httpkitmiddleware.Chain(
			http.HandlerFunc(s.relinkProxyIdentity),
			policies.Authenticate,
			policies.RequireAdmin,
			policies.Mutation,
		))
	mux.Handle("GET /api/registration",
		httpkitmiddleware.Chain(
			http.HandlerFunc(s.getRegistration),
			policies.Authenticate,
			policies.RequireAdmin,
			policies.Mutation,
		))
	mux.Handle("PUT /api/registration",
		httpkitmiddleware.Chain(
			http.HandlerFunc(s.setRegistration),
			policies.Authenticate,
			policies.RequireAdmin,
			policies.Mutation,
		))
	mux.Handle("GET /api/pending-identities",
		httpkitmiddleware.Chain(
			http.HandlerFunc(s.listPendingIdentities),
			policies.Authenticate,
			policies.RequireAdmin,
			policies.Mutation,
		))
	mux.Handle("DELETE /api/pending-identities",
		httpkitmiddleware.Chain(
			http.HandlerFunc(s.rejectPendingIdentity),
			policies.Authenticate,
			policies.RequireAdmin,
			policies.Mutation,
		))
	mux.Handle("POST /api/pending-identities/approve",
		httpkitmiddleware.Chain(
			http.HandlerFunc(s.approvePendingIdentity),
			policies.Authenticate,
			policies.RequireAdmin,
			policies.Mutation,
		))
	mux.Handle("POST /api/pending-identities/link",
		httpkitmiddleware.Chain(
			http.HandlerFunc(s.linkPendingIdentity),
			policies.Authenticate,
			policies.RequireAdmin,
			policies.Mutation,
		))
	mux.Handle("POST /api/pending-identities/reopen",
		httpkitmiddleware.Chain(
			http.HandlerFunc(s.reopenPendingIdentity),
			policies.Authenticate,
			policies.RequireAdmin,
			policies.Mutation,
		))
	mux.Handle("GET /api/garage/{operation}",
		httpkitmiddleware.Chain(
			http.HandlerFunc(s.garageRequest),
			policies.Authenticate,
			policies.GaragePolicy,
			policies.Mutation,
		))
	mux.Handle("POST /api/garage/{operation}",
		httpkitmiddleware.Chain(
			http.HandlerFunc(s.garageRequest),
			policies.Authenticate,
			policies.GaragePolicy,
			policies.Mutation,
		))
	mux.Handle("POST /api/uploads/presign",
		httpkitmiddleware.Chain(
			http.HandlerFunc(s.presignUpload),
			policies.Authenticate,
			policies.Mutation,
		))
	mux.Handle("POST /api/objects/presign",
		httpkitmiddleware.Chain(
			http.HandlerFunc(s.presignObject),
			policies.Authenticate,
			policies.Mutation,
		))
	mux.Handle("GET /", s.frontend())

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

// frontend serves the SPA with a deployment-relative base URL and static assets.
// AppKit strips the route prefix before reaching this handler, just like Kumbuka.
func (s *Server) frontend() http.Handler {
	assets := http.FileServer(http.FS(s.assets))
	index, err := fs.ReadFile(s.assets, "index.html")
	if err != nil {
		// A missing embedded index is a build error; fail each request safely.
		return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "frontend unavailable", http.StatusInternalServerError)
		})
	}

	notFound, err := fs.ReadFile(s.assets, "404.html")
	if err != nil {
		return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "frontend unavailable", http.StatusInternalServerError)
		})
	}

	base := strings.TrimRight(s.prefix, "/") + "/"
	index = replaceBase(index, base)
	notFound = replaceBase(notFound, base)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" || r.URL.Path == "/index.html" || (r.URL.Path == "/login" && s.auth != nil && s.auth.LocalLoginEnabled()) {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.Header().Set("Cache-Control", "no-store")
			_, _ = w.Write(index)
			return
		}

		if isMissingPage(r) {
			if _, err := fs.Stat(s.assets, strings.TrimPrefix(r.URL.Path, "/")); err != nil {
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
