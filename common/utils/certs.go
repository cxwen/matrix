package utils

import (
	cryptorand "crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"math"
	"math/big"
	"net"
	"time"
)

const (
	rsaKeySize   = 2048
	duration365d = time.Hour * 24 * 365
	defaultExpiredTime = 100
)

const (
	ECPrivateKeyBlockType       = "EC PRIVATE KEY"
	RSAPrivateKeyBlockType      = "RSA PRIVATE KEY"
	PrivateKeyBlockType         = "PRIVATE KEY"
	CertificateBlockType        = "CERTIFICATE"
	CertificateRequestBlockType = "CERTIFICATE REQUEST"
)

type Config struct {
	CommonName   string
	Organization []string
	AltNames     AltNames
	Usages       []x509.ExtKeyUsage
}

type AltNames struct {
	DNSNames []string
	IPs      []net.IP
}

// NewSelfSignedCACert creates a CA certificate
func NewSelfSignedCACert(cfg Config, key *rsa.PrivateKey) (*x509.Certificate, error) {
	now := time.Now()
	tmpl := x509.Certificate{
		SerialNumber: new(big.Int).SetInt64(0),
		Subject: pkix.Name{
			CommonName:   cfg.CommonName,
			Organization: cfg.Organization,
		},
		NotBefore:             now.UTC(),
		NotAfter:              now.Add(duration365d * time.Duration(defaultExpiredTime)).UTC(),
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		BasicConstraintsValid: true,
		IsCA: true,
	}

	certDERBytes, err := x509.CreateCertificate(cryptorand.Reader, &tmpl, &tmpl, key.Public(), key)
	if err != nil {
		return nil, err
	}
	return x509.ParseCertificate(certDERBytes)
}

// NewSignedCert creates a signed certificate using the given CA certificate and key
func NewSignedCert(cfg Config, key *rsa.PrivateKey, caCert *x509.Certificate, caKey *rsa.PrivateKey) (*x509.Certificate, error) {
	serial, err := cryptorand.Int(cryptorand.Reader, new(big.Int).SetInt64(math.MaxInt64))
	if err != nil {
		return nil, err
	}
	if len(cfg.CommonName) == 0 {
		return nil, errors.New("must specify a CommonName")
	}
	if len(cfg.Usages) == 0 {
		return nil, errors.New("must specify at least one ExtKeyUsage")
	}

	certTmpl := x509.Certificate{
		Subject: pkix.Name{
			CommonName:   cfg.CommonName,
			Organization: cfg.Organization,
		},
		DNSNames:     cfg.AltNames.DNSNames,
		IPAddresses:  cfg.AltNames.IPs,
		SerialNumber: serial,
		NotBefore:    caCert.NotBefore,
		NotAfter:     caCert.NotAfter,
		KeyUsage:     x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  cfg.Usages,
	}
	certDERBytes, err := x509.CreateCertificate(cryptorand.Reader, &certTmpl, caCert, key.Public(), caKey)
	if err != nil {
		return nil, err
	}
	return x509.ParseCertificate(certDERBytes)
}

func GenerateCaCert(cfg Config) (*x509.Certificate,*rsa.PrivateKey,error) {
	key, err := NewPrivateKey()
	if err != nil {
		return nil, nil, err
	}

	caCert, err := NewSelfSignedCACert(cfg, key)
	if err != nil {
		return nil, nil, err
	}

	return caCert, key, nil
}

func GenerateCert(cfg Config, caCert *x509.Certificate, caKey *rsa.PrivateKey) (*x509.Certificate,*rsa.PrivateKey,error) {
	key, err := NewPrivateKey()
	if err != nil {
		return nil, nil, err
	}

	cert, err := NewSignedCert(cfg, key, caCert, caKey)
	if err != nil {
		return nil, nil, err
	}

	return cert, key, nil
}

func NewPrivateKey() (*rsa.PrivateKey, error) {
	return rsa.GenerateKey(cryptorand.Reader, rsaKeySize)
}

func EncodeCertPEM(cert *x509.Certificate) []byte {
	block := pem.Block{
		Type:  CertificateBlockType,
		Bytes: cert.Raw,
	}

	return pem.EncodeToMemory(&block)
}

func EncodePrivateKeyPEM(key *rsa.PrivateKey) []byte {
	block := pem.Block{
		Type:  RSAPrivateKeyBlockType,
		Bytes: x509.MarshalPKCS1PrivateKey(key),
	}

	return pem.EncodeToMemory(&block)
}