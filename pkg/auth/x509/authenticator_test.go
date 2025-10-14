// Copyright 2023 The emqx-go Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package x509

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/turtacn/emqx-go/pkg/auth"
	tlspkg "github.com/turtacn/emqx-go/pkg/tls"
)

// Test certificate generation helpers
func generateTestCA(t *testing.T) (*x509.Certificate, *rsa.PrivateKey, string) {
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			Organization:  []string{"Test CA"},
			Country:       []string{"US"},
			Province:      []string{""},
			Locality:      []string{"Test"},
			StreetAddress: []string{""},
			PostalCode:    []string{""},
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(365 * 24 * time.Hour),
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		IsCA:                  true,
	}

	certDER, err := x509.CreateCertificate(rand.Reader, &template, &template, &priv.PublicKey, priv)
	require.NoError(t, err)

	cert, err := x509.ParseCertificate(certDER)
	require.NoError(t, err)

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	return cert, priv, string(certPEM)
}

func generateTestClientCert(t *testing.T, caCert *x509.Certificate, caKey *rsa.PrivateKey, subject pkix.Name, sans []string) (*x509.Certificate, string) {
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	template := x509.Certificate{
		SerialNumber: big.NewInt(2),
		Subject:      subject,
		NotBefore:    time.Now(),
		NotAfter:     time.Now().Add(365 * 24 * time.Hour),
		KeyUsage:     x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
	}

	// Add SANs if provided
	for _, san := range sans {
		if ip := net.ParseIP(san); ip != nil {
			template.IPAddresses = append(template.IPAddresses, ip)
		} else if u, err := url.Parse("mailto:" + san); err == nil && u.Scheme == "mailto" {
			template.EmailAddresses = append(template.EmailAddresses, san)
		} else if u, err := url.Parse(san); err == nil && u.Scheme != "" {
			template.URIs = append(template.URIs, u)
		} else {
			template.DNSNames = append(template.DNSNames, san)
		}
	}

	certDER, err := x509.CreateCertificate(rand.Reader, &template, caCert, &priv.PublicKey, caKey)
	require.NoError(t, err)

	cert, err := x509.ParseCertificate(certDER)
	require.NoError(t, err)

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	return cert, string(certPEM)
}

func TestNewX509Authenticator(t *testing.T) {
	certMgr := tlspkg.NewCertificateManager()

	tests := []struct {
		name      string
		config    *X509Config
		expectErr bool
	}{
		{
			name:      "nil config uses default",
			config:    nil,
			expectErr: false,
		},
		{
			name:      "default config",
			config:    DefaultX509Config(),
			expectErr: false,
		},
		{
			name: "valid config with pattern",
			config: &X509Config{
				Enabled:         true,
				IdentitySource:  IdentityFromSubjectCN,
				IdentityPattern: "^test-.*",
			},
			expectErr: false,
		},
		{
			name: "invalid regex pattern",
			config: &X509Config{
				Enabled:         true,
				IdentitySource:  IdentityFromSubjectCN,
				IdentityPattern: "[invalid-regex",
			},
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			auth, err := NewX509Authenticator(tt.config, certMgr)
			if tt.expectErr {
				assert.Error(t, err)
				assert.Nil(t, auth)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, auth)
				assert.Equal(t, "x509", auth.Name())
			}
		})
	}
}

func TestX509Authenticator_Authenticate(t *testing.T) {
	certMgr := tlspkg.NewCertificateManager()
	config := DefaultX509Config()
	config.Enabled = true

	authInstance, err := NewX509Authenticator(config, certMgr)
	require.NoError(t, err)

	// Test disabled authenticator
	authInstance.SetEnabled(false)
	result := authInstance.Authenticate("test", "test")
	assert.Equal(t, auth.AuthIgnore, result)

	// Test enabled authenticator validates identity
	authInstance.SetEnabled(true)
	result = authInstance.Authenticate("test-user", "")
	assert.Equal(t, auth.AuthSuccess, result)

	// Test empty username fails
	result = authInstance.Authenticate("", "")
	assert.Equal(t, auth.AuthFailure, result)
}

func TestX509Authenticator_AuthenticateWithCertificate(t *testing.T) {
	certMgr := tlspkg.NewCertificateManager()

	// Generate test CA and client certificate
	caCert, caKey, _ := generateTestCA(t)

	// Test disabled authenticator
	config := &X509Config{
		Enabled:        false,
		IdentitySource: IdentityFromSubjectCN,
	}
	authInstance, err := NewX509Authenticator(config, certMgr)
	require.NoError(t, err)

	connState := &tls.ConnectionState{}
	identity, result := authInstance.AuthenticateWithCertificate(connState)
	assert.Equal(t, auth.AuthIgnore, result)
	assert.Empty(t, identity)

	// Test no peer certificates
	config.Enabled = true
	authInstance, err = NewX509Authenticator(config, certMgr)
	require.NoError(t, err)

	connState = &tls.ConnectionState{
		PeerCertificates: []*x509.Certificate{},
	}
	identity, result = authInstance.AuthenticateWithCertificate(connState)
	assert.Equal(t, auth.AuthIgnore, result)
	assert.Empty(t, identity)

	// Test extract identity from subject CN
	subject := pkix.Name{
		CommonName:   "test-client",
		Organization: []string{"Test Org"},
	}
	clientCert, _ := generateTestClientCert(t, caCert, caKey, subject, nil)
	connState = &tls.ConnectionState{
		PeerCertificates: []*x509.Certificate{clientCert},
	}
	identity, result = authInstance.AuthenticateWithCertificate(connState)
	assert.Equal(t, auth.AuthSuccess, result)
	assert.Equal(t, "test-client", identity)
}

func TestX509Authenticator_ValidateCertificateFromPEM(t *testing.T) {
	certMgr := tlspkg.NewCertificateManager()
	config := &X509Config{
		Enabled:        true,
		IdentitySource: IdentityFromSubjectCN,
	}

	authInstance, err := NewX509Authenticator(config, certMgr)
	require.NoError(t, err)

	// Generate test certificates
	caCert, caKey, _ := generateTestCA(t)
	subject := pkix.Name{CommonName: "test-client"}
	_, clientCertPEM := generateTestClientCert(t, caCert, caKey, subject, nil)

	// Test valid certificate
	identity, err := authInstance.ValidateCertificateFromPEM(clientCertPEM)
	assert.NoError(t, err)
	assert.Equal(t, "test-client", identity)

	// Test invalid PEM
	_, err = authInstance.ValidateCertificateFromPEM("invalid pem data")
	assert.Error(t, err)
}

func TestX509Authenticator_RevokedSerials(t *testing.T) {
	certMgr := tlspkg.NewCertificateManager()
	config := DefaultX509Config()
	authInstance, err := NewX509Authenticator(config, certMgr)
	require.NoError(t, err)

	// Test adding revoked serials
	authInstance.AddRevokedSerial("12345")
	authInstance.AddRevokedSerial("67890")

	serials := authInstance.GetRevokedSerials()
	assert.Contains(t, serials, "12345")
	assert.Contains(t, serials, "67890")
	assert.Len(t, serials, 2)

	// Test removing revoked serial
	authInstance.RemoveRevokedSerial("12345")
	serials = authInstance.GetRevokedSerials()
	assert.NotContains(t, serials, "12345")
	assert.Contains(t, serials, "67890")
	assert.Len(t, serials, 1)

	// Test removing non-existent serial
	authInstance.RemoveRevokedSerial("99999")
	serials = authInstance.GetRevokedSerials()
	assert.Len(t, serials, 1)
}

func TestX509Authenticator_UpdateConfig(t *testing.T) {
	certMgr := tlspkg.NewCertificateManager()
	config := DefaultX509Config()
	authInstance, err := NewX509Authenticator(config, certMgr)
	require.NoError(t, err)

	// Test nil config
	err = authInstance.UpdateConfig(nil)
	assert.Error(t, err)

	// Test valid config update
	newConfig := &X509Config{
		Enabled:         false,
		IdentitySource:  IdentityFromSAN,
		IdentityPattern: "^new-.*",
	}
	err = authInstance.UpdateConfig(newConfig)
	assert.NoError(t, err)
	assert.Equal(t, newConfig.Enabled, authInstance.Enabled())
	assert.Equal(t, newConfig, authInstance.GetConfig())

	// Test invalid pattern
	invalidConfig := &X509Config{
		Enabled:         true,
		IdentitySource:  IdentityFromSubjectCN,
		IdentityPattern: "[invalid-regex",
	}
	err = authInstance.UpdateConfig(invalidConfig)
	assert.Error(t, err)
}

func TestX509Authenticator_EnableDisable(t *testing.T) {
	certMgr := tlspkg.NewCertificateManager()
	config := DefaultX509Config()
	authInstance, err := NewX509Authenticator(config, certMgr)
	require.NoError(t, err)

	// Initially enabled based on config
	assert.False(t, authInstance.Enabled()) // Default config has Enabled: false

	// Enable the authenticator
	authInstance.SetEnabled(true)
	assert.True(t, authInstance.Enabled())

	// Disable the authenticator
	authInstance.SetEnabled(false)
	assert.False(t, authInstance.Enabled())
}

func TestDefaultX509Config(t *testing.T) {
	config := DefaultX509Config()

	assert.False(t, config.Enabled)
	assert.Equal(t, IdentityFromSubjectCN, config.IdentitySource)
	assert.Equal(t, 10, config.CAChainDepth)
	assert.True(t, config.VerifyCertChain)
	assert.Empty(t, config.AllowedCNs)
	assert.Empty(t, config.RevokedSerials)
}

func TestIdentitySourceConstants(t *testing.T) {
	assert.Equal(t, IdentitySource("subject_dn"), IdentityFromSubjectDN)
	assert.Equal(t, IdentitySource("subject_cn"), IdentityFromSubjectCN)
	assert.Equal(t, IdentitySource("san"), IdentityFromSAN)
	assert.Equal(t, IdentitySource("serial"), IdentityFromSerial)
	assert.Equal(t, IdentitySource("fingerprint"), IdentityFromFingerprint)
}
