package server

import (
	"net/http"

	"github.com/containeroo/garage-webui/internal/http/endpoint"
	"github.com/containeroo/garage-webui/internal/http/middleware"
	httpkitmiddleware "github.com/containeroo/httpkit/middleware"
)

// Handler returns the application routes.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	policies := middleware.New(s.auth, s.s3Origin, s.publicURL, s.loginLimiter, s.logger)

	mux.Handle(http.MethodGet+" /api/session", endpoint.Session(s.auth, s.logger))
	mux.Handle(http.MethodPost+" /api/login",
		httpkitmiddleware.Chain(
			endpoint.Login(s.auth, s.logger),
			policies.Mutation,
			policies.LimitLogin(true),
		))
	mux.Handle(http.MethodPost+" /api/logout",
		httpkitmiddleware.Chain(
			endpoint.Logout(s.auth),
			policies.Mutation,
		))

	mux.Handle(http.MethodGet+" /api/profile",
		httpkitmiddleware.Chain(
			endpoint.OwnProfile(),
			policies.Authenticate,
		))
	mux.Handle(http.MethodGet+" /api/themes",
		httpkitmiddleware.Chain(
			endpoint.ThemeCatalog(s.themes),
			policies.Authenticate,
		))
	mux.Handle(http.MethodPut+" /api/profile/theme",
		httpkitmiddleware.Chain(
			endpoint.UpdateOwnTheme(s.auth, s.themes),
			policies.Authenticate,
			policies.Mutation,
		))

	garage := endpoint.GarageRequest(s.auth, s.objects, s.logger)
	mux.Handle(http.MethodGet+" /api/garage/{operation}",
		httpkitmiddleware.Chain(
			garage,
			policies.Authenticate,
			policies.Mutation,
		))
	mux.Handle(http.MethodPost+" /api/garage/{operation}",
		httpkitmiddleware.Chain(
			garage,
			policies.Authenticate,
			policies.Mutation,
		))

	transfer := endpoint.TransferObject(s.objects, s.logger, s.transferOptions)
	mux.Handle(http.MethodGet+" /api/objects/transfer",
		httpkitmiddleware.Chain(
			transfer,
			policies.Authenticate,
			middleware.RequireTransferOrigin(s.publicURL),
		))
	mux.Handle(http.MethodPut+" /api/objects/transfer",
		httpkitmiddleware.Chain(
			transfer,
			policies.Authenticate,
			middleware.RequireTransferOrigin(s.publicURL),
		))
	mux.Handle(http.MethodPost+" /api/objects/transfer",
		httpkitmiddleware.Chain(
			transfer,
			policies.Authenticate,
			middleware.RequireTransferOrigin(s.publicURL),
		))
	mux.Handle(http.MethodDelete+" /api/objects/transfer",
		httpkitmiddleware.Chain(
			transfer,
			policies.Authenticate,
			middleware.RequireTransferOrigin(s.publicURL),
		))

	mux.Handle(http.MethodPost+" /api/uploads/presign",
		httpkitmiddleware.Chain(
			endpoint.PresignUpload(s.objects, s.logger),
			policies.Authenticate,
			policies.Mutation,
		))
	mux.Handle(http.MethodPost+" /api/objects/presign",
		httpkitmiddleware.Chain(
			endpoint.PresignObject(s.objects, s.logger),
			policies.Authenticate,
			policies.Mutation,
		))

	mux.Handle(http.MethodGet+" /", endpoint.Frontend(s.assets, s.prefix))

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
