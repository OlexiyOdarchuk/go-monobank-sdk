package auth

import (
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/asn1"
	"encoding/base64"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"fmt"
	"log/slog"
	"math/big"
	"net/http"
	"strconv"
	"strings"
	"time"

	secp256k1 "github.com/decred/dcrd/dcrec/secp256k1/v4"
)

// Permission represents the permissions for corporate authorization.
// They are sent in the X-Permissions header on /personal/auth/request
// and define which data becomes visible after the client confirms.
//
// The typed string protects against accidental typos: the compiler
// will not let you pass "x" where a [Permission] is expected. When
// you need the value as a plain string (for example, for logs),
// convert explicitly: string(p).
type Permission string

// Standard permissions. The list is complete at the time of writing;
// if Mono adds new ones, use Permission("...") directly.
const (
	// PermSt — statements (transactions) and clientInfo of an
	// individual.
	PermSt Permission = "s"
	// PermPI — personal data (first name and last name).
	PermPI Permission = "p"
	// PermFOP — statements and clientInfo for sole-proprietor (FOP)
	// accounts.
	PermFOP Permission = "f"
)

// Errors that may surface from the corporate-authorization helpers.
var (
	// ErrDecodePrivateKey indicates that the PEM block of the private
	// key could not be decoded.
	ErrDecodePrivateKey = errors.New("failed to decode private key")
	// ErrEncodePublicKey indicates that the SHA-1 of the public key
	// (needed as X-Key-Id) could not be produced.
	ErrEncodePublicKey = errors.New("failed to encode public key with sha1")
	// ErrNoPrivateKey indicates that the input bytes contain no PEM
	// block of type "EC PRIVATE KEY".
	ErrNoPrivateKey = errors.New("failed to find private key block")
	// ErrInvalidEC indicates that the private-key value does not lie on
	// the secp256k1 curve.
	ErrInvalidEC = errors.New("invalid elliptic curve private key value")
	// ErrInvalidPrivateKey indicates an invalid private-key length
	// after stripping leading zeros (must be ≤ 32 bytes).
	ErrInvalidPrivateKey = errors.New("invalid private key length")
)

// CorpAuthMakerAPI is the factory of per-call corporate authorizers.
// The corporate client uses it to pick the right scope for each
// request: either a request-id (for requests about data of an
// already-approved client) or permissions (for the initial
// /personal/auth/request).
type CorpAuthMakerAPI interface {
	// New returns an Authorizer for endpoints that pass the request-id
	// in X-Request-Id.
	New(requestID string) Authorizer

	// NewPermissions returns an Authorizer for /personal/auth/request,
	// passing permissions in X-Permissions. An empty list means
	// "all permissions".
	NewPermissions(permissions ...Permission) Authorizer
}

// CorpAuthMaker holds the private ECDSA key (secp256k1) and the
// X-Key-Id derived from it (SHA-1 of the uncompressed public point).
// It produces per-request Authorizers that sign each request the way
// Mono expects.
type CorpAuthMaker struct {
	privateKey *ecdsa.PrivateKey
	// KeyID is the service's X-Key-Id: hex(sha1(uncompressed-pubkey)).
	KeyID string
}

type ecPrivateKey struct {
	Version       int
	PrivateKey    []byte
	NamedCurveOID asn1.ObjectIdentifier `asn1:"optional,explicit,tag:0"`
	PublicKey     asn1.BitString        `asn1:"optional,explicit,tag:1"`
}

const (
	ecPrivateKeyBlockType = "EC PRIVATE KEY"
	ecPrivateKeyVersion   = 1
)

// NewCorpAuthMaker builds a [CorpAuthMaker] from a PEM-encoded
// private key in the SEC1 ("EC PRIVATE KEY") format on the secp256k1
// curve. If the input contains several PEM blocks (for example,
// a parameters header plus the key itself), the decoder skips the
// irrelevant ones and finds the right block.
func NewCorpAuthMaker(secKey []byte) (*CorpAuthMaker, error) {
	privateKey, err := decodePrivateKey(secKey)
	if err != nil {
		return nil, ErrDecodePrivateKey
	}

	// X-Key-Id = hex(sha1(uncompressed-pubkey)). SHA-1 here is the
	// bank's chosen format (not a crypto weakness: it is only an
	// identifier, not a signature). The point is serialized via
	// secp256k1.SerializeUncompressed (the deprecated elliptic.Marshal
	// is replaced by the native method).
	pkBytes := serializeECDSAPubKeyUncompressed(&privateKey.PublicKey)
	hash := sha1.New() //nolint:gosec // SHA-1 is Mono's X-Key-Id format, not used for signing
	if _, err := hash.Write(pkBytes); err != nil {
		return nil, ErrEncodePublicKey
	}
	keyID := hex.EncodeToString(hash.Sum(nil))

	return &CorpAuthMaker{
		privateKey: privateKey,
		KeyID:      keyID,
	}, nil
}

// LogValue is the slog serializer that hides the private ECDSA key.
// Without it, `slog.Info("maker", "v", maker)` would dump the raw key
// coordinates.
func (c *CorpAuthMaker) LogValue() slog.Value {
	return slog.GroupValue(
		slog.String("KeyID", c.KeyID),
		slog.String("privateKey", "***"),
	)
}

// New returns a [Corp] authorizer bound to the given requestID. An
// empty requestID is valid for endpoints that require neither a
// request-id nor permissions (for example /personal/corp/settings,
// /personal/auth/registration).
func (c *CorpAuthMaker) New(requestID string) Authorizer {
	return Corp{maker: c, requestID: requestID}
}

// NewPermissions returns a [Corp] authorizer bound to a set of
// permissions (sent in X-Permissions). Used only for
// /personal/auth/request — for other endpoints use [CorpAuthMaker.New].
func (c *CorpAuthMaker) NewPermissions(permissions ...Permission) Authorizer {
	parts := make([]string, len(permissions))
	for i, p := range permissions {
		parts[i] = string(p)
	}
	return Corp{maker: c, permissions: strings.Join(parts, "")}
}

// Corp is the corporate Authorizer for a single request. Do not
// construct it directly; obtain one via [CorpAuthMaker.New] or
// [CorpAuthMaker.NewPermissions].
type Corp struct {
	maker       *CorpAuthMaker
	requestID   string
	permissions string
}

// SetAuth signs the outgoing request and sets the X-Key-Id, X-Time,
// X-Sign headers plus either X-Request-Id or X-Permissions depending
// on how this Corp was constructed. A nil request is a no-op; a
// request without a URL returns an error (the signature requires the
// path).
func (a Corp) SetAuth(r *http.Request) error {
	if r == nil {
		return nil
	}

	var actor string
	switch {
	case a.requestID != "":
		actor = a.requestID
		r.Header.Set("X-Request-Id", actor)
	case a.permissions != "":
		actor = a.permissions
		r.Header.Set("X-Permissions", actor)
	}

	timestamp := strconv.FormatInt(time.Now().Unix(), 10)

	if r.URL == nil {
		return errors.New("missing URL in request")
	}
	sign, err := a.sign(timestamp, actor, r.URL.Path)
	if err != nil {
		return fmt.Errorf("calculate Sign: %w", err)
	}

	r.Header.Set("X-Key-Id", a.maker.KeyID)
	r.Header.Set("X-Time", timestamp)
	r.Header.Set("X-Sign", sign)

	return nil
}

// sign computes Sign = base64(ECDSA(SHA-256(X-Time | actor | URL.Path))),
// where actor is the X-Request-Id or X-Permissions value (empty for
// endpoints that have neither). Concatenation uses no separators.
func (a Corp) sign(timestamp, actor, urlPath string) (string, error) {
	return a.signString(timestamp + actor + urlPath)
}

func (a Corp) signString(str string) (string, error) {
	hash := sha256.Sum256([]byte(str))

	r, s, err := ecdsa.Sign(rand.Reader, a.maker.privateKey, hash[:])
	if err != nil {
		return "", err
	}

	asn1Data := []*big.Int{r, s}
	bb, err := asn1.Marshal(asn1Data)
	if err != nil {
		return "", err
	}

	return base64.StdEncoding.EncodeToString(bb), nil
}

// serializeECDSAPubKeyUncompressed returns the uncompressed encoding
// (SEC1 §2.3.3) of a secp256k1 point: one prefix byte 0x04 + X
// (32 bytes, left-padded) + Y (32 bytes, left-padded). It replaces
// the deprecated elliptic.Marshal.
func serializeECDSAPubKeyUncompressed(pub *ecdsa.PublicKey) []byte {
	const coordSize = 32
	out := make([]byte, 1+2*coordSize)
	out[0] = 0x04
	xb := pub.X.Bytes()
	yb := pub.Y.Bytes()
	copy(out[1+coordSize-len(xb):1+coordSize], xb)
	copy(out[1+2*coordSize-len(yb):], yb)
	return out
}

// decodePrivateKey extracts an ECDSA private key from PEM-encoded SEC1
// ("EC PRIVATE KEY") data on secp256k1.
func decodePrivateKey(b []byte) (*ecdsa.PrivateKey, error) {
	for {
		var block *pem.Block
		block, b = pem.Decode(b)
		if block == nil {
			return nil, ErrNoPrivateKey
		}
		if block.Type != ecPrivateKeyBlockType {
			continue
		}
		return parseECPrivateKey(block.Bytes)
	}
}

// parseECPrivateKey reads a SEC1 ASN.1 EC private-key blob and constructs
// an ecdsa.PrivateKey on secp256k1.
func parseECPrivateKey(b []byte) (*ecdsa.PrivateKey, error) {
	var privKey ecPrivateKey
	if _, err := asn1.Unmarshal(b, &privKey); err != nil {
		return nil, fmt.Errorf("failed to parse EC private key: %w", err)
	}
	if privKey.Version != ecPrivateKeyVersion {
		return nil, fmt.Errorf("unknown EC private key version %d", privKey.Version)
	}

	curve := secp256k1.S256()
	// SEC1 allows leading zeros; secp256k1 expects exactly 32 bytes.
	raw := privKey.PrivateKey
	expected := (curve.Params().N.BitLen() + 7) / 8
	for len(raw) > expected && raw[0] == 0 {
		raw = raw[1:]
	}
	if len(raw) > expected {
		return nil, ErrInvalidPrivateKey
	}

	if new(big.Int).SetBytes(raw).Cmp(curve.Params().N) >= 0 {
		return nil, ErrInvalidEC
	}

	padded := make([]byte, expected)
	copy(padded[expected-len(raw):], raw)

	priv := secp256k1.PrivKeyFromBytes(padded).ToECDSA()
	return priv, nil
}
