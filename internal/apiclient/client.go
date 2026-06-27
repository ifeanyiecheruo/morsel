// Package apiclient provides a typed HTTP client for the Morsel API built on
// the ogen-generated oas.Client.
package apiclient

import (
	"context"

	"github.com/ifeanyiecheruo/morsel/internal/api/oas"
)

// Client wraps the ogen-generated API client and injects a bearer token on
// every authenticated request via the oas.SecuritySource interface.
type Client struct {
	inner *oas.Client
	token string
}

func New(serverURL, accessToken string) (*Client, error) {
	c := &Client{token: accessToken}
	inner, err := oas.NewClient(serverURL, c)
	if err != nil {
		return nil, err
	}
	c.inner = inner
	return c, nil
}

// BearerAuth implements oas.SecuritySource.
func (c *Client) BearerAuth(_ context.Context, _ oas.OperationName) (oas.BearerAuth, error) {
	return oas.BearerAuth{Token: c.token}, nil
}

func (c *Client) Inner() *oas.Client { return c.inner }

// compile-time interface check.
var _ oas.SecuritySource = (*Client)(nil)
