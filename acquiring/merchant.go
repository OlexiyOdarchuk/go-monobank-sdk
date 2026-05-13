package acquiring

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// MerchantDetails повертає профіль мерчанта: id, назву, ЄДРПОУ.
// Зручно для smoke-test токена.
// https://api.monobank.ua/docs/acquiring.html#tag/Vyklyki-dlya-mercha/paths/~1api~1merchant~1details/get
func (c *Client) MerchantDetails(ctx context.Context) (*MerchantDetails, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "/api/merchant/details", http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	var out MerchantDetails
	if err := c.c.Do(req, &out, http.StatusOK); err != nil {
		return nil, err
	}
	return &out, nil
}

// Employees перелічує активних співробітників мерчанта (наприклад,
// одержувачів чайових). ID із цього списку передається у
// CreateInvoiceRequest.TipsEmployeeID.
// https://api.monobank.ua/docs/acquiring.html#tag/Vyklyki-dlya-mercha/paths/~1api~1merchant~1employee~1list/get
func (c *Client) Employees(ctx context.Context) ([]Employee, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "/api/merchant/employee/list", http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	var out EmployeeList
	if err := c.c.Do(req, &out, http.StatusOK); err != nil {
		return nil, err
	}
	return out.List, nil
}

// PubKey повертає публічний ключ мерчанта (base64-кодований PEM x.509
// ECDSA, NIST P-256), яким верифікуються підписи вебхуків еквайрингу.
// Розбери поле Key через [ServerKey.Public] чи [ParsePubKey] і кешуй —
// банк ротейтить ключ рідко, але потенційно міняє. На відміну від
// /bank/sync — це окремий ключ для еквайрингових webhook-ів.
// https://api.monobank.ua/docs/acquiring.html#tag/Vyklyki-dlya-mercha/paths/~1api~1merchant~1pubkey/get
func (c *Client) PubKey(ctx context.Context) (*ServerKey, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "/api/merchant/pubkey", http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	var out ServerKey
	if err := c.c.Do(req, &out, http.StatusOK); err != nil {
		return nil, err
	}
	return &out, nil
}

// Submerchants перелічує субмерчантів, налаштованих під цим мерчантом.
// Subm-Code використовується у Statement-фільтрі та CreateInvoice.Code.
// https://api.monobank.ua/docs/acquiring.html#tag/Vyklyki-dlya-mercha/paths/~1api~1merchant~1submerchant~1list/get
func (c *Client) Submerchants(ctx context.Context) ([]Submerchant, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "/api/merchant/submerchant/list", http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	var out SubmerchantList
	if err := c.c.Do(req, &out, http.StatusOK); err != nil {
		return nil, err
	}
	return out.List, nil
}

// Statement повертає виписку за період для цього мерчанта. to-нульовий
// означає «до теперішнього часу». code (опційно) фільтрує по
// субмерчанту. Кожен рядок несе CancelList — історію повернень для
// конкретного інвойсу.
// https://api.monobank.ua/docs/acquiring.html#tag/Vyplaty-ta-zvirky/paths/~1api~1merchant~1statement/get
func (c *Client) Statement(ctx context.Context, from, to time.Time, code string) ([]StatementInvoice, error) {
	q := url.Values{}
	q.Set("from", strconv.FormatInt(from.Unix(), 10))
	if !to.IsZero() {
		q.Set("to", strconv.FormatInt(to.Unix(), 10))
	}
	if code != "" {
		q.Set("code", code)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		"/api/merchant/statement?"+q.Encode(), http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	var out StatementResponse
	if err := c.c.Do(req, &out, http.StatusOK); err != nil {
		return nil, err
	}
	return out.List, nil
}
