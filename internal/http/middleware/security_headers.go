package middleware

import (
	"net/http"

	httpkitmiddleware "github.com/containeroo/httpkit/middleware"
)

// SecurityHeaders configures browser protections and permits the S3 origin when set.
func (s *Policies) SecurityHeaders() httpkitmiddleware.Middleware {
	csp := "default-src 'self'; frame-ancestors 'none'; base-uri 'self'"
	if s.s3Origin != "" {
		// The browser needs an explicit connect-src for cross-origin S3 PUTs.
		csp += "; connect-src 'self' " + s.s3Origin
		csp += "; img-src 'self' data: " + s.s3Origin
		csp += "; media-src 'self' " + s.s3Origin
	}

	return httpkitmiddleware.SecurityHeaders(http.Header{
		"X-Content-Type-Options":  {"nosniff"},
		"X-Frame-Options":         {"DENY"},
		"Referrer-Policy":         {"same-origin"},
		"Content-Security-Policy": {csp},
	})
}
