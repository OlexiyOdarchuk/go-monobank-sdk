package business

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
)

// UploadPayslips завантажує пакет розрахункових листів за період
// (YYYY-MM). До 1000 співробітників на пакет; можна викликати
// багаторазово на той самий період — статистика акумулюється у
// OverallStats. FailedEmployees містить, кого не вдалось знайти у
// довіднику контактів (потрібен попередній [Client.CreateContact]).
// https://corp-api.monobank.ua/docs/#operation/batchUploadPayslips
func (c *Client) UploadPayslips(ctx context.Context, in *BatchPayslipRequest) (*BatchPayslipResponse, error) {
	if in == nil {
		return nil, ErrNilRequest
	}
	body, err := json.Marshal(in)
	if err != nil {
		return nil, fmt.Errorf("marshal: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		"/ext/v1/payslips/batch", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	var wrap resultWrapper[BatchPayslipResponse]
	if err := c.c.Do(req, &wrap, http.StatusOK); err != nil {
		return nil, err
	}
	return &wrap.Result, nil
}

// DeletePayslips видаляє завантажені розрахункові листи за період для
// конкретних співробітників (за ІПН або номером паспорта). Для повного
// видалення імпорту за період — [Client.DeleteImport].
// https://corp-api.monobank.ua/docs/#operation/batchDeletePayslips
func (c *Client) DeletePayslips(ctx context.Context, in *DeletePayslipsRequest) error {
	if in == nil {
		return ErrNilRequest
	}
	body, err := json.Marshal(in)
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete,
		"/ext/v1/payslips/batch", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	return c.c.Do(req, nil, http.StatusOK)
}

// ImportStatus повертає кумулятивний стан імпорту розрахункових листів
// за період (YYYY-MM): скільки співробітників, скільки помилок, статус
// LOADING/LOADED/FAILED/SENT/DELETED.
// https://corp-api.monobank.ua/docs/#operation/getImportStatus
func (c *Client) ImportStatus(ctx context.Context, period string) (*ImportStatusResponse, error) {
	q := url.Values{}
	q.Set("period", period)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		"/ext/v1/payslip-imports/status?"+q.Encode(), http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	var wrap resultWrapper[ImportStatusResponse]
	if err := c.c.Do(req, &wrap, http.StatusOK); err != nil {
		return nil, err
	}
	return &wrap.Result, nil
}

// DeleteImport повністю видаляє імпорт розрахункових листів за період.
// На відміну від [Client.DeletePayslips], стирає взагалі все за вказаний
// YYYY-MM, а не для окремих співробітників.
// https://corp-api.monobank.ua/docs/#operation/deleteImport
func (c *Client) DeleteImport(ctx context.Context, period string) error {
	q := url.Values{}
	q.Set("period", period)
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete,
		"/ext/v1/payslip-imports?"+q.Encode(), http.NoBody)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	return c.c.Do(req, nil, http.StatusOK)
}

// SendPayslipsToMobile відправляє розрахункові листи за період у
// mobile-апи співробітникам. Викликається після успішного [Client.UploadPayslips].
// Повертає EmployeesSent — скільки фактично отримало push.
// https://corp-api.monobank.ua/docs/#operation/sendToMobile
func (c *Client) SendPayslipsToMobile(ctx context.Context, period string) (*SendResult, error) {
	q := url.Values{}
	q.Set("period", period)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		"/ext/v1/payslip-imports/send?"+q.Encode(), http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	var wrap resultWrapper[SendResult]
	if err := c.c.Do(req, &wrap, http.StatusOK); err != nil {
		return nil, err
	}
	return &wrap.Result, nil
}

// PayslipPDF повертає байти PDF-розрахункового листа конкретного
// співробітника за період. Користувач сам зберігає їх куди треба
// (файл, blob storage, response.Write).
// https://corp-api.monobank.ua/docs/#operation/generatePayslipPdfByIdentification
func (c *Client) PayslipPDF(ctx context.Context, identification, period string) ([]byte, error) {
	q := url.Values{}
	q.Set("identification", identification)
	q.Set("period", period)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		"/ext/v1/payslips/pdf?"+q.Encode(), http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Accept", "application/pdf")

	var body []byte
	if err := c.c.Do(req, &body, http.StatusOK); err != nil {
		return nil, err
	}
	return body, nil
}
