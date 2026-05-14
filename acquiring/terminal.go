package acquiring

import (
	"context"
	"fmt"
	"net/http"
)

// Terminal is a single T2P (terminal-to-phone) terminal running on
// a merchant employee's smartphone.
type Terminal struct {
	Code     string `json:"code"`
	Name     string `json:"name"`
	Terminal string `json:"terminal"`
}

// TerminalListResponse wraps the /t2p/terminal/list response.
type TerminalListResponse struct {
	List []Terminal `json:"list"`
}

// Terminals returns the list of T2P terminals (terminal on a phone)
// for this merchant. For a merchant without terminals the list may
// be empty, or simply absent from the response.
func (c *Client) Terminals(ctx context.Context) ([]Terminal, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		"/api/merchant/t2p/terminal/list", http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	var out TerminalListResponse
	if err := c.c.Do(req, &out, http.StatusOK); err != nil {
		return nil, err
	}
	return out.List, nil
}
