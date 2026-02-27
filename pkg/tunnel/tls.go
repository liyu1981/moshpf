package tunnel

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"fmt"
	"math/big"
	"time"
)

// GenerateEphemeralCert generates a self-signed certificate and its SHA256 fingerprint.
func GenerateEphemeralCert() (*tls.Certificate, string, error) {
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, "", err
	}

	notBefore := time.Now()
	notAfter := notBefore.Add(24 * time.Hour)

	serialNumberLimit := new(big.Int).Lsh(big.NewInt(1), 128)
	serialNumber, err := rand.Int(rand.Reader, serialNumberLimit)
	if err != nil {
		return nil, "", err
	}

	template := x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			Organization: []string{"moshpf"},
		},
		NotBefore:             notBefore,
		NotAfter:              notAfter,
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
	}

	derBytes, err := x509.CreateCertificate(rand.Reader, &template, &template, &priv.PublicKey, priv)
	if err != nil {
		return nil, "", err
	}

	hash := sha256.Sum256(derBytes)
	fingerprint := hex.EncodeToString(hash[:])

	cert := tls.Certificate{
		Certificate: [][]byte{derBytes},
		PrivateKey:  priv,
	}

	return &cert, fingerprint, nil
}

// GetTLSConfigClient returns a tls.Config for the client that pins the server's certificate.
func GetTLSConfigClient(expectedFingerprint string) *tls.Config {
	return &tls.Config{
		InsecureSkipVerify: true,
		VerifyPeerCertificate: func(rawCerts [][]byte, _ [][]*x509.Certificate) error {
			if len(rawCerts) == 0 {
				return fmt.Errorf("no certificate provided by server")
			}
			hash := sha256.Sum256(rawCerts[0])
			fingerprint := hex.EncodeToString(hash[:])
			if fingerprint != expectedFingerprint {
				return fmt.Errorf("certificate fingerprint mismatch: expected %s, got %s", expectedFingerprint, fingerprint)
			}
			return nil
		},
		NextProtos: []string{"moshpf-0"},
	}
}

// GetTLSConfigServer returns a tls.Config for the server with the given certificate.
func GetTLSConfigServer(cert *tls.Certificate) *tls.Config {
	return &tls.Config{
		Certificates: []tls.Certificate{*cert},
		NextProtos:   []string{"moshpf-0"},
	}
}
