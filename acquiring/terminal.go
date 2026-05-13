package acquiring

import (
	"context"
	"fmt"
	"net/http"
)

// Terminal — один T2P (terminal-to-phone) термінал у смартфоні
// співробітника торговця.
type Terminal struct {
	Code     string `json:"code"`
	Name     string `json:"name"`
	Terminal string `json:"terminal"`
}

// TerminalListResponse — обгортка для /t2p/terminal/list.
type TerminalListResponse struct {
	List []Terminal `json:"list"`
}

// Terminals повертає список T2P-терміналів (термінал у смартфоні) для
// цього торговця. У клієнта без терміналів список може бути порожнім або
// взагалі відсутнім у відповіді.
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
