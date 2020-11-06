package services

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"io/ioutil"
	"math/big"
	"os"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	corecli "k8s.io/client-go/kubernetes"
	corelister "k8s.io/client-go/listers/core/v1"
)

// Secret contains functions to deal with needed secrets in the namespace.
type Secret struct {
	sclister corelister.SecretLister
	corcli   corecli.Interface
}

// NewSecret returns a new Secret helper.
func NewSecret(corcli corecli.Interface, sclister corelister.SecretLister) *Secret {
	return &Secret{
		corcli:   corcli,
		sclister: sclister,
	}
}

// CreateCertificates creates a secret with a key and a crt to be used by
// our mutating webhook http server.
func (s *Secret) CreateCertificates(ctx context.Context) (*corev1.Secret, error) {
	crt, key, err := s.generateCertificates()
	if err != nil {
		return nil, err
	}

	sec := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "certs",
			Namespace: "tagger",
		},
		Data: map[string][]byte{
			"server.crt": crt,
			"server.key": key,
		},
	}

	return s.corcli.CoreV1().Secrets("tagger").Create(
		ctx, sec, metav1.CreateOptions{},
	)
}

// CopySecret copies the content of a secret within the local file system.
// Everything is stored under /assets that ideally will be mounted as an
// emptyDir. Returns if something has been written from the secret and into
// the disk.
func (s *Secret) CopySecret(sct *corev1.Secret) (bool, error) {
	written := false
	for fname, content := range sct.Data {
		fpath := fmt.Sprintf("/assets/%s", fname)
		if _, err := os.Stat(fpath); err != nil {
			if !os.IsNotExist(err) {
				return false, err
			}
			if err := ioutil.WriteFile(fpath, content, 0600); err != nil {
				return false, err
			}
			continue
		}

		// XXX do this by file and secret modified time?
		fcontent, err := ioutil.ReadFile(fpath)
		if err != nil {
			return false, err
		}

		ndata := sha256.New().Sum(content)
		odata := sha256.New().Sum(fcontent)
		if string(ndata) == string(odata) {
			continue
		}

		if err := ioutil.WriteFile(fpath, content, 0600); err != nil {
			return false, err
		}
		written = true
	}
	return written, nil
}

// generateCertificates creates a self signed certificate valid for one year
// and valid when accepting connections to `webhooks.tagger.svc`, this is the
// service name we use for our mutating webhook.
func (s *Secret) generateCertificates() ([]byte, []byte, error) {
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, nil, err
	}

	kusage := x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature
	tpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{Organization: []string{"TagOperator"}},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(time.Hour * 24 * 365),
		KeyUsage:              kusage,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		DNSNames:              []string{"webhooks.tagger.svc"},
		BasicConstraintsValid: true,
	}

	der, err := x509.CreateCertificate(rand.Reader, tpl, tpl, &priv.PublicKey, priv)
	if err != nil {
		return nil, nil, err
	}

	crt := &bytes.Buffer{}
	if err := pem.Encode(
		crt, &pem.Block{Type: "CERTIFICATE", Bytes: der},
	); err != nil {
		return nil, nil, err
	}

	key := &bytes.Buffer{}
	if err := pem.Encode(
		key,
		&pem.Block{
			Type:  "RSA PRIVATE KEY",
			Bytes: x509.MarshalPKCS1PrivateKey(priv),
		},
	); err != nil {
		return nil, nil, err
	}

	return crt.Bytes(), key.Bytes(), nil
}
