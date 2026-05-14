package installment

import (
	"context"
	"net/http"
)

// DailyReport returns the merchant's daily report of operations
// (issuances, returns, commissions) for a date in "YYYY-MM-DD"
// format.
//
// POST /api/store/report  (200 → DailyReportResponse)
func (c *Client) DailyReport(ctx context.Context, date string) ([]ReportOrder, error) {
	if date == "" {
		return nil, ErrEmptyDate
	}
	var out DailyReportResponse
	if err := c.doJSON(ctx, "/api/store/report",
		DailyReportRequest{Date: date}, &out, http.StatusOK); err != nil {
		return nil, err
	}
	return out.Orders, nil
}
