package business

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
)

// ErrBatchSize is returned by [Client.SubmitSalaryRegistryBatch] when
// a batch carries no recipients or more than
// [MaxSalaryBatchRecipients].
var ErrBatchSize = errors.New("business: salary batch size out of range")

// ErrEmptyRegistryReference is returned by
// [Client.FinalizeSalaryRegistry] when registryReference is empty.
var ErrEmptyRegistryReference = errors.New("business: registry reference is required")

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
	if idempotencyKey == "" {
		return nil, ErrIdempotencyKeyRequired
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

// SubmitSalaryRegistryBatch uploads up to [MaxSalaryBatchRecipients]
// recipients into a salary registry. The first call with a new
// RegistryReference creates the registry in [RegistryAPILoading];
// later calls with the same reference append batches to it. Call
// [Client.FinalizeSalaryRegistry] once every batch is uploaded.
// idempotencyKey is a UUID v4 that makes a retry of the same batch
// safe.
// https://corp-api.monobank.ua/docs/#operation/submit-salary-registry-batch
func (c *Client) SubmitSalaryRegistryBatch(ctx context.Context, idempotencyKey string,
	in *SubmitSalaryRegistryBatchRequest) (*SalaryRegistryBatchAccepted, error) {

	if in == nil {
		return nil, ErrNilRequest
	}
	if idempotencyKey == "" {
		return nil, ErrIdempotencyKeyRequired
	}
	if n := len(in.Recipients); n == 0 || n > MaxSalaryBatchRecipients {
		return nil, fmt.Errorf("%w: %d recipients, want 1..%d",
			ErrBatchSize, n, MaxSalaryBatchRecipients)
	}
	body, err := json.Marshal(in)
	if err != nil {
		return nil, fmt.Errorf("marshal: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		"/ext/v2/payments/salary/registries", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Idempotency-Key", idempotencyKey)

	var out SalaryRegistryBatchAccepted
	if err := c.c.Do(req, &out, http.StatusOK); err != nil {
		return nil, err
	}
	return &out, nil
}

// FinalizeSalaryRegistry closes a registry uploaded with
// [Client.SubmitSalaryRegistryBatch]. It moves straight to
// [RegistryAdded] when every batch is processed and otherwise stays in
// [RegistryAPILoading] until they are.
// https://corp-api.monobank.ua/docs/#operation/finalize-salary-registry-batch
func (c *Client) FinalizeSalaryRegistry(ctx context.Context, registryReference string) error {
	if registryReference == "" {
		return ErrEmptyRegistryReference
	}
	body, err := json.Marshal(struct {
		RegistryReference string `json:"registryReference"`
	}{registryReference})
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		"/ext/v2/payments/salary/registries/finalize", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	return c.c.Do(req, nil, http.StatusOK)
}
