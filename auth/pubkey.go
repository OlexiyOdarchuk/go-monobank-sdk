package auth

import (
	"encoding/asn1"
	"encoding/pem"
	"fmt"
)

// ASN.1 OIDs used to marshal a secp256k1 public key into the SPKI
// format.
//
//	id-ecPublicKey = 1.2.840.10045.2.1
//	secp256k1      = 1.3.132.0.10
var (
	oidPublicKeyECDSA = asn1.ObjectIdentifier{1, 2, 840, 10045, 2, 1}
	oidSecp256k1      = asn1.ObjectIdentifier{1, 3, 132, 0, 10}
)

// pkixPublicKey is the minimal ASN.1 structure for the X.509
// SubjectPublicKeyInfo, hand-built for secp256k1 (because the
// standard x509.MarshalPKIXPublicKey does not support this curve).
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

// PublicKeyPEM returns this CorpAuthMaker's public key in the format
// [corporate.Client.Register] and [corporate.Client.RegistrationStatus]
// expect: a PEM block of type "PUBLIC KEY" that contains a
// SubjectPublicKeyInfo with the secp256k1 curve and an uncompressed
// point (0x04 || X || Y).
//
// Handy for the initial registration:
//
//	maker, _ := auth.NewCorpAuthMaker(privPEM)
//	pubPEM, _ := maker.PublicKeyPEM()
//	cli.Register(ctx, &corporate.RegistrationRequest{
//	    Pubkey: pubPEM,
//	    Name:   "ТОВ \"Acme\"",
//	    ...
//	})
//
// Implemented by hand because the standard library's
// x509.MarshalPKIXPublicKey does not support secp256k1 (only
// P-224/P-256/P-384/P-521).
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
