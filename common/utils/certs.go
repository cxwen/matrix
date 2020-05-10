package utils

import (
	cryptorand "crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"fmt"
	"k8s.io/apimachinery/pkg/runtime"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
	clientcmdlatest "k8s.io/client-go/tools/clientcmd/api/latest"
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

	AdminKubeConfigFileName             = "admin.conf"
	ControllerManagerKubeConfigFileName = "controller-manager.conf"
	SchedulerKubeConfigFileName         = "scheduler.conf"
	DefaultServerUrl                    = "https://127.0.0.1:6443"
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

func GenerateServiceAccountKey() (*rsa.PrivateKey,error) {
	saSigningKey, err := NewServiceAccountSigningKey()
	if err != nil {
		return nil, err
	}

	return saSigningKey, nil
}

func NewServiceAccountSigningKey() (*rsa.PrivateKey, error) {
	saSigningKey, err := NewPrivateKey()
	if err != nil {
		return nil, fmt.Errorf("failure while creating service account token signing key: %v", err)
	}

	return saSigningKey, nil
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

func EncodePublicKeyPEM(key *rsa.PublicKey) ([]byte, error) {
	der, err := x509.MarshalPKIXPublicKey(key)
	if err != nil {
		return []byte{}, err
	}
	block := pem.Block{
		Type:  "PUBLIC KEY",
		Bytes: der,
	}
	return pem.EncodeToMemory(&block), nil
}

func CreateKubeconfigWithCerts(serverURL, clusterName, userName string, caCert []byte, clientKey []byte, clientCert []byte) *clientcmdapi.Config {
	config := CreateBasic(serverURL, clusterName, userName, caCert)
	config.AuthInfos[userName] = &clientcmdapi.AuthInfo{
		ClientKeyData:         clientKey,
		ClientCertificateData: clientCert,
	}
	return config
}

func CreateBasic(serverURL, clusterName, userName string, caCert []byte) *clientcmdapi.Config {
	// Use the cluster and the username as the context name
	contextName := fmt.Sprintf("%s@%s", userName, clusterName)

	return &clientcmdapi.Config{
		Clusters: map[string]*clientcmdapi.Cluster{
			clusterName: {
				Server: serverURL,
				CertificateAuthorityData: caCert,
			},
		},
		Contexts: map[string]*clientcmdapi.Context{
			contextName: {
				Cluster:  clusterName,
				AuthInfo: userName,
			},
		},
		AuthInfos:      map[string]*clientcmdapi.AuthInfo{},
		CurrentContext: contextName,
	}
}

func CreateAdminKubeconfig(caCert *x509.Certificate, caKey *rsa.PrivateKey) ([]byte, error) {
	cfg := Config{
		CommonName:   "kubernetes-admin",
		Organization: []string{"system:masters"},
		Usages:       []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
	}

	cert, key, err := GenerateCert(cfg, caCert, caKey)
	if err != nil {
		return nil, err
	}

	kubeconfig := CreateKubeconfigWithCerts(DefaultServerUrl, "kubernetes", "kubernetes-admin",
		EncodeCertPEM(caCert), EncodeCertPEM(cert), EncodePrivateKeyPEM(key))

	return runtime.Encode(clientcmdlatest.Codec, kubeconfig)
}

func CreateControllerManagerKUbeconfig(caCert *x509.Certificate, caKey *rsa.PrivateKey) ([]byte, error) {
	cfg := Config{
		CommonName: "system:kube-controller-manager",
		Usages:     []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
	}

	cert, key, err := GenerateCert(cfg, caCert, caKey)
	if err != nil {
		return nil, err
	}

	kubeconfig := CreateKubeconfigWithCerts(DefaultServerUrl, "kubernetes", "system:kube-controller-manager",
		EncodeCertPEM(caCert), EncodeCertPEM(cert), EncodePrivateKeyPEM(key))

	return runtime.Encode(clientcmdlatest.Codec, kubeconfig)
}

func CreateSchedulerKubeconfig(caCert *x509.Certificate, caKey *rsa.PrivateKey) ([]byte, error) {
	cfg := Config{
		CommonName: "system:kube-scheduler",
		Usages:     []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
	}

	cert, key, err := GenerateCert(cfg, caCert, caKey)
	if err != nil {
		return nil, err
	}

	kubeconfig := CreateKubeconfigWithCerts(DefaultServerUrl, "kubernetes", "system:kube-scheduler",
		EncodeCertPEM(caCert), EncodeCertPEM(cert), EncodePrivateKeyPEM(key))

	return runtime.Encode(clientcmdlatest.Codec, kubeconfig)
}