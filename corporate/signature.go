package corporate

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
)

// DocumentStatus — стан окремого документа у заявці monoКЕП.
type DocumentStatus string

// Можливі значення DocumentStatus.
const (
	// DocPending — документ чекає підпису.
	DocPending DocumentStatus = "pending"
	// DocSigned — документ підписано всіма необхідними підписантами.
	DocSigned DocumentStatus = "signed"
	// DocCanceled — заявку скасовано (клієнтом або через
	// [Client.SignatureCancel]).
	DocCanceled DocumentStatus = "canceled"
	// DocExpired — минув термін дії заявки (3 доби).
	DocExpired DocumentStatus = "expired"
)

// Document описує документ для підпису (на запиті) або його поточний
// стан (на відповіді). Status і Signers заповнені тільки у відповідях
// [Client.SignatureStatus]; Type і Link необов'язкові на запиті.
//
// Hash — геш документа у HEX (алгоритм ГОСТ 34.311-95). Type — одне з
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

// Signer — одна сторона, що підписала документ через monoКЕП. TIN —
// РНОКПП (ідентифікаційний номер фізичної особи); EDRPOU/Company/Post
// заповнюються, коли підписант підписував від імені юридичної особи.
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

// SignatureCreateRequest — body POST /personal/signature/create.
// Documents має містити 1..10 елементів. OneSigner за замовчуванням
// true; false дозволяє кільком підписантам підписувати документи.
// CallbackURL (опційно) — Mono POST-не на нього при зміні статусу.
type SignatureCreateRequest struct {
	Documents   []Document `json:"documents"`
	OneSigner   *bool      `json:"oneSigner,omitempty"`
	CallbackURL string     `json:"callbackUrl,omitempty"`
}

// SignatureCreateResponse — відповідь /create. RequestID — для
// подальшого опитування статусу/скасування. Deeplink — посилання, яке
// підписант відкриває у mobile-апі Mono.
type SignatureCreateResponse struct {
	RequestID string `json:"requestId"`
	Deeplink  string `json:"deeplink"`
}

// SignatureStatusResponse — body GET /personal/signature/status: масив
// документів із поточним статусом і даними підписантів.
type SignatureStatusResponse struct {
	Documents []Document `json:"documents"`
}

// SignatureCreate створює заявку на підписання документів через
// monoКЕП. Термін дії заявки — 3 доби. Після створення передай
// [SignatureCreateResponse.Deeplink] підписанту (як QR-код або
// клікабельне посилання).
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

// SignatureStatus повертає поточний стан заявки monoКЕП за requestID:
// статус кожного документа і дані підписантів, якщо вже хтось підписав.
// https://api.monobank.ua/docs/corporate.html#tag/monoKEP/paths/~1personal~1signature~1status?requestId={requestId}/get
func (c *Client) SignatureStatus(ctx context.Context, requestID string) (*SignatureStatusResponse, error) {
	uri := "/personal/signature/status?requestId=" + url.QueryEscape(requestID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, uri, http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	var out SignatureStatusResponse
	if err := c.do(req, c.authMaker.New(""), &out, http.StatusOK); err != nil {
		return nil, err
	}
	return &out, nil
}

// SignatureCancel скасовує очікувану заявку monoКЕП. Після скасування
// усі документи заявки переходять у DocCanceled і deeplink перестає
// працювати.
// https://api.monobank.ua/docs/corporate.html#tag/monoKEP/paths/~1personal~1signature~1cancel?requestId={requestId}/delete
func (c *Client) SignatureCancel(ctx context.Context, requestID string) error {
	uri := "/personal/signature/cancel?requestId=" + url.QueryEscape(requestID)
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, uri, http.NoBody)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	return c.do(req, c.authMaker.New(""), nil, http.StatusOK)
}
