package bank

import (
	"crypto/ecdsa"
	"encoding/base64"
	"errors"
	"fmt"
	"time"

	secp256k1 "github.com/decred/dcrd/dcrec/secp256k1/v4"
)

// Uncompressed encoding of a secp256k1 point (SEC 1, §2.3.3): one
// prefix byte (0x04), followed by the X and Y coordinates (32 bytes
// each).
const (
	uncompressedPointPrefix  = 0x04
	secp256k1CoordinateBytes = 32
	uncompressedPointLength  = 1 + 2*secp256k1CoordinateBytes
)

// ErrInvalidPubKey is returned from [Client.ServerKey] when
// /bank/sync produced a serverPubKey that is not a valid uncompressed
// secp256k1 point (must be 1+32+32 = 65 bytes with a 0x04 prefix).
var ErrInvalidPubKey = errors.New("invalid serverPubKey: not an uncompressed secp256k1 point")

// ServerKey is the bank's current public ECDSA key (secp256k1) along
// with its identifier (X-Key-Id) and the server time at the moment of
// the call.
//
// The X-Key-Id header on every incoming webhook equals [ServerKey.ID]
// for the key that signed the body. When the two stop matching, Mono
// has rotated the key — call [Client.ServerKey] again and refresh
// the cache.
//
// If you use [webhook.Handler], it does this automatically: it stores
// [ServerKey], reads X-Key-Id from each request, and requests a new
// key via [Client.ServerKey] when needed.
type ServerKey struct {
	ID         string
	PubKey     *ecdsa.PublicKey
	ServerTime time.Time
}

// bankSyncResponse is the JSON shape of the /bank/sync response. The
// type is package-private because the public surface is [ServerKey],
// which is built from it via asServerKey.
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
	// secp256k1.ParsePubKey verifies that (X, Y) actually lies on the
	// curve (guards against a MITM attack that swapped ServerKey for
	// an off-curve point — otherwise every verify would fail with 401
	// and the handler would DDoS /bank/sync with auto-refreshes in
	// response).
	pk, err := secp256k1.ParsePubKey(pubBytes)
	if err != nil {
		return nil, fmt.Errorf("%w: %s", ErrInvalidPubKey, err)
	}
	return &ServerKey{
		ID: r.ServerKeyID,
		PubKey: &ecdsa.PublicKey{
			//nolint:staticcheck // SA1019: Mono signs on secp256k1; the deprecated S256() is the only way to get an elliptic.Curve for crypto/ecdsa interop.
			Curve: secp256k1.S256(),
			X:     pk.X(),
			Y:     pk.Y(),
		},
		ServerTime: time.UnixMilli(r.ServerTimeMsec),
	}, nil
}
