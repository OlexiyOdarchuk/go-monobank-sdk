package business

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
)

// CreateSalaryRegistry creates a new salary registry. idempotencyKey
// is a UUID v4 (recommended) that lets you safely retry the call on
// network failures: a repeat with the same key will not create a
// duplicate.
// https://corp-api.monobank.ua/docs/#operation/create-salary-registries
func (c *Client) CreateSalaryRegistry(ctx context.Context, idempotencyKey string,
	in *CreateSalaryRegistryRequest) (*SalaryRegistryCreated, error) {

	if in == nil {
		return nil, ErrNilRequest
	}
	body, err := json.Marshal(in)
	if err != nil {
		return nil, fmt.Errorf("marshal: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		"/ext/v1/payments/salary/registries", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Idempotency-Key", idempotencyKey)

	var out SalaryRegistryCreated
	if err := c.c.Do(req, &out, http.StatusOK); err != nil {
		return nil, err
	}
	return &out, nil
}

// SalaryRegistryTypes returns the list of available salary-registry
// types (for example, SALARY_ADVANCE — "salary advance"). One of
// these aliases must be passed in
// CreateSalaryRegistryRequest.SalaryRegistryType.
// https://corp-api.monobank.ua/docs/#operation/get-salary-registries-types
func (c *Client) SalaryRegistryTypes(ctx context.Context) ([]SalaryRegistryType, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		"/ext/v1/payments/salary/registries/types", http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	var out []SalaryRegistryType
	if err := c.c.Do(req, &out, http.StatusOK); err != nil {
		return nil, err
	}
	return out, nil
}

// SalaryRegistryStatus returns the current state of a salary
// registry by its ID. For terminal statuses (FAIL / PAYMENTS_DONE)
// DeclineReason explains what went wrong (when FAIL).
// https://corp-api.monobank.ua/docs/#operation/get-salary-registries-status
func (c *Client) SalaryRegistryStatus(ctx context.Context, id string) (*SalaryRegistryStatus, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		"/ext/v1/payments/salary/registries/"+url.PathEscape(id)+"/status", http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	var out SalaryRegistryStatus
	if err := c.c.Do(req, &out, http.StatusOK); err != nil {
		return nil, err
	}
	return &out, nil
}
