package acquiring

import (
	"context"
	"fmt"
	"net/http"
)

// SplitReceiver — один отримувач у схемі розщеплення платежів.
// SplitReceiverID використовується у тілі CreateInvoiceRequest при
// розщепленні (передається банку через окремий механізм агентської
// угоди — недоступний у дефолтних мерчантів).
type SplitReceiver struct {
	SplitReceiverID string `json:"splitReceiverId"`
	OKPO            string `json:"okpo"`
	Name            string `json:"name"`
}

// SplitReceiverListResponse — обгортка для /split-receiver/list.
type SplitReceiverListResponse struct {
	List []SplitReceiver `json:"list"`
}

// SplitReceivers повертає список отримувачів для розщеплення платежів
// (саб-мерчантів). Працює лише в мерчантів, у яких налаштована схема
// агентського/розщеплювального еквайрингу.
func (c *Client) SplitReceivers(ctx context.Context) ([]SplitReceiver, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		"/api/merchant/split-receiver/list", http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	var out SplitReceiverListResponse
	if err := c.c.Do(req, &out, http.StatusOK); err != nil {
		return nil, err
	}
	return out.List, nil
}
