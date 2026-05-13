package acquiring

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

// MonoPayKey — публічний ключ торговця для підпису запитів до віджета
// monopay (це інший потік, ніж /api/merchant/pubkey, який віддає ключ
// банку для верифікації вебхуків).
type MonoPayKey struct {
	KeyID     string `json:"keyId"`
	KeyValue  string `json:"keyValue,omitempty"`
	KeyName   string `json:"keyName,omitempty"`
	ExpiresAt string `json:"expiresAt,omitempty"`
}

// MonoPayKeyImportRequest — тіло POST /api/merchant/monopay/pubkey-import.
// KeyValue — base64-кодований PEM з публічним ключем у форматі x.509
// SubjectPublicKeyInfo.
type MonoPayKeyImportRequest struct {
	KeyValue  string `json:"keyValue"`
	KeyName   string `json:"keyName,omitempty"`
	ExpiresAt string `json:"expiresAt,omitempty"`
}

// MonoPayKeyImportResponse — обгортка над результатом імпорту.
type MonoPayKeyImportResponse struct {
	Result struct {
		KeyID string `json:"keyId"`
	} `json:"result"`
}

// MonoPayKeyDeleteRequest — тіло POST /api/merchant/monopay/pubkey-delete.
type MonoPayKeyDeleteRequest struct {
	KeyID string `json:"keyId"`
}

// MonoPayKeyListResponse — відповідь GET /api/merchant/monopay/pubkey-list.
type MonoPayKeyListResponse struct {
	Result []MonoPayKey `json:"result"`
}

// MonoPayKeyImport імпортує публічний ключ торговця для підпису запитів
// до віджета monopay.
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

// MonoPayKeyDelete видаляє раніше імпортований ключ за його keyId.
func (c *Client) MonoPayKeyDelete(ctx context.Context, keyID string) error {
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

// MonoPayKeyList повертає всі імпортовані ключі торговця.
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
