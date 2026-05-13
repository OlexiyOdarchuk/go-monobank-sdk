package business

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
)

// CreateSalaryRegistry створює нову зарплатну відомість. idempotencyKey —
// UUID v4 (рекомендовано), що дозволяє безпечно повторювати виклик при
// помилках мережі: запит з однаковим ключем не створить дублікат.
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

// SalaryRegistryTypes повертає список доступних типів зарплатних
// відомостей (наприклад SALARY_ADVANCE — «аванс по зарплаті»). Один із
// цих alias-ів треба передати у CreateSalaryRegistryRequest.SalaryRegistryType.
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

// SalaryRegistryStatus повертає поточний стан зарплатної відомості за
// її ID. Для термінальних статусів (FAIL / PAYMENTS_DONE) DeclineReason
// розкаже, що пішло не так (якщо FAIL).
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
