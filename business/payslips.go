package business

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
)

// UploadPayslips uploads a batch of payslips for a period (YYYY-MM).
// Up to 1000 employees per batch; you can call it multiple times for
// the same period — the statistics accumulate in OverallStats.
// FailedEmployees lists those that could not be found in the contacts
// directory (a prior [Client.CreateContact] is required).
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

// DeletePayslips removes uploaded payslips for a period for specific
// employees (by INN or passport number). For full removal of the
// period's import use [Client.DeleteImport].
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

// ImportStatus returns the cumulative state of the payslip import
// for a period (YYYY-MM): number of employees, number of errors, and
// status LOADING/LOADED/FAILED/SENT/DELETED.
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

// DeleteImport fully removes the payslip import for a period. Unlike
// [Client.DeletePayslips], it wipes everything for the given YYYY-MM,
// not just individual employees.
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

// SendPayslipsToMobile sends payslips for the period to employees'
// mobile apps. Call after a successful [Client.UploadPayslips].
// Returns EmployeesSent — how many actually received the push.
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

// PayslipPDF returns the PDF bytes of a specific employee's payslip
// for a period. The caller is responsible for storing them as needed
// (file, blob storage, response.Write).
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
