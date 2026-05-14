package corporate

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"
)

// SignatureRequestTTL is how long a monoKEP request stays valid on
// Mono's side. After this window every unsigned document moves to
// [DocExpired] and the deeplink stops working. Used in godoc and
// available to callers who poll status.
const SignatureRequestTTL = 72 * time.Hour

// DocumentStatus is the state of a single document in a monoKEP
// request.
type DocumentStatus string

// Possible DocumentStatus values.
const (
	// DocPending — the document is awaiting a signature.
	DocPending DocumentStatus = "pending"
	// DocSigned — the document has been signed by every required
	// signer.
	DocSigned DocumentStatus = "signed"
	// DocCanceled — the request was canceled (by the client or via
	// [Client.SignatureCancel]).
	DocCanceled DocumentStatus = "canceled"
	// DocExpired — the request has expired ([SignatureRequestTTL]).
	DocExpired DocumentStatus = "expired"
)

// Document describes a document to sign (on a request) or its
// current state (on a response). Status and Signers are populated
// only in [Client.SignatureStatus] responses; Type and Link are
// optional on the request.
//
// Hash is the document hash in HEX (GOST 34.311-95). Type is one of
// "pdf", "doc", "docx", "odt", "json", "xml", "html", "png", "jpg",
// "jpeg", "other".
type Document struct {
	Name    string         `json:"name"`
	Hash    string         `json:"hash"`
	Type    string         `json:"type,omitempty"`
	Link    string         `json:"link,omitempty"`
	Status  DocumentStatus `json:"status,omitempty"`
	Signers []Signer       `json:"signers,omitempty"`
}

// Signer is a single party that signed the document via monoKEP.
// TIN is the РНОКПП (a personal taxpayer ID); EDRPOU/Company/Post
// are populated when the signer signed on behalf of a legal entity.
type Signer struct {
	Name       string `json:"name"`
	TIN        string `json:"tin"`
	CertSerial string `json:"certSerial"`
	Signature  string `json:"signature"` // base64
	Date       string `json:"date"`      // ISO-8601
	EDRPOU     string `json:"edrpou,omitempty"`
	Company    string `json:"company,omitempty"`
	Post       string `json:"post,omitempty"`
}

// SignatureCreateRequest is the body of POST
// /personal/signature/create. Documents must contain 1..10 elements.
// OneSigner defaults to true; false lets multiple signers sign the
// documents. CallbackURL (optional) — Mono POSTs to it on status
// changes.
type SignatureCreateRequest struct {
	Documents   []Document `json:"documents"`
	OneSigner   *bool      `json:"oneSigner,omitempty"`
	CallbackURL string     `json:"callbackUrl,omitempty"`
}

// SignatureCreateResponse is the response of /create. RequestID is
// used for subsequent status polling / cancellation. Deeplink is the
// link the signer opens in the Mono mobile app.
type SignatureCreateResponse struct {
	RequestID string `json:"requestId"`
	Deeplink  string `json:"deeplink"`
}

// SignatureStatusResponse is the body of GET
// /personal/signature/status: an array of documents with their
// current status and signer data.
type SignatureStatusResponse struct {
	Documents []Document `json:"documents"`
}

// SignatureCreate creates a request to sign documents via monoKEP.
// The request is valid for 3 days. After creation pass
// [SignatureCreateResponse.Deeplink] to the signer (as a QR code or
// a clickable link).
// https://api.monobank.ua/docs/corporate.html#tag/monoKEP/paths/~1personal~1signature~1create/post
func (c *Client) SignatureCreate(ctx context.Context, in *SignatureCreateRequest) (*SignatureCreateResponse, error) {
	if in == nil {
		return nil, ErrNilRequest
	}
	body, err := json.Marshal(in)
	if err != nil {
		return nil, fmt.Errorf("marshal: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		"/personal/signature/create", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	var out SignatureCreateResponse
	if err := c.do(req, c.authMaker.New(""), &out, http.StatusOK); err != nil {
		return nil, err
	}
	return &out, nil
}

// SignatureStatus returns the current state of a monoKEP request by
// requestID: the status of each document and signer data when
// somebody has already signed.
// https://api.monobank.ua/docs/corporate.html#tag/monoKEP/paths/~1personal~1signature~1status?requestId={requestId}/get
func (c *Client) SignatureStatus(ctx context.Context, requestID string) (*SignatureStatusResponse, error) {
	u := url.URL{
		Path:     "/personal/signature/status",
		RawQuery: url.Values{"requestId": {requestID}}.Encode(),
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.RequestURI(), http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	var out SignatureStatusResponse
	if err := c.do(req, c.authMaker.New(""), &out, http.StatusOK); err != nil {
		return nil, err
	}
	return &out, nil
}

// SignatureCancel cancels a pending monoKEP request. After
// cancellation every document in the request moves to DocCanceled
// and the deeplink stops working.
// https://api.monobank.ua/docs/corporate.html#tag/monoKEP/paths/~1personal~1signature~1cancel?requestId={requestId}/delete
func (c *Client) SignatureCancel(ctx context.Context, requestID string) error {
	u := url.URL{
		Path:     "/personal/signature/cancel",
		RawQuery: url.Values{"requestId": {requestID}}.Encode(),
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, u.RequestURI(), http.NoBody)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	return c.do(req, c.authMaker.New(""), nil, http.StatusOK)
}
