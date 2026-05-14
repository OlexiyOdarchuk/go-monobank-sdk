package auth

import (
	"encoding/asn1"
	"encoding/pem"
	"fmt"
)

// ASN.1 OID-и для маршалінгу публічного ключа secp256k1 в SPKI-формат.
//
//	id-ecPublicKey = 1.2.840.10045.2.1
//	secp256k1      = 1.3.132.0.10
var (
	oidPublicKeyECDSA = asn1.ObjectIdentifier{1, 2, 840, 10045, 2, 1}
	oidSecp256k1      = asn1.ObjectIdentifier{1, 3, 132, 0, 10}
)

// pkixPublicKey — мінімальна ASN.1-структура для X.509
// SubjectPublicKeyInfo, ручкою сконструйована для secp256k1 (бо
// стандартна x509.MarshalPKIXPublicKey не підтримує цю криву).
type pkixPublicKey struct {
	Algorithm pkixAlgorithm
	PublicKey asn1.BitString
}

type pkixAlgorithm struct {
	Algorithm  asn1.ObjectIdentifier
	Parameters asn1.RawValue
}

// secp256k1 uncompressed point: 0x04 || X (32 bytes) || Y (32 bytes).
const pointCoordinateBytes = 32

// PublicKeyPEM повертає публічний ключ цього CorpAuthMaker у форматі,
// якого очікують [corporate.Client.Register] і
// [corporate.Client.RegistrationStatus]: PEM-блок типу "PUBLIC KEY",
// який містить SubjectPublicKeyInfo з кривою secp256k1 і uncompressed
// точкою (0x04 || X || Y).
//
// Зручно для першої подачі заявки:
//
//	maker, _ := auth.NewCorpAuthMaker(privPEM)
//	pubPEM, _ := maker.PublicKeyPEM()
//	cli.Register(ctx, &corporate.RegistrationRequest{
//	    Pubkey: pubPEM,
//	    Name:   "ТОВ \"Acme\"",
//	    ...
//	})
//
// Реалізовано вручну, бо x509.MarshalPKIXPublicKey зі стандартної
// бібліотеки не підтримує secp256k1 (тільки P-224/P-256/P-384/P-521).
func (c *CorpAuthMaker) PublicKeyPEM() ([]byte, error) {
	if c.privateKey.X.BitLen() > 8*pointCoordinateBytes ||
		c.privateKey.Y.BitLen() > 8*pointCoordinateBytes {
		return nil, fmt.Errorf("auth: public-key coordinates exceed 32 bytes")
	}
	point := serializeECDSAPubKeyUncompressed(&c.privateKey.PublicKey)

	paramBytes, err := asn1.Marshal(oidSecp256k1)
	if err != nil {
		return nil, fmt.Errorf("auth: marshal curve OID: %w", err)
	}

	info := pkixPublicKey{
		Algorithm: pkixAlgorithm{
			Algorithm:  oidPublicKeyECDSA,
			Parameters: asn1.RawValue{FullBytes: paramBytes},
		},
		PublicKey: asn1.BitString{
			Bytes:     point,
			BitLength: 8 * len(point),
		},
	}

	der, err := asn1.Marshal(info)
	if err != nil {
		return nil, fmt.Errorf("auth: marshal SubjectPublicKeyInfo: %w", err)
	}

	return pem.EncodeToMemory(&pem.Block{
		Type:  "PUBLIC KEY",
		Bytes: der,
	}), nil
}
