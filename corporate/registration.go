package corporate

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

// RegistrationState is the state of a corporate-API registration
// request.
type RegistrationState string

// Possible RegistrationState values.
const (
	// RegistrationNew — the application has been created and is
	// awaiting Mono's manual approval.
	RegistrationNew RegistrationState = "New"
	// RegistrationDeclined — the application was declined.
	RegistrationDeclined RegistrationState = "Declined"
	// RegistrationApproved — the application is approved; KeyID is
	// ready for use.
	RegistrationApproved RegistrationState = "Approved"
)

// RegistrationRequest is the body of POST /personal/auth/registration:
// the initial application for corporate-API approval. All fields are
// required.
//
// Pubkey is a PEM-encoded secp256k1 public key (encoding/json
// automatically base64-encodes []byte for the wire format). Logo is
// the raw bytes of a PNG/JPEG image, also base64-encoded.
type RegistrationRequest struct {
	Pubkey        []byte `json:"pubkey"`
	Name          string `json:"name"`
	Description   string `json:"description"`
	ContactPerson string `json:"contactPerson"`
	Phone         string `json:"phone"`
	Email         string `json:"email"`
	Logo          []byte `json:"logo"`
}

// RegistrationResponse is the response of POST
// /personal/auth/registration: it confirms the application was
// accepted (typically Status == "New").
type RegistrationResponse struct {
	Status RegistrationState `json:"status"`
}

// RegistrationStatusRequest is the body of POST
// /personal/auth/registration/status: the same PEM pubkey that was
// submitted at registration — Mono uses it as the application
// identifier.
type RegistrationStatusRequest struct {
	Pubkey []byte `json:"pubkey"`
}

// RegistrationStatusResponse is the response of POST
// /personal/auth/registration/status. KeyID is populated once Mono
// approves the application and matches the X-Key-Id the corporate
// client must use from then on.
type RegistrationStatusResponse struct {
	Status RegistrationState `json:"status"`
	KeyID  string            `json:"keyId"`
}

// Register submits a corporate-API registration application. Mono
// reviews it manually (from a few hours to a few days). Poll status
// via [Client.RegistrationStatus].
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

// RegistrationStatus checks whether the application submitted with
// pubkeyPEM has been approved by the bank. Polling once a minute to
// once an hour is a normal strategy (approval is manual, no point in
// rushing).
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
