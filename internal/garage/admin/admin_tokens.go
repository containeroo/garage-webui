package admin

import (
	"context"
	"errors"
)

// AdminToken describes the identity and scopes of a Garage administration token.
type AdminToken struct {
	// Name is the display label of the Garage credential.
	Name string `json:"name"`
	// Scope lists the permitted Admin API operations.
	Scope []string `json:"scope"`
	// Expired reports whether Garage has expired this token.
	Expired bool `json:"expired"`
}

// CurrentAdminToken validates the supplied token and returns its active scopes.
func (c *Client) CurrentAdminToken(ctx context.Context) (AdminToken, error) {
	var info AdminToken
	if err := c.request(ctx, OpGetCurrentAdminTokenInfo, nil, nil, &info); err != nil {
		return AdminToken{}, err
	}

	if info.Name == "" || len(info.Scope) == 0 {
		return AdminToken{}, errors.New("garage returned incomplete admin token metadata")
	}

	return info, nil
}
