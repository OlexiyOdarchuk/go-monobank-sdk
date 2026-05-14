package acquiring

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

// MonoPayKey is the merchant's public key used to sign requests to
// the monopay widget (a different flow from /api/merchant/pubkey,
// which returns the bank's key for verifying webhooks).
type MonoPayKey struct {
	KeyID     string `json:"keyId"`
	KeyValue  string `json:"keyValue,omitempty"`
	KeyName   string `json:"keyName,omitempty"`
	ExpiresAt string `json:"expiresAt,omitempty"`
}

// MonoPayKeyImportRequest is the body of
// POST /api/merchant/monopay/pubkey-import. KeyValue is a
// base64-encoded PEM containing a public key in the x.509
// SubjectPublicKeyInfo format.
type MonoPayKeyImportRequest struct {
	KeyValue  string `json:"keyValue"`
	KeyName   string `json:"keyName,omitempty"`
	ExpiresAt string `json:"expiresAt,omitempty"`
}

// MonoPayKeyImportResponse wraps the import result.
type MonoPayKeyImportResponse struct {
	Result struct {
		KeyID string `json:"keyId"`
	} `json:"result"`
}

// MonoPayKeyDeleteRequest is the body of
// POST /api/merchant/monopay/pubkey-delete.
type MonoPayKeyDeleteRequest struct {
	KeyID string `json:"keyId"`
}

// MonoPayKeyListResponse is the response of
// GET /api/merchant/monopay/pubkey-list.
type MonoPayKeyListResponse struct {
	Result []MonoPayKey `json:"result"`
}

// MonoPayKeyImport imports the merchant's public key used to sign
// requests to the monopay widget.
func (c *Client) MonoPayKeyImport(ctx context.Context, in *MonoPayKeyImportRequest) (string, error) {
	if in == nil {
		return "", ErrNilRequest
	}
	body, err := json.Marshal(in)
	if err != nil {
		return "", fmt.Errorf("marshal: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		"/api/merchant/monopay/pubkey-import", bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}
	var out MonoPayKeyImportResponse
	if err := c.c.Do(req, &out, http.StatusOK); err != nil {
		return "", err
	}
	return out.Result.KeyID, nil
}

// MonoPayKeyDelete removes a previously imported key by its keyId.
func (c *Client) MonoPayKeyDelete(ctx context.Context, keyID string) error {
	if keyID == "" {
		return ErrEmptyID
	}
	body, err := json.Marshal(MonoPayKeyDeleteRequest{KeyID: keyID})
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		"/api/merchant/monopay/pubkey-delete", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	return c.c.Do(req, nil, http.StatusOK)
}

// MonoPayKeyList returns every imported merchant key.
func (c *Client) MonoPayKeyList(ctx context.Context) ([]MonoPayKey, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		"/api/merchant/monopay/pubkey-list", http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	var out MonoPayKeyListResponse
	if err := c.c.Do(req, &out, http.StatusOK); err != nil {
		return nil, err
	}
	return out.Result, nil
}
