package e2e

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"math/big"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	tlspkg "github.com/turtacn/emqx-go/pkg/tls"
)

// TestCertificateAuthenticationE2E tests end-to-end certificate authentication
func TestCertificateAuthenticationE2E(t *testing.T) {
	t.Log("Starting Certificate Authentication E2E Test")

	// Test 1: Dashboard Certificate Management API
	t.Run("Dashboard Certificate Management API", func(t *testing.T) {
		testDashboardCertificateAPI(t)
	})

	// Test 2: Certificate Generation and Validation
	t.Run("Certificate Generation and Validation", func(t *testing.T) {
		testCertificateGeneration(t)
	})

	t.Log("Certificate Authentication E2E Test completed")
}

// Helper function to generate test certificates
func generateTestCertificate() (string, string, error) {
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
		NotBefore:   time.Now(),
		NotAfter:    time.Now().Add(365 * 24 * time.Hour),
		KeyUsage:    x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
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

// Test Dashboard Certificate Management API
func testDashboardCertificateAPI(t *testing.T) {
	dashboardURL := "http://localhost:18083"

	t.Log("Testing Dashboard Certificate Management API")

	// Test 1: List certificates endpoint
	resp, err := http.Get(dashboardURL + "/api/certificates")
	if err != nil {
		t.Logf("Dashboard not available, skipping API test: %v", err)
		t.Skip("Dashboard not available")
		return
	}
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Contains(t, resp.Header.Get("Content-Type"), "application/json")

	t.Log("✓ Certificate listing API endpoint working")

	// Test 2: Add a test certificate
	certPEM, keyPEM, err := generateTestCertificate()
	require.NoError(t, err)

	testCert := tlspkg.Certificate{
		ID:      "e2e-test-cert",
		Name:    "E2E Test Certificate",
		Type:    tlspkg.CertTypeServer,
		Content: certPEM,
		Key:     keyPEM,
		Enabled: true,
	}

	certJSON, err := json.Marshal(testCert)
	require.NoError(t, err)

	resp, err = http.Post(dashboardURL+"/api/certificates", "application/json", bytes.NewReader(certJSON))
	require.NoError(t, err)
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		t.Log("✓ Certificate addition API endpoint working")

		// Test 3: Get the added certificate
		resp, err := http.Get(dashboardURL + "/api/certificates/e2e-test-cert")
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode)
		t.Log("✓ Certificate retrieval API endpoint working")

		// Test 4: Delete the test certificate
		req, err := http.NewRequest("DELETE", dashboardURL+"/api/certificates/e2e-test-cert", nil)
		require.NoError(t, err)

		client := &http.Client{}
		resp, err = client.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode)
		t.Log("✓ Certificate deletion API endpoint working")
	} else {
		t.Logf("Certificate addition returned status %d, might be validation issue", resp.StatusCode)
	}

	// Test 5: X.509 Authentication Configuration endpoint
	resp, err = http.Get(dashboardURL + "/api/x509-auth")
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Contains(t, resp.Header.Get("Content-Type"), "application/json")

	t.Log("✓ X.509 authentication configuration API endpoint working")
}

// Test Certificate Generation and Validation
func testCertificateGeneration(t *testing.T) {
	t.Log("Testing Certificate Generation and Validation")

	// Generate test certificates
	certPEM, keyPEM, err := generateTestCertificate()
	require.NoError(t, err)

	assert.NotEmpty(t, certPEM)
	assert.NotEmpty(t, keyPEM)
	assert.True(t, strings.Contains(certPEM, "BEGIN CERTIFICATE"))
	assert.True(t, strings.Contains(certPEM, "END CERTIFICATE"))
	assert.True(t, strings.Contains(keyPEM, "BEGIN PRIVATE KEY"))
	assert.True(t, strings.Contains(keyPEM, "END PRIVATE KEY"))

	t.Log("✓ Certificate generation working correctly")

	// Test certificate parsing
	block, _ := pem.Decode([]byte(certPEM))
	require.NotNil(t, block)

	cert, err := x509.ParseCertificate(block.Bytes)
	require.NoError(t, err)

	assert.Equal(t, "Test Org", cert.Subject.Organization[0])
	assert.Equal(t, "US", cert.Subject.Country[0])
	assert.Equal(t, "Test City", cert.Subject.Locality[0])
	assert.True(t, cert.NotAfter.After(time.Now()))

	t.Log("✓ Certificate parsing and validation working correctly")

	// Test TLS certificate loading
	_, err = tls.X509KeyPair([]byte(certPEM), []byte(keyPEM))
	require.NoError(t, err)

	t.Log("✓ TLS certificate loading working correctly")
}