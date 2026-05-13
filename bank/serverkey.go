package bank

import (
	"crypto/ecdsa"
	"encoding/base64"
	"errors"
	"fmt"
	"math/big"
	"time"

	secp256k1 "github.com/decred/dcrd/dcrec/secp256k1/v4"
)

// Uncompressed-кодування точки secp256k1 (SEC 1, §2.3.3): 1 префіксний
// байт (0x04), за ним координати X та Y (по 32 байти кожна).
const (
	uncompressedPointPrefix  = 0x04
	secp256k1CoordinateBytes = 32
	uncompressedPointLength  = 1 + 2*secp256k1CoordinateBytes
)

// ErrInvalidPubKey повертається з [Client.ServerKey], коли /bank/sync
// віддав serverPubKey, що не є валідною uncompressed-точкою secp256k1
// (має бути 1+32+32 = 65 байт із префіксом 0x04).
var ErrInvalidPubKey = errors.New("invalid serverPubKey: not an uncompressed secp256k1 point")

// ServerKey — поточний публічний ECDSA-ключ банку (secp256k1) разом з
// його ідентифікатором (X-Key-Id) і серверним часом на момент виклику.
//
// Заголовок X-Key-Id у кожному вхідному webhook-у дорівнює [ServerKey.ID]
// для ключа, яким підписано body. Коли вони перестають збігатися — Mono
// провернула ключ, треба перевиклик [Client.ServerKey] і оновити кеш.
//
// Якщо ти використовуєш [webhook.Handler], він робить це автоматично:
// зберігає [ServerKey], читає X-Key-Id з кожного запиту і запитує новий
// ключ через [Client.ServerKey] коли потрібно.
type ServerKey struct {
	ID         string
	PubKey     *ecdsa.PublicKey
	ServerTime time.Time
}

// bankSyncResponse — JSON-форма відповіді /bank/sync. Тип package-private,
// бо публічна поверхня — це [ServerKey], який збирається з нього через
// asServerKey.
type bankSyncResponse struct {
	ServerKeyID    string `json:"serverKeyId"`
	ServerPubKey   string `json:"serverPubKey"`
	ServerTimeMsec int64  `json:"serverTimeMsec"`
}

func (r bankSyncResponse) asServerKey() (*ServerKey, error) {
	pubBytes, err := base64.StdEncoding.DecodeString(r.ServerPubKey)
	if err != nil {
		return nil, fmt.Errorf("decode serverPubKey: %w", err)
	}
	if len(pubBytes) != uncompressedPointLength || pubBytes[0] != uncompressedPointPrefix {
		return nil, ErrInvalidPubKey
	}
	return &ServerKey{
		ID: r.ServerKeyID,
		PubKey: &ecdsa.PublicKey{
			Curve: secp256k1.S256(),
			X:     new(big.Int).SetBytes(pubBytes[1 : 1+secp256k1CoordinateBytes]),
			Y:     new(big.Int).SetBytes(pubBytes[1+secp256k1CoordinateBytes:]),
		},
		ServerTime: time.UnixMilli(r.ServerTimeMsec),
	}, nil
}
