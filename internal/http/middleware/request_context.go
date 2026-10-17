package middleware

import (
	"net/http"

	httpkitmiddleware "github.com/containeroo/httpkit/middleware"
	httpkitresponse "github.com/containeroo/httpkit/response"
)

// RequestContext tracks the response status for downstream middleware.
func RequestContext() httpkitmiddleware.Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			next.ServeHTTP(httpkitresponse.NewRequestWriter(w, r), r)
		})
	}
}
