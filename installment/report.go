package installment

import (
	"context"
	"net/http"
)

// DailyReport повертає денний звіт операцій (видачі, повернення,
// комісії) для магазину за дату у форматі "YYYY-MM-DD".
//
// POST /api/store/report  (200 → DailyReportResponse)
func (c *Client) DailyReport(ctx context.Context, date string) ([]ReportOrder, error) {
	var out DailyReportResponse
	if err := c.doJSON(ctx, "/api/store/report",
		DailyReportRequest{Date: date}, &out, http.StatusOK); err != nil {
		return nil, err
	}
	return out.Orders, nil
}
