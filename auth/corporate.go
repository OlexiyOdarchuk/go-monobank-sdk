package auth

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/asn1"
	"encoding/base64"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"strconv"
	"strings"
	"time"

	secp256k1 "github.com/decred/dcrd/dcrec/secp256k1/v4"
)

// Літери дозволів (permissions) для корпоративної авторизації. Передаються
// у заголовку X-Permissions при /personal/auth/request і визначають, які
// дані буде видно після підтвердження клієнтом.
const (
	// PermSt — виписки (транзакції) і clientInfo фізичної особи.
	PermSt = "s"
	// PermPI — персональні дані (ім'я та прізвище).
	PermPI = "p"
	// PermFOP — виписки і clientInfo для рахунків ФОП.
	PermFOP = "f"
)

// Помилки, які можуть повернутись із хелперів корпоративної авторизації.
var (
	// ErrDecodePrivateKey — не вдалося декодувати PEM-блок приватного ключа.
	ErrDecodePrivateKey = errors.New("failed to decode private key")
	// ErrEncodePublicKey — не вдалося згенерувати SHA-1 публічного ключа
	// (потрібен як X-Key-Id).
	ErrEncodePublicKey = errors.New("failed to encode public key with sha1")
	// ErrNoPrivateKey — у вхідних байтах немає PEM-блоку типу "EC PRIVATE KEY".
	ErrNoPrivateKey = errors.New("failed to find private key block")
	// ErrInvalidEC — значення приватного ключа не лежить на кривій secp256k1.
	ErrInvalidEC = errors.New("invalid elliptic curve private key value")
	// ErrInvalidPrivateKey — некоректна довжина приватного ключа після
	// стрипу провідних нулів (має бути ≤ 32 байти).
	ErrInvalidPrivateKey = errors.New("invalid private key length")
)

// CorpAuthMakerAPI — фабрика per-call корпоративних авторизаторів.
// Корпоративний клієнт використовує її, щоб для кожного запиту обрати
// потрібний scope: або request-id (для запитів про дані вже схваленого
// клієнта), або permissions (для початкового /personal/auth/request).
type CorpAuthMakerAPI interface {
	// New повертає Authorizer для endpoint-ів із request-id у X-Request-Id.
	New(requestID string) Authorizer

	// NewPermissions повертає Authorizer для /personal/auth/request,
	// передаючи permissions у X-Permissions. Порожній список означає
	// «усі дозволи».
	NewPermissions(permissions ...string) Authorizer
}

// CorpAuthMaker зберігає приватний ECDSA-ключ (secp256k1) і обчислений
// з нього X-Key-Id (SHA-1 від uncompressed public point). Породжує
// per-request Authorizer-и, що підписують кожен запит так, як очікує Mono.
type CorpAuthMaker struct {
	privateKey *ecdsa.PrivateKey
	// KeyID — X-Key-Id сервісу: hex(sha1(uncompressed-pubkey)).
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

// NewCorpAuthMaker будує [CorpAuthMaker] з PEM-кодованого приватного
// ключа у форматі SEC1 ("EC PRIVATE KEY") на кривій secp256k1. Якщо у
// вхідних байтах є кілька PEM-блоків (наприклад, заголовок параметрів +
// власне ключ), декодер пропустить нерелевантні і знайде потрібний.
func NewCorpAuthMaker(secKey []byte) (*CorpAuthMaker, error) {
	privateKey, err := decodePrivateKey(secKey)
	if err != nil {
		return nil, ErrDecodePrivateKey
	}

	publicKey := privateKey.PublicKey
	data := elliptic.Marshal(publicKey, publicKey.X, publicKey.Y)
	hash := sha1.New()
	if _, err := hash.Write(data); err != nil {
		return nil, ErrEncodePublicKey
	}
	keyID := hex.EncodeToString(hash.Sum(nil))

	return &CorpAuthMaker{
		privateKey: privateKey,
		KeyID:      keyID,
	}, nil
}

// New повертає [Corp]-авторизатор, прив'язаний до конкретного
// requestID. Порожній requestID валідний для endpoint-ів, які не
// потребують ані request-id, ані permissions (наприклад
// /personal/corp/settings, /personal/auth/registration).
func (c *CorpAuthMaker) New(requestID string) Authorizer {
	return Corp{maker: c, requestID: requestID}
}

// NewPermissions повертає [Corp]-авторизатор, прив'язаний до набору
// permissions (передаються у X-Permissions). Використовується тільки для
// /personal/auth/request — для решти endpoint-ів вживай [CorpAuthMaker.New].
func (c *CorpAuthMaker) NewPermissions(permissions ...string) Authorizer {
	return Corp{maker: c, permissions: strings.Join(permissions, "")}
}

// Corp — корпоративний Authorizer для одного запиту. Напряму не
// конструюй; отримуй через [CorpAuthMaker.New] або
// [CorpAuthMaker.NewPermissions].
type Corp struct {
	maker       *CorpAuthMaker
	requestID   string
	permissions string
}

// SetAuth підписує вихідний запит і виставляє заголовки X-Key-Id,
// X-Time, X-Sign плюс або X-Request-Id, або X-Permissions залежно від
// того, як було сконструйовано цей Corp. nil-request — no-op; запит без
// URL поверне помилку (підпис не побудувати без шляху).
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

// sign обчислює Sign = base64(ECDSA(SHA-256(X-Time | actor | URL.Path))),
// де actor — це X-Request-Id або X-Permissions (порожній для endpoint-ів,
// що не мають ані того, ані того). Конкатенація без роздільників.
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
