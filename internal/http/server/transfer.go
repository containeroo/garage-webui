package server

import (
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"time"

	"github.com/containeroo/garage-webui/internal/application/objects"
	"github.com/containeroo/garage-webui/internal/http/auth"
	"github.com/containeroo/garage-webui/internal/http/middleware"
	httpresponse "github.com/containeroo/garage-webui/internal/http/response"
)

// TransferOptions configures the server-side S3 transport independently of browser operations.
type TransferOptions struct {
	// ResponseHeaderTimeout bounds the wait for S3 response headers after the request body is sent.
	// Zero disables this timeout; it never limits the time spent streaming file bytes.
	ResponseHeaderTimeout time.Duration
}

// objectTransfer authorizes requests before handing their streams to the S3 reverse proxy.
type objectTransfer struct {
	services  ObjectServices
	transport http.RoundTripper
	logger    *slog.Logger
}

// transferPlan is derived exclusively from authorized application results, never a client-supplied URL.
type transferPlan struct {
	input  transferRequest
	result objects.Result
	target *url.URL
}

// TransferObject constructs a regular HTTP handler backed by a shared S3 transport.
func TransferObject(services ObjectServices, logger *slog.Logger, options TransferOptions) http.Handler {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.ResponseHeaderTimeout = options.ResponseHeaderTimeout
	transport.DisableCompression = true

	return &objectTransfer{services: services, transport: transport, logger: logger}
}

func (h *objectTransfer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	input, err := decodeTransferRequest(r)
	if err != nil {
		httpresponse.Error(w, err.Error(), transferErrorStatus(err))
		return
	}

	plan, err := h.prepare(r, input)
	if err != nil {
		httpresponse.Error(w, "cannot prepare object operation", transferErrorStatus(err))
		return
	}

	if r.Method != http.MethodGet {
		r.Body = http.MaxBytesReader(w, r.Body, input.bodyLimit())
	}

	newObjectStream(plan, h.transport, h.logger).ServeHTTP(w, r)
}

// prepare resolves the current identity and signs the internal target with only its Garage permissions.
func (h *objectTransfer) prepare(r *http.Request, input transferRequest) (transferPlan, error) {
	id, _ := middleware.RequestIdentity(r)
	service, err := h.services.ForIdentity(id)
	if err != nil {
		return transferPlan{}, fmt.Errorf("%w: object services unavailable", objects.ErrForbidden)
	}

	var result objects.Result
	if input.isSingleUpload() {
		result, err = service.PrepareUpload(r.Context(), id.Credential.Kind, input.BucketID, input.Key, input.Size, time.Now())
	} else {
		result, err = service.Prepare(r.Context(), id.Credential.Kind, objects.Request(input.objectRequest), time.Now())
	}
	if err != nil {
		return transferPlan{}, err
	}

	target, err := url.Parse(result.URL)
	if err != nil {
		return transferPlan{}, fmt.Errorf("invalid prepared S3 target")
	}

	return transferPlan{input: input, result: result, target: target}, nil
}

// transferOrigin protects non-JSON streamed writes with the same strict origin policy.
func transferOrigin(publicURL string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodGet && !auth.SameOrigin(r, publicURL) {
				httpresponse.Error(w, "invalid transfer origin", http.StatusForbidden)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
