package openbanking

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

// certRequestPath is the test-certificate desk.
const certRequestPath = pathPrefix + "/test-integration/cert-request"

// CertificateStatus is the state of a test-certificate application.
type CertificateStatus string

// Possible CertificateStatus values.
const (
	// CertPending — the application is queued for review.
	CertPending CertificateStatus = "pending"
	// CertApproved — the certificate is issued and present in
	// [CertificateStatusResponse.Certificate].
	CertApproved CertificateStatus = "approved"
	// CertRejected — declined;
	// [CertificateStatusResponse.RejectReason] says why.
	CertRejected CertificateStatus = "rejected"
)

// CertificateRequest applies for a test QWAC. Every field is
// mandatory.
type CertificateRequest struct {
	// Organization is the applicant's legal name (max 500 chars).
	Organization string `json:"organization"`
	// ContactPerson is who to come back to (max 200 chars).
	ContactPerson string `json:"contactPerson"`
	// Phone of the contact person (max 50 chars).
	Phone string `json:"phone"`
	// Email of the contact person (max 200 chars).
	Email string `json:"email"`
	// Description explains the services and the purpose of the
	// integration (max 2000 chars); a human reads it.
	Description string `json:"description"`
	// EDRPOU is the organisation's ЄДРПОУ code (max 10 chars).
	EDRPOU string `json:"edrpou"`
	// CSRPEM is the certificate signing request, PEM-encoded and then
	// base64-encoded. The spec requires the CSR to be signed with
	// ECDSA. Keep the exact string: it is also the lookup key of
	// [Client.TestCertificateStatus].
	CSRPEM string `json:"csrPem"`
}

// CertificateRequestResponse acknowledges a filed application.
type CertificateRequestResponse struct {
	// Status is [CertPending] on a fresh application.
	Status CertificateStatus `json:"status"`
}

// CertificateStatusResponse reports on a filed application and, once
// approved, carries the issued certificate.
type CertificateStatusResponse struct {
	Status CertificateStatus `json:"status"`
	// Certificate is the issued certificate in PEM format, present
	// only for [CertApproved].
	Certificate string `json:"certificate,omitempty"`
	// RejectReason is present only for [CertRejected].
	RejectReason string `json:"rejectReason,omitempty"`
}

// RequestTestCertificate files an application for a test QWAC.
//
// Sandbox only. This is the one pair of operations in the API that
// needs no mTLS — it is how a TPP obtains the certificate in the
// first place — and it lives on [BaseURLSandbox], so point a
// dedicated client at that host with an ordinary *http.Client rather
// than reusing the mTLS one:
//
//	onboarding := openbanking.New(monobank.WithBaseURL(openbanking.BaseURLSandbox))
//	_, err := onboarding.RequestTestCertificate(ctx, &openbanking.CertificateRequest{…})
//
// An operator reviews the application by hand, so expect
// [CertPending] for a while and poll [Client.TestCertificateStatus].
// The endpoint is rate limited: HTTP 429 (errors.Is against
// [monobank.ErrTooManyRequests]) means back off, not retry.
//
// Per the spec the issued test certificate carries the PSD2 roles
// PSP_AI and PSP_PI, is valid for two years, and works against
// [BaseURLStage].
func (c *Client) RequestTestCertificate(
	ctx context.Context, in *CertificateRequest,
) (*CertificateRequestResponse, error) {
	if in == nil {
		return nil, ErrNilRequest
	}
	body, err := json.Marshal(in)
	if err != nil {
		return nil, fmt.Errorf("marshal: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, certRequestPath, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	var out CertificateRequestResponse
	if err := c.c.Do(req, &out, http.StatusCreated); err != nil {
		return nil, err
	}

	return &out, nil
}

// TestCertificateStatus polls a test-certificate application and, on
// [CertApproved], returns the issued certificate.
//
// Sandbox only, and — like [Client.RequestTestCertificate] — served
// from [BaseURLSandbox] without mTLS. The application is identified
// by the very CSR that was submitted, so keep that string around;
// there is no application id. An unknown CSR comes back as HTTP 404
// (errors.Is against [monobank.ErrNotFound]).
func (c *Client) TestCertificateStatus(ctx context.Context, csrPEM string) (*CertificateStatusResponse, error) {
	if csrPEM == "" {
		return nil, ErrEmptyID
	}
	body, err := json.Marshal(struct {
		CSRPEM string `json:"csrPem"`
	}{CSRPEM: csrPEM})
	if err != nil {
		return nil, fmt.Errorf("marshal: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, certRequestPath+"/status", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	var out CertificateStatusResponse
	if err := c.c.Do(req, &out, http.StatusOK); err != nil {
		return nil, err
	}

	return &out, nil
}
