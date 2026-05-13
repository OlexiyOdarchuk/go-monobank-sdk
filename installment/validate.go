package installment

import (
	"context"
	"net/http"
)

// ValidateClientLegacy — застаріла валідація клієнта. Повертає повну
// інформацію (ПІБ, ІПН), якщо клієнт знайдений.
//
// POST /api/client/validate  (200 → ValidateClientResponse)
//
// Deprecated: використовуй [Client.ValidateClient] (v2) для всіх нових
// інтеграцій.
func (c *Client) ValidateClientLegacy(ctx context.Context, phone string) (*ValidateClientResponse, error) {
	var out ValidateClientResponse
	if err := c.doJSON(ctx, "/api/client/validate",
		ValidateClientRequest{Phone: phone}, &out, http.StatusOK); err != nil {
		return nil, err
	}
	return &out, nil
}

// ValidateClient перевіряє, чи телефон належить клієнту monobank.
// Повертає лише прапор found, без персональних даних (на відміну від
// застарілої версії).
//
// POST /api/v2/client/validate  (200 → ValidateClientSimpleResponse)
func (c *Client) ValidateClient(ctx context.Context, phone string) (bool, error) {
	var out ValidateClientSimpleResponse
	if err := c.doJSON(ctx, "/api/v2/client/validate",
		ValidateClientRequest{Phone: phone}, &out, http.StatusOK); err != nil {
		return false, err
	}
	return out.Found, nil
}
