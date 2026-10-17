package middleware

import (
	"net/http"

	httpkitmiddleware "github.com/containeroo/httpkit/middleware"
	httpkitresponse "github.com/containeroo/httpkit/response"
)

// AccessLogger logs requests with the recorded response status.
func (s *Policies) AccessLogger() httpkitmiddleware.Middleware {
	return httpkitmiddleware.AccessLogWithAttributes(s.logger, func(w http.ResponseWriter, _ *http.Request) []any {
		if response, ok := w.(*httpkitresponse.RequestWriter); ok {
			status := response.Status
			if status == 0 {
				status = http.StatusOK
			}
			return []any{"status", status}
		}
		return nil
	})
}
