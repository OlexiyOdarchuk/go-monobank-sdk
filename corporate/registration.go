package corporate

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

// RegistrationState — стан запиту на реєстрацію корпоративного API.
type RegistrationState string

// Можливі значення RegistrationState.
const (
	// RegistrationNew — заявка створена, чекає ручного схвалення Mono.
	RegistrationNew RegistrationState = "New"
	// RegistrationDeclined — заявку відхилено.
	RegistrationDeclined RegistrationState = "Declined"
	// RegistrationApproved — заявку схвалено; KeyID готовий до використання.
	RegistrationApproved RegistrationState = "Approved"
)

// RegistrationRequest — body POST /personal/auth/registration: первинна
// заявка на схвалення корпоративного API. Всі поля обов'язкові.
//
// Pubkey — PEM-кодований публічний ключ secp256k1 (encoding/json
// автоматично base64-кодує []byte для wire-format). Logo — сирі байти
// зображення PNG/JPEG, теж base64-кодується.
type RegistrationRequest struct {
	Pubkey        []byte `json:"pubkey"`
	Name          string `json:"name"`
	Description   string `json:"description"`
	ContactPerson string `json:"contactPerson"`
	Phone         string `json:"phone"`
	Email         string `json:"email"`
	Logo          []byte `json:"logo"`
}

// RegistrationResponse — відповідь POST /personal/auth/registration:
// підтверджує, що заявку прийнято (зазвичай Status == "New").
type RegistrationResponse struct {
	Status RegistrationState `json:"status"`
}

// RegistrationStatusRequest — body POST /personal/auth/registration/status:
// той самий PEM-pubkey, який було подано при реєстрації — Mono використовує
// його як ідентифікатор заявки.
type RegistrationStatusRequest struct {
	Pubkey []byte `json:"pubkey"`
}

// RegistrationStatusResponse — відповідь
// POST /personal/auth/registration/status. KeyID заповнюється, коли
// Mono схвалила заявку, і відповідає X-Key-Id, який корпоративний клієнт
// зобов'язаний використовувати надалі.
type RegistrationStatusResponse struct {
	Status RegistrationState `json:"status"`
	KeyID  string            `json:"keyId"`
}

// Register відправляє заявку на реєстрацію корпоративного API. Mono
// розглядає її вручну (від кількох годин до кількох днів). Перевіряй
// статус через [Client.RegistrationStatus].
// https://api.monobank.ua/docs/corporate.html#tag/Avtoryzaciya-ta-nalashtuvannya-kompaniyi/paths/~1personal~1auth~1registration/post
func (c *Client) Register(ctx context.Context, in *RegistrationRequest) (*RegistrationResponse, error) {
	if in == nil {
		return nil, ErrNilRequest
	}
	body, err := json.Marshal(in)
	if err != nil {
		return nil, fmt.Errorf("marshal: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		"/personal/auth/registration", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	var out RegistrationResponse
	if err := c.do(req, c.authMaker.New(""), &out, http.StatusOK); err != nil {
		return nil, err
	}
	return &out, nil
}

// RegistrationStatus перевіряє, чи заявка, подана з pubkeyPEM, схвалена
// банком. Полінг 1 раз на хвилину-годину — нормальна стратегія
// (схвалення ручне, поспішати немає сенсу).
// https://api.monobank.ua/docs/corporate.html#tag/Avtoryzaciya-ta-nalashtuvannya-kompaniyi/paths/~1personal~1auth~1registration~1status/post
func (c *Client) RegistrationStatus(ctx context.Context, pubkeyPEM []byte) (*RegistrationStatusResponse, error) {
	body, err := json.Marshal(RegistrationStatusRequest{Pubkey: pubkeyPEM})
	if err != nil {
		return nil, fmt.Errorf("marshal: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		"/personal/auth/registration/status", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	var out RegistrationStatusResponse
	if err := c.do(req, c.authMaker.New(""), &out, http.StatusOK); err != nil {
		return nil, err
	}
	return &out, nil
}
