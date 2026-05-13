// Package webhook — серверні хелпери для monobank webhook-ів:
// верифікація підпису, парсинг payload-у, готовий до монтування
// http.Handler з автоматичною ротацією ключа, плюс in-memory deduper,
// що поглинає 60-секундні і 600-секундні повторні доставки Mono.
//
// Mono підписує кожен body вебхука ECDSA на кривій secp256k1 і шле
// ідентифікатор ключа у X-Key-Id, а сам підпис — у X-Sign. Ключ
// зрідка ротується; банк публікує поточний у /bank/sync
// ([bank.Client.ServerKey]).
//
// Для «батарейки в комплекті» бери [NewHandler] — він тримає ключ у
// кеші, переотримує його коли X-Key-Id перестає збігатися, верифікує
// підпис, парсить тіло, відсіює дублі через Deduper і викликає твій
// callback. Якщо інтегруєш у власний HTTP-фреймворк з власним
// роутингом — користуйся нижчорівневими [Verify] і [Parse] напряму.
package webhook

import (
	"crypto/ecdsa"
	"crypto/sha256"
	"encoding/asn1"
	"encoding/base64"
	"errors"
	"fmt"
	"math/big"
)

// Помилки верифікації підпису.
var (
	// ErrBadSignature — підпис не збігається з body для даного ключа.
	// Перевіряй через errors.Is.
	ErrBadSignature = errors.New("webhook signature is invalid")
	// ErrBadSignatureEncoding — X-Sign не є валідним base64.
	ErrBadSignatureEncoding = errors.New("X-Sign is not valid base64")
	// ErrMissingPubKey — спроба верифікувати з nil-ключем.
	ErrMissingPubKey = errors.New("missing public key")
)

// Довжина «сирого» (r||s) підпису для secp256k1: дві координати по 32 байти.
const rawSigLen = 64

// Verify повертає nil, тільки якщо xSign — валідний ECDSA-підпис body,
// зроблений приватним ключем, що відповідає pub. Підтримує обидві форми,
// якими Mono історично шле підпис: сирий r||s (64 байти, base64) і
// ASN.1 DER — обидва декодуються прозоро.
//
// Зазвичай ти не викликаєш Verify напряму, бо [Handler] робить це за
// тебе. Винятки: інтеграція в чужий HTTP-фреймворк або написання
// тестових хелперів.
func Verify(pub *ecdsa.PublicKey, body []byte, xSign string) error {
	if pub == nil {
		return ErrMissingPubKey
	}
	sig, err := base64.StdEncoding.DecodeString(xSign)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrBadSignatureEncoding, err)
	}
	digest := sha256.Sum256(body)

	if len(sig) == rawSigLen {
		r := new(big.Int).SetBytes(sig[:rawSigLen/2])
		s := new(big.Int).SetBytes(sig[rawSigLen/2:])
		if ecdsa.Verify(pub, digest[:], r, s) {
			return nil
		}
		// raw r||s did not verify — fall through to ASN.1 DER, since some
		// encoders produce DER that also happens to be 64 bytes long.
	}

	var asn1Sig struct{ R, S *big.Int }
	if _, err := asn1.Unmarshal(sig, &asn1Sig); err != nil {
		return ErrBadSignature
	}
	if asn1Sig.R == nil || asn1Sig.S == nil {
		return ErrBadSignature
	}
	if !ecdsa.Verify(pub, digest[:], asn1Sig.R, asn1Sig.S) {
		return ErrBadSignature
	}
	return nil
}
