package cert

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"time"
)

// New generates a new self signed certificate. Returned key is valid for one year.
func New(org string) ([]byte, []byte, error) {
	pkey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, nil, fmt.Errorf("error generating key: %w", err)
	}

	kusage := x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature
	template := x509.Certificate{
		SerialNumber:          big.NewInt(1),
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(time.Hour * 24 * 356),
		KeyUsage:              kusage,
		BasicConstraintsValid: true,
		ExtKeyUsage: []x509.ExtKeyUsage{
			x509.ExtKeyUsageServerAuth,
		},
		Subject: pkix.Name{
			Organization: []string{org},
		},
	}

	cert, err := x509.CreateCertificate(
		rand.Reader, &template, &template, &pkey.PublicKey, pkey,
	)
	if err != nil {
		return nil, nil, fmt.Errorf("error creating certificate: %w", err)
	}

	crtbuf := bytes.NewBuffer(nil)
	pem.Encode(
		crtbuf,
		&pem.Block{
			Type:  "CERTIFICATE",
			Bytes: cert,
		},
	)

	pbytes, err := x509.MarshalPKCS8PrivateKey(pkey)
	if err != nil {
		return nil, nil, fmt.Errorf("error marshaling private key: %w", err)
	}

	keybuf := bytes.NewBuffer(nil)
	pem.Encode(
		keybuf,
		&pem.Block{
			Type:  "RSA PRIVATE KEY",
			Bytes: pbytes,
		},
	)

	return keybuf.Bytes(), crtbuf.Bytes(), nil
}
