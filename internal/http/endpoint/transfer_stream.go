package endpoint

import (
	"errors"
	"io"
	"log"
	"log/slog"
	"net/http"
	"net/http/httputil"
	"time"

	httpresponse "github.com/containeroo/garage-webui/internal/http/response"
)

const (
	// ReverseProxy documents a negative flush interval as immediate streaming.
	flushStreamImmediately time.Duration = -1

	// Uploaded documents must not execute with the Web UI's origin or session privileges.
	streamedObjectContentPolicy = "sandbox allow-downloads; default-src 'none'; style-src 'unsafe-inline'"
)

// objectStream is a regular handler responsible only for transport and response streaming.
type objectStream struct {
	proxy *httputil.ReverseProxy
}

// newObjectStream binds one trusted transfer plan to the shared upstream transport.
func newObjectStream(plan transferPlan, transport http.RoundTripper, logger *slog.Logger) http.Handler {
	proxy := &httputil.ReverseProxy{
		Transport:     transport,
		FlushInterval: flushStreamImmediately,
		// Our error handler reports failures without logging signed upstream URLs.
		ErrorLog:       log.New(io.Discard, "", 0),
		Rewrite:        func(request *httputil.ProxyRequest) { rewriteTransferRequest(request, plan) },
		ModifyResponse: sanitizeTransferResponse,
		ErrorHandler:   streamErrorHandler(logger, plan.input),
	}

	return &objectStream{proxy: proxy}
}

func (h *objectStream) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// File transfers may outlive the API server's short response-write deadline.
	err := http.NewResponseController(w).SetWriteDeadline(time.Time{})
	if err != nil && !errors.Is(err, http.ErrNotSupported) {
		httpresponse.Error(w, "cannot start streamed transfer", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Security-Policy", streamedObjectContentPolicy)

	h.proxy.ServeHTTP(w, r)
}

// rewriteTransferRequest forwards only required headers, never browser cookies or authorization.
func rewriteTransferRequest(request *httputil.ProxyRequest, plan transferPlan) {
	target := *plan.target
	request.Out.URL = &target
	request.Out.Host = target.Host
	request.Out.Method = plan.result.Method
	request.Out.Header = make(http.Header)

	copyTransferHeaders(request.Out.Header, request.In.Header, "Content-Type", "Range", "If-Range")

	// Copy source and range headers come from the validated operation, not browser headers.
	if plan.result.CopySource != "" {
		request.Out.Header.Set("X-Amz-Copy-Source", plan.result.CopySource)
	}
	for name, value := range plan.result.CopyHeaders {
		request.Out.Header.Set(name, value)
	}
}

// sanitizeTransferResponse preserves file metadata and isolates untrusted uploaded documents.
func sanitizeTransferResponse(response *http.Response) error {
	headers := make(http.Header)
	copyTransferHeaders(headers, response.Header,
		"Content-Type", "Content-Length", "Content-Disposition", "ETag",
		"Last-Modified", "Accept-Ranges", "Content-Range",
	)
	response.Header = headers
	return nil
}

func copyTransferHeaders(target, source http.Header, names ...string) {
	for _, name := range names {
		if value := source.Get(name); value != "" {
			target.Set(name, value)
		}
	}
}

// streamErrorHandler reports transport failures without leaking signed URLs.
func streamErrorHandler(logger *slog.Logger, input transferRequest) func(http.ResponseWriter, *http.Request, error) {
	return func(w http.ResponseWriter, r *http.Request, err error) {
		if r.Context().Err() != nil {
			return
		}
		var oversized *http.MaxBytesError
		if errors.As(err, &oversized) {
			httpresponse.Error(w, "request body too large", http.StatusRequestEntityTooLarge)
			return
		}
		logger.Warn("S3 transfer failed", "bucket", input.BucketID, "action", input.Action)
		httpresponse.Error(w, "cannot reach Garage S3; check the server-side endpoint and network", http.StatusBadGateway)
	}
}
