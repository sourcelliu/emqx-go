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

package tls

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Helper function to generate test certificates
func generateTestCertificate(isCA bool) (string, string, error) {
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return "", "", err
	}

	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			Organization:  []string{"Test Org"},
			Country:       []string{"US"},
			Province:      []string{""},
			Locality:      []string{"Test City"},
			StreetAddress: []string{""},
			PostalCode:    []string{""},
		},
		NotBefore:    time.Now(),
		NotAfter:     time.Now().Add(365 * 24 * time.Hour),
		KeyUsage:     x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}

	if isCA {
		template.IsCA = true
		template.KeyUsage |= x509.KeyUsageCertSign
		template.BasicConstraintsValid = true
	}

	certDER, err := x509.CreateCertificate(rand.Reader, &template, &template, &priv.PublicKey, priv)
	if err != nil {
		return "", "", err
	}

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})

	privDER, err := x509.MarshalPKCS8PrivateKey(priv)
	if err != nil {
		return "", "", err
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: privDER})

	return string(certPEM), string(keyPEM), nil
}

func TestNewCertificateManager(t *testing.T) {
	cm := NewCertificateManager()

	assert.NotNil(t, cm)
	assert.NotNil(t, cm.certificates)
	assert.NotNil(t, cm.configs)
	assert.Equal(t, 0, len(cm.certificates))
	assert.Equal(t, 0, len(cm.configs))
}

func TestCertificateManager_AddCertificate(t *testing.T) {
	cm := NewCertificateManager()

	// Generate test certificate
	certPEM, keyPEM, err := generateTestCertificate(false)
	require.NoError(t, err)

	tests := []struct {
		name        string
		cert        *Certificate
		expectError bool
	}{
		{
			name: "valid certificate",
			cert: &Certificate{
				ID:      "test-cert-1",
				Name:    "Test Certificate",
				Type:    CertTypeServer,
				Content: certPEM,
				Key:     keyPEM,
				Enabled: true,
			},
			expectError: false,
		},
		{
			name: "empty ID",
			cert: &Certificate{
				Name:    "Test Certificate",
				Type:    CertTypeServer,
				Content: certPEM,
				Key:     keyPEM,
				Enabled: true,
			},
			expectError: true,
		},
		{
			name: "invalid certificate content",
			cert: &Certificate{
				ID:      "test-cert-2",
				Name:    "Invalid Certificate",
				Type:    CertTypeServer,
				Content: "invalid pem content",
				Key:     keyPEM,
				Enabled: true,
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := cm.AddCertificate(tt.cert)
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)

				// Verify certificate was added
				stored, err := cm.GetCertificate(tt.cert.ID)
				assert.NoError(t, err)
				assert.Equal(t, tt.cert.ID, stored.ID)
				assert.Equal(t, tt.cert.Name, stored.Name)
				assert.NotEmpty(t, stored.Subject)
				assert.NotEmpty(t, stored.Issuer)
				assert.NotEmpty(t, stored.Fingerprint)
				assert.False(t, stored.CreatedAt.IsZero())
				assert.False(t, stored.UpdatedAt.IsZero())
			}
		})
	}
}

func TestCertificateManager_GetCertificate(t *testing.T) {
	cm := NewCertificateManager()

	// Generate and add test certificate
	certPEM, keyPEM, err := generateTestCertificate(false)
	require.NoError(t, err)

	cert := &Certificate{
		ID:      "test-cert-1",
		Name:    "Test Certificate",
		Type:    CertTypeServer,
		Content: certPEM,
		Key:     keyPEM,
		Enabled: true,
	}

	err = cm.AddCertificate(cert)
	require.NoError(t, err)

	tests := []struct {
		name        string
		certID      string
		expectError bool
	}{
		{
			name:        "existing certificate",
			certID:      "test-cert-1",
			expectError: false,
		},
		{
			name:        "non-existent certificate",
			certID:      "non-existent",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			retrieved, err := cm.GetCertificate(tt.certID)
			if tt.expectError {
				assert.Error(t, err)
				assert.Nil(t, retrieved)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, retrieved)
				assert.Equal(t, tt.certID, retrieved.ID)
			}
		})
	}
}

func TestCertificateManager_ListCertificates(t *testing.T) {
	cm := NewCertificateManager()

	// Initially empty
	certs := cm.ListCertificates()
	assert.Equal(t, 0, len(certs))

	// Add test certificates
	certPEM, keyPEM, err := generateTestCertificate(false)
	require.NoError(t, err)

	for i := 1; i <= 3; i++ {
		cert := &Certificate{
			ID:      fmt.Sprintf("test-cert-%d", i),
			Name:    fmt.Sprintf("Test Certificate %d", i),
			Type:    CertTypeServer,
			Content: certPEM,
			Key:     keyPEM,
			Enabled: true,
		}
		err = cm.AddCertificate(cert)
		require.NoError(t, err)
	}

	// Verify list
	certs = cm.ListCertificates()
	assert.Equal(t, 3, len(certs))

	// Verify all certificates are present
	certIDs := make(map[string]bool)
	for _, cert := range certs {
		certIDs[cert.ID] = true
	}

	assert.True(t, certIDs["test-cert-1"])
	assert.True(t, certIDs["test-cert-2"])
	assert.True(t, certIDs["test-cert-3"])
}

func TestCertificateManager_DeleteCertificate(t *testing.T) {
	cm := NewCertificateManager()

	// Add test certificate
	certPEM, keyPEM, err := generateTestCertificate(false)
	require.NoError(t, err)

	cert := &Certificate{
		ID:      "test-cert-1",
		Name:    "Test Certificate",
		Type:    CertTypeServer,
		Content: certPEM,
		Key:     keyPEM,
		Enabled: true,
	}

	err = cm.AddCertificate(cert)
	require.NoError(t, err)

	tests := []struct {
		name        string
		certID      string
		expectError bool
	}{
		{
			name:        "delete existing certificate",
			certID:      "test-cert-1",
			expectError: false,
		},
		{
			name:        "delete non-existent certificate",
			certID:      "non-existent",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := cm.DeleteCertificate(tt.certID)
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)

				// Verify certificate was deleted
				_, err := cm.GetCertificate(tt.certID)
				assert.Error(t, err)
			}
		})
	}
}

func TestCertificateManager_ValidateCertificate(t *testing.T) {
	cm := NewCertificateManager()

	// Generate CA and client certificates
	caCertPEM, _, err := generateTestCertificate(true)
	require.NoError(t, err)

	clientCertPEM, _, err := generateTestCertificate(false)
	require.NoError(t, err)

	tests := []struct {
		name        string
		certContent string
		caCerts     []string
		expectError bool
	}{
		{
			name:        "valid certificate without CA validation",
			certContent: clientCertPEM,
			caCerts:     []string{},
			expectError: false,
		},
		{
			name:        "valid certificate with CA validation",
			certContent: clientCertPEM,
			caCerts:     []string{caCertPEM},
			expectError: true, // Will fail because client cert is self-signed, not signed by CA
		},
		{
			name:        "invalid certificate content",
			certContent: "invalid pem content",
			caCerts:     []string{},
			expectError: true,
		},
		{
			name:        "expired certificate",
			certContent: generateExpiredCertificate(t),
			caCerts:     []string{},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := cm.ValidateCertificate(tt.certContent, tt.caCerts...)
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func generateExpiredCertificate(t *testing.T) string {
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			Organization: []string{"Test Org"},
		},
		NotBefore: time.Now().Add(-48 * time.Hour),
		NotAfter:  time.Now().Add(-24 * time.Hour), // Expired yesterday
		KeyUsage:  x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
	}

	certDER, err := x509.CreateCertificate(rand.Reader, &template, &template, &priv.PublicKey, priv)
	require.NoError(t, err)

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	return string(certPEM)
}

func TestCertificateManager_SetTLSConfig(t *testing.T) {
	cm := NewCertificateManager()

	config := &TLSConfig{
		Enabled:        true,
		Port:           8883,
		Verify:         VerifyPeer,
		ClientCertAuth: true,
	}

	cm.SetTLSConfig("test-config", config)

	// Verify config was stored
	tlsConfig, err := cm.GetTLSConfig("test-config")
	assert.NoError(t, err)
	assert.NotNil(t, tlsConfig)
}

func TestCertificateManager_GetTLSConfig(t *testing.T) {
	cm := NewCertificateManager()

	tests := []struct {
		name        string
		configID    string
		setup       func()
		expectError bool
	}{
		{
			name:     "existing config",
			configID: "test-config",
			setup: func() {
				config := &TLSConfig{
					Enabled:        true,
					Port:           8883,
					Verify:         VerifyPeer,
					ClientCertAuth: true,
				}
				cm.SetTLSConfig("test-config", config)
			},
			expectError: false,
		},
		{
			name:        "non-existent config",
			configID:    "non-existent",
			setup:       func() {},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setup()

			tlsConfig, err := cm.GetTLSConfig(tt.configID)
			if tt.expectError {
				assert.Error(t, err)
				assert.Nil(t, tlsConfig)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, tlsConfig)
			}
		})
	}
}

func TestCertificateManager_GetTLSConfigWithCertificates(t *testing.T) {
	cm := NewCertificateManager()

	// Generate test certificates
	serverCertPEM, serverKeyPEM, err := generateTestCertificate(false)
	require.NoError(t, err)

	caCertPEM, _, err := generateTestCertificate(true)
	require.NoError(t, err)

	// Add certificates to manager
	serverCert := &Certificate{
		ID:      "server-cert",
		Name:    "Server Certificate",
		Type:    CertTypeServer,
		Content: serverCertPEM,
		Key:     serverKeyPEM,
		Enabled: true,
	}
	err = cm.AddCertificate(serverCert)
	require.NoError(t, err)

	caCert := &Certificate{
		ID:      "ca-cert",
		Name:    "CA Certificate",
		Type:    CertTypeCA,
		Content: caCertPEM,
		Enabled: true,
	}
	err = cm.AddCertificate(caCert)
	require.NoError(t, err)

	// Create TLS config
	config := &TLSConfig{
		Enabled:         true,
		Port:            8883,
		Verify:          VerifyPeer,
		ClientCertAuth:  true,
		CertificateID:   "server-cert",
		CACertificateID: "ca-cert",
	}
	cm.SetTLSConfig("test-config", config)

	// Get TLS config
	tlsConfig, err := cm.GetTLSConfig("test-config")
	assert.NoError(t, err)
	assert.NotNil(t, tlsConfig)
	assert.Equal(t, tls.VerifyClientCertIfGiven, tlsConfig.ClientAuth)
	assert.Len(t, tlsConfig.Certificates, 1)
	assert.NotNil(t, tlsConfig.ClientCAs)
}

func TestCertificateManager_IsExpiringSoon(t *testing.T) {
	cm := NewCertificateManager()

	// Generate test certificate that expires in 1 hour
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			Organization: []string{"Test Org"},
		},
		NotBefore: time.Now(),
		NotAfter:  time.Now().Add(1 * time.Hour), // Expires in 1 hour
		KeyUsage:  x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
	}

	certDER, err := x509.CreateCertificate(rand.Reader, &template, &template, &priv.PublicKey, priv)
	require.NoError(t, err)

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})

	cert := &Certificate{
		ID:      "expiring-cert",
		Name:    "Expiring Certificate",
		Type:    CertTypeServer,
		Content: string(certPEM),
		Enabled: true,
	}

	err = cm.AddCertificate(cert)
	require.NoError(t, err)

	// Test expiration checking
	assert.True(t, cm.IsExpiringSoon("expiring-cert", 2*time.Hour))   // Within 2 hours
	assert.False(t, cm.IsExpiringSoon("expiring-cert", 30*time.Minute)) // Within 30 minutes
	assert.False(t, cm.IsExpiringSoon("non-existent", 2*time.Hour))   // Non-existent cert
}

func TestCertificateManager_GetExpiringCertificates(t *testing.T) {
	cm := NewCertificateManager()

	// Add a certificate expiring soon and one expiring later
	for i, expireAfter := range []time.Duration{1 * time.Hour, 48 * time.Hour} {
		priv, err := rsa.GenerateKey(rand.Reader, 2048)
		require.NoError(t, err)

		template := x509.Certificate{
			SerialNumber: big.NewInt(int64(i + 1)),
			Subject: pkix.Name{
				Organization: []string{"Test Org"},
			},
			NotBefore: time.Now(),
			NotAfter:  time.Now().Add(expireAfter),
			KeyUsage:  x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		}

		certDER, err := x509.CreateCertificate(rand.Reader, &template, &template, &priv.PublicKey, priv)
		require.NoError(t, err)

		certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})

		cert := &Certificate{
			ID:      fmt.Sprintf("cert-%d", i+1),
			Name:    fmt.Sprintf("Certificate %d", i+1),
			Type:    CertTypeServer,
			Content: string(certPEM),
			Enabled: true,
		}

		err = cm.AddCertificate(cert)
		require.NoError(t, err)
	}

	// Get certificates expiring within 24 hours
	expiring := cm.GetExpiringCertificates(24 * time.Hour)
	assert.Len(t, expiring, 1)
	assert.Equal(t, "cert-1", expiring[0].ID)

	// Get certificates expiring within 72 hours
	expiring = cm.GetExpiringCertificates(72 * time.Hour)
	assert.Len(t, expiring, 2)
}

func TestTLSVerifyModeConstants(t *testing.T) {
	assert.Equal(t, TLSVerifyMode("none"), VerifyNone)
	assert.Equal(t, TLSVerifyMode("verify_peer"), VerifyPeer)
	assert.Equal(t, TLSVerifyMode("verify_peer_fail_if_no_peer_cert"), VerifyPeerFailIfNoCert)
}

func TestCertificateTypeConstants(t *testing.T) {
	assert.Equal(t, CertificateType("server"), CertTypeServer)
	assert.Equal(t, CertificateType("ca"), CertTypeCA)
	assert.Equal(t, CertificateType("client"), CertTypeClient)
}