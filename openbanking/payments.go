package openbanking

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
)

// paymentsPath is the payment-initiation collection; the payment
// product is the next segment.
const paymentsPath = pathPrefix + "/v2/payments"

// CreatePaymentRequest is the single-payment body (the Berlin Group
// singlePaymentCore). Creditor, CreditorAccount, InstructedAmount and
// DebtorAccount are mandatory.
type CreatePaymentRequest struct {
	// InstructedAmount is the amount to transfer; its currency must
	// match both accounts, otherwise the bank answers
	// [CodeCurrencyMismatch].
	InstructedAmount Amount `json:"instructedAmount"`
	// CreditorAccount is the beneficiary's account.
	CreditorAccount PaymentAccountReference `json:"creditorAccount"`
	// Creditor names and identifies the beneficiary.
	Creditor Creditor `json:"creditor"`
	// DebtorAccount is the payer's account — the one the payment is
	// authorised from. The spec states no ownership rule beyond that.
	DebtorAccount PaymentAccountReference `json:"debtorAccount"`
	// RemittanceInformationUnstructured is the payment purpose. The
	// spec allows exactly one element of at most 140 characters.
	RemittanceInformationUnstructured []string `json:"remittanceInformationUnstructured,omitempty"`
}

// PaymentLinks is the _links object of a created payment.
type PaymentLinks struct {
	// StartAuthorisation is the URL behind
	// [Client.StartPaymentAuthorisation]; informational, the SDK
	// builds the path itself.
	StartAuthorisation *Href `json:"startAuthorisation,omitempty"`
}

// CreatePaymentResponse is the body of a successful
// [Client.CreatePayment].
type CreatePaymentResponse struct {
	TransactionStatus TransactionStatus `json:"transactionStatus"`
	PaymentID         string            `json:"paymentId"`
	Links             PaymentLinks      `json:"_links,omitzero"`
}

// PaymentStatusResponse is the status-only projection of a payment.
type PaymentStatusResponse struct {
	TransactionStatus TransactionStatus `json:"transactionStatus"`
}

// Payment is the stored state of an initiated payment.
type Payment struct {
	TransactionStatus TransactionStatus       `json:"transactionStatus"`
	PaymentID         string                  `json:"paymentId"`
	InstructedAmount  Amount                  `json:"instructedAmount,omitzero"`
	CreditorAccount   PaymentAccountReference `json:"creditorAccount,omitzero"`
	Creditor          Creditor                `json:"creditor,omitzero"`
	DebtorAccount     PaymentAccountReference `json:"debtorAccount,omitzero"`
}

// PaymentAuthorisationsResponse lists the authorisation processes
// started for a payment.
type PaymentAuthorisationsResponse struct {
	AuthorisationIDs []string `json:"authorisationIds"`
}

// AuthorisationStatusResponse is the SCA status of one authorisation.
type AuthorisationStatusResponse struct {
	SCAStatus SCAStatus `json:"scaStatus"`
}

// paymentPath builds /v2/payments/{product}/{paymentId} and validates
// both segments. suffix is appended verbatim ("", "/status",
// "/authorisations", …).
func paymentPath(product PaymentProduct, paymentID, suffix string) (string, error) {
	if badPathSegment(string(product)) || badPathSegment(paymentID) {
		return "", ErrEmptyID
	}

	return paymentsPath + "/" + url.PathEscape(string(product)) +
		"/" + url.PathEscape(paymentID) + suffix, nil
}

// CreatePayment initiates a single credit transfer. The payment is
// only registered here — no money moves until the PSU authorises it
// through [Client.StartPaymentAuthorisation].
//
// product selects the rail: [CreditTransfers] or
// [InstantCreditTransfers]. opts.PSU.IPAddress is mandatory.
func (c *Client) CreatePayment(
	ctx context.Context, product PaymentProduct, in *CreatePaymentRequest, opts InitiationOptions,
) (*CreatePaymentResponse, error) {
	if badPathSegment(string(product)) {
		return nil, ErrEmptyID
	}
	if in == nil {
		return nil, ErrNilRequest
	}
	if opts.PSU.IPAddress == "" {
		return nil, ErrMissingPSUIPAddress
	}
	body, err := json.Marshal(in)
	if err != nil {
		return nil, fmt.Errorf("marshal: %w", err)
	}
	uri := paymentsPath + "/" + url.PathEscape(string(product))
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, uri, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	opts.PSU.apply(req.Header)
	setIfNotEmpty(req.Header, HeaderTPPRedirectURI, opts.TPPRedirectURI)

	var out CreatePaymentResponse
	if err := c.c.Do(req, &out, http.StatusCreated); err != nil {
		return nil, err
	}

	return &out, nil
}

// Payment reads back a payment: its status, amount and both parties.
func (c *Client) Payment(ctx context.Context, product PaymentProduct, paymentID string) (*Payment, error) {
	uri, err := paymentPath(product, paymentID, "")
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, uri, http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	var out Payment
	if err := c.c.Do(req, &out, http.StatusOK); err != nil {
		return nil, err
	}

	return &out, nil
}

// PaymentStatus reads only the transaction status. Use it for
// polling after an authorisation: the terminal success state is
// [StatusAcceptedCreditSettlementCompleted], and
// [StatusRejected] / [StatusCancelled] are terminal failures.
func (c *Client) PaymentStatus(
	ctx context.Context, product PaymentProduct, paymentID string,
) (*PaymentStatusResponse, error) {
	uri, err := paymentPath(product, paymentID, "/status")
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, uri, http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	var out PaymentStatusResponse
	if err := c.c.Do(req, &out, http.StatusOK); err != nil {
		return nil, err
	}

	return &out, nil
}

// CancelPayment cancels a payment; [StatusCancelled] is the state
// the spec defines for that outcome. The bank answers 204 with no
// body. The spec states no precondition, so which payments are still
// cancellable is up to the bank — check the status afterwards.
func (c *Client) CancelPayment(ctx context.Context, product PaymentProduct, paymentID string) error {
	uri, err := paymentPath(product, paymentID, "")
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, uri, http.NoBody)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	return c.c.Do(req, nil, http.StatusNoContent)
}

// StartPaymentAuthorisation opens the SCA process for a payment.
//
// Under [SCARedirect] the response carries Links.SCARedirect — send
// the PSU there. Under [SCADecoupled] it does not: the PSU confirms
// in their own banking app, and the TPP polls
// [Client.PaymentAuthorisationStatus] (or [Client.PaymentStatus])
// instead. opts.SCAApproachPreference is only a hint; the bank picks
// the approach from the customer type.
func (c *Client) StartPaymentAuthorisation(
	ctx context.Context, product PaymentProduct, paymentID string, opts AuthorisationOptions,
) (*StartAuthorisationResponse, error) {
	uri, err := paymentPath(product, paymentID, "/authorisations")
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, uri, http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	setIfNotEmpty(req.Header, HeaderTPPSCAApproachPreference, string(opts.SCAApproachPreference))

	var out StartAuthorisationResponse
	if err := c.c.Do(req, &out, http.StatusCreated); err != nil {
		return nil, err
	}

	return &out, nil
}

// PaymentAuthorisations lists the ids of the authorisation processes
// started for a payment. The spec does not say when a payment gets
// more than one, so do not assume the list has a single element.
func (c *Client) PaymentAuthorisations(
	ctx context.Context, product PaymentProduct, paymentID string,
) ([]string, error) {
	uri, err := paymentPath(product, paymentID, "/authorisations")
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, uri, http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	var out PaymentAuthorisationsResponse
	if err := c.c.Do(req, &out, http.StatusOK); err != nil {
		return nil, err
	}

	return out.AuthorisationIDs, nil
}

// PaymentAuthorisationStatus reads the SCA status of one
// authorisation. It is the polling endpoint of the [SCADecoupled]
// flow, where there is no redirect to bring the PSU back.
func (c *Client) PaymentAuthorisationStatus(
	ctx context.Context, product PaymentProduct, paymentID, authorisationID string,
) (*AuthorisationStatusResponse, error) {
	if badPathSegment(authorisationID) {
		return nil, ErrEmptyID
	}
	uri, err := paymentPath(product, paymentID, "/authorisations/"+url.PathEscape(authorisationID))
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, uri, http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	var out AuthorisationStatusResponse
	if err := c.c.Do(req, &out, http.StatusOK); err != nil {
		return nil, err
	}

	return &out, nil
}
