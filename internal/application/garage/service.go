package garage

import (
	"context"
	"errors"
	"io"
	"net/url"
)

// Requester sends requests to the Garage Admin API.
type Requester interface {
	Request(context.Context, string, string, url.Values, io.Reader) (statusCode int, responseBody []byte, err error)
}

// Service authorizes and forwards supported Garage operations.
type Service struct {
	client Requester
}

// New constructs a Garage operation service.
func New(client Requester) *Service { return &Service{client: client} }

// Operation errors describe rejected requests.
var (
	ErrForbidden        = errors.New("insufficient permissions")
	ErrUnknownOperation = errors.New("unknown operation")
	ErrMethod           = errors.New("method not allowed")
)

// Validate checks the role and method for an operation.
func (s *Service) Validate(role, method, operation string) error {
	if method == "POST" && requiresClusterAdmin(operation) && role != "admin" {
		return ErrForbidden
	}
	if !CanAccess(role, method) {
		return ErrForbidden
	}

	allowed, ok := OperationMethod(operation)
	if !ok {
		return ErrUnknownOperation
	}

	if method != allowed {
		return ErrMethod
	}

	return nil
}

// Request validates and forwards an operation.
func (s *Service) Request(
	ctx context.Context,
	role, method, operation string,
	query url.Values,
	body io.Reader,
) (statusCode int, responseBody []byte, err error) {
	if err := s.Validate(role, method, operation); err != nil {
		return 0, nil, err
	}

	safeQuery := make(url.Values, len(query))
	for key, values := range query {
		if key != "showSecretKey" {
			safeQuery[key] = values
		}
	}

	return s.client.Request(ctx, method, operation, safeQuery, body)
}
