// cms/cms.go
package messagesigning

import (
	"crypto"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"github.com/github/smimesign/ietf-cms"
	"grpc-benchmark-study/internal/resources"
)

var (
	signingCert *x509.Certificate
	signingKey  *rsa.PrivateKey
	trustedPool *x509.CertPool
)

// LoadSigner loads a PEM-encoded certificate and private key from the given paths.
// It returns the first certificate (as *x509.Certificate) and the corresponding private key,
// which must implement crypto.Signer.
func LoadSigner(certPath, keyPath, caPath string) error {
	// Read certificate PEM file.
	//certPEM, err := os.ReadFile(certPath)
	certPEM, err := resources.CMS.ReadFile(certPath)
	if err != nil {
		return err
	}
	// Read key PEM file.
	keyPEM, err := resources.CMS.ReadFile(keyPath)
	if err != nil {
		return err
	}
	// Load the key pair using tls.X509KeyPair.
	tlsCert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		return err
	}
	// Parse the first certificate.
	cert, err := x509.ParseCertificate(tlsCert.Certificate[0])
	if err != nil {
		return err
	}
	// Ensure the private key implements crypto.Signer.
	_, ok := tlsCert.PrivateKey.(crypto.Signer)
	if !ok {
		return errors.New("private key does not implement crypto.Signer")
	}
	signingCert = cert
	signingKey = tlsCert.PrivateKey.(*rsa.PrivateKey)

	caPEM, err := resources.CMS.ReadFile(caPath)
	if err != nil {
		return err
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(caPEM) {
		return errors.New("failed to append CA certificate")
	}

	trustedPool = pool

	return nil
}

// Sign creates a CMS (PKCS#7) signed envelope for the given data using the provided certificate and key.
// The returned []byte contains the CMS envelope with the embedded content.
func Sign(data []byte) ([]byte, error) {
	der, err := cms.Sign(data, []*x509.Certificate{signingCert}, signingKey)
	if err != nil {
		return nil, errors.New("Unable to sign data: " + err.Error())
	}
	return der, nil
}

// Verify parses and verifies the CMS-signed data using the provided CA certificate pool.
// If the signature is valid, it returns the original unwrapped content.
// Verify parses and verifies the CMS-signed data using the provided CA certificate pool.
// If the signature is valid, it returns the original unwrapped content.
func Verify(signedData []byte) ([]byte, error) {
	sd, err := cms.ParseSignedData(signedData)
	if err != nil {
		return nil, err
	}

	// Use the trustedPool loaded via LoadTrustedCA.
	opts := x509.VerifyOptions{
		Roots: trustedPool,
	}

	if _, err := sd.Verify(opts); err != nil {
		return nil, errors.New("Unable to verify data: " + err.Error())
	}
	return sd.GetData()
}
