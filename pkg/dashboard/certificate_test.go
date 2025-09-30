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

package dashboard

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/turtacn/emqx-go/pkg/admin"
	authx509 "github.com/turtacn/emqx-go/pkg/auth/x509"
	"github.com/turtacn/emqx-go/pkg/metrics"
	"github.com/turtacn/emqx-go/pkg/monitor"
	tlspkg "github.com/turtacn/emqx-go/pkg/tls"
)

// Helper function to generate test certificates for dashboard tests
func generateDashboardTestCertificate() (string, string, error) {
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return "", "", err
	}

	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			Organization:  []string{"Dashboard Test Org"},
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

func createCertificateTestServer(t *testing.T) *Server {
	config := &Config{
		Address:    "127.0.0.1",
		Port:       18083,
		Username:   "admin",
		Password:   "public",
		EnableAuth: false, // Disable auth for easier testing
	}

	metricsManager := metrics.NewMetricsManager()
	healthChecker := monitor.NewHealthChecker()
	mockBroker := &mockBrokerInterface{}
	adminAPI := admin.NewAPIServer(metricsManager, mockBroker)
	certManager := tlspkg.NewCertificateManager()

	server, err := NewServer(config, adminAPI, metricsManager, healthChecker, certManager)
	require.NoError(t, err)
	require.NotNil(t, server)

	return server
}

func TestHandleCertificatesPage(t *testing.T) {
	server := createCertificateTestServer(t)

	req, err := http.NewRequest("GET", "/certificates", nil)
	require.NoError(t, err)

	rr := httptest.NewRecorder()
	server.handleCertificates(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	assert.Contains(t, rr.Header().Get("Content-Type"), "text/html")
}

func TestHandleTLSConfigPage(t *testing.T) {
	server := createCertificateTestServer(t)

	req, err := http.NewRequest("GET", "/tls-config", nil)
	require.NoError(t, err)

	rr := httptest.NewRecorder()
	server.handleTLSConfig(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	assert.Contains(t, rr.Header().Get("Content-Type"), "text/html")
}

func TestHandleAPICertificates_GET(t *testing.T) {
	server := createCertificateTestServer(t)

	// Add a test certificate first
	certPEM, keyPEM, err := generateDashboardTestCertificate()
	require.NoError(t, err)

	cert := &tlspkg.Certificate{
		ID:      "test-cert-1",
		Name:    "Test Certificate",
		Type:    tlspkg.CertTypeServer,
		Content: certPEM,
		Key:     keyPEM,
		Enabled: true,
	}

	err = server.certificateManager.AddCertificate(cert)
	require.NoError(t, err)

	req, err := http.NewRequest("GET", "/api/certificates", nil)
	require.NoError(t, err)

	rr := httptest.NewRecorder()
	server.handleAPICertificates(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	assert.Equal(t, "application/json", rr.Header().Get("Content-Type"))

	var response map[string]interface{}
	err = json.Unmarshal(rr.Body.Bytes(), &response)
	require.NoError(t, err)

	assert.Contains(t, response, "certificates")
	assert.Contains(t, response, "count")

	count, ok := response["count"].(float64)
	require.True(t, ok)
	assert.Equal(t, float64(1), count)

	certificates, ok := response["certificates"].([]interface{})
	require.True(t, ok)
	assert.Len(t, certificates, 1)
}

func TestHandleAPICertificates_POST(t *testing.T) {
	server := createCertificateTestServer(t)

	certPEM, keyPEM, err := generateDashboardTestCertificate()
	require.NoError(t, err)

	cert := tlspkg.Certificate{
		ID:      "test-cert-2",
		Name:    "Test Certificate 2",
		Type:    tlspkg.CertTypeServer,
		Content: certPEM,
		Key:     keyPEM,
		Enabled: true,
	}

	certJSON, err := json.Marshal(cert)
	require.NoError(t, err)

	tests := []struct {
		name           string
		body           string
		expectedStatus int
	}{
		{
			name:           "valid certificate",
			body:           string(certJSON),
			expectedStatus: http.StatusOK,
		},
		{
			name:           "invalid JSON",
			body:           "invalid json",
			expectedStatus: http.StatusBadRequest,
		},
		{
			name: "empty certificate ID",
			body: `{
				"name": "Test Certificate",
				"type": "server",
				"content": "` + strings.ReplaceAll(certPEM, "\n", "\\n") + `",
				"enabled": true
			}`,
			expectedStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, err := http.NewRequest("POST", "/api/certificates", strings.NewReader(tt.body))
			require.NoError(t, err)
			req.Header.Set("Content-Type", "application/json")

			rr := httptest.NewRecorder()
			server.handleAPICertificates(rr, req)

			assert.Equal(t, tt.expectedStatus, rr.Code)

			if tt.expectedStatus == http.StatusOK {
				var response map[string]interface{}
				err = json.Unmarshal(rr.Body.Bytes(), &response)
				require.NoError(t, err)

				assert.Contains(t, response, "message")
				assert.Contains(t, response, "id")
			}
		})
	}
}

func TestHandleAPICertificateByID_GET(t *testing.T) {
	server := createCertificateTestServer(t)

	// Add a test certificate first
	certPEM, keyPEM, err := generateDashboardTestCertificate()
	require.NoError(t, err)

	cert := &tlspkg.Certificate{
		ID:      "test-cert-3",
		Name:    "Test Certificate 3",
		Type:    tlspkg.CertTypeServer,
		Content: certPEM,
		Key:     keyPEM,
		Enabled: true,
	}

	err = server.certificateManager.AddCertificate(cert)
	require.NoError(t, err)

	tests := []struct {
		name           string
		certID         string
		expectedStatus int
	}{
		{
			name:           "existing certificate",
			certID:         "test-cert-3",
			expectedStatus: http.StatusOK,
		},
		{
			name:           "non-existent certificate",
			certID:         "non-existent",
			expectedStatus: http.StatusNotFound,
		},
		{
			name:           "empty certificate ID",
			certID:         "",
			expectedStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			url := "/api/certificates/" + tt.certID
			req, err := http.NewRequest("GET", url, nil)
			require.NoError(t, err)

			rr := httptest.NewRecorder()
			server.handleAPICertificateByID(rr, req)

			assert.Equal(t, tt.expectedStatus, rr.Code)

			if tt.expectedStatus == http.StatusOK {
				var retrievedCert tlspkg.Certificate
				err = json.Unmarshal(rr.Body.Bytes(), &retrievedCert)
				require.NoError(t, err)

				assert.Equal(t, tt.certID, retrievedCert.ID)
				assert.Equal(t, "Test Certificate 3", retrievedCert.Name)
			}
		})
	}
}

func TestHandleAPICertificateByID_DELETE(t *testing.T) {
	server := createCertificateTestServer(t)

	// Add a test certificate first
	certPEM, keyPEM, err := generateDashboardTestCertificate()
	require.NoError(t, err)

	cert := &tlspkg.Certificate{
		ID:      "test-cert-4",
		Name:    "Test Certificate 4",
		Type:    tlspkg.CertTypeServer,
		Content: certPEM,
		Key:     keyPEM,
		Enabled: true,
	}

	err = server.certificateManager.AddCertificate(cert)
	require.NoError(t, err)

	tests := []struct {
		name           string
		certID         string
		expectedStatus int
	}{
		{
			name:           "delete existing certificate",
			certID:         "test-cert-4",
			expectedStatus: http.StatusOK,
		},
		{
			name:           "delete non-existent certificate",
			certID:         "non-existent",
			expectedStatus: http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			url := "/api/certificates/" + tt.certID
			req, err := http.NewRequest("DELETE", url, nil)
			require.NoError(t, err)

			rr := httptest.NewRecorder()
			server.handleAPICertificateByID(rr, req)

			assert.Equal(t, tt.expectedStatus, rr.Code)

			if tt.expectedStatus == http.StatusOK {
				var response map[string]interface{}
				err = json.Unmarshal(rr.Body.Bytes(), &response)
				require.NoError(t, err)

				assert.Contains(t, response, "message")

				// Verify certificate was actually deleted
				_, err = server.certificateManager.GetCertificate(tt.certID)
				assert.Error(t, err)
			}
		})
	}
}

func TestHandleAPITLSConfig_POST(t *testing.T) {
	server := createCertificateTestServer(t)

	tlsConfig := tlspkg.TLSConfig{
		Enabled:        true,
		Port:           8883,
		Verify:         tlspkg.VerifyPeer,
		ClientCertAuth: true,
	}

	configJSON, err := json.Marshal(tlsConfig)
	require.NoError(t, err)

	tests := []struct {
		name           string
		body           string
		expectedStatus int
	}{
		{
			name:           "valid TLS config",
			body:           string(configJSON),
			expectedStatus: http.StatusOK,
		},
		{
			name:           "invalid JSON",
			body:           "invalid json",
			expectedStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, err := http.NewRequest("POST", "/api/tls-config", strings.NewReader(tt.body))
			require.NoError(t, err)
			req.Header.Set("Content-Type", "application/json")

			rr := httptest.NewRecorder()
			server.handleAPITLSConfig(rr, req)

			assert.Equal(t, tt.expectedStatus, rr.Code)

			if tt.expectedStatus == http.StatusOK {
				var response map[string]interface{}
				err = json.Unmarshal(rr.Body.Bytes(), &response)
				require.NoError(t, err)

				assert.Contains(t, response, "message")
				assert.Contains(t, response, "config_id")
			}
		})
	}
}

func TestHandleAPIX509Auth_GET(t *testing.T) {
	server := createCertificateTestServer(t)

	req, err := http.NewRequest("GET", "/api/x509-auth", nil)
	require.NoError(t, err)

	rr := httptest.NewRecorder()
	server.handleAPIX509Auth(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	assert.Equal(t, "application/json", rr.Header().Get("Content-Type"))

	var config authx509.X509Config
	err = json.Unmarshal(rr.Body.Bytes(), &config)
	require.NoError(t, err)

	// Verify it returns default config
	defaultConfig := authx509.DefaultX509Config()
	assert.Equal(t, defaultConfig.Enabled, config.Enabled)
	assert.Equal(t, defaultConfig.IdentitySource, config.IdentitySource)
	assert.Equal(t, defaultConfig.CAChainDepth, config.CAChainDepth)
}

func TestHandleAPIX509Auth_POST(t *testing.T) {
	server := createCertificateTestServer(t)

	x509Config := authx509.X509Config{
		Enabled:         true,
		IdentitySource:  authx509.IdentityFromSubjectCN,
		IdentityPattern: "^test-.*",
		CAChainDepth:    5,
		VerifyCertChain: true,
		AllowedCNs:      []string{"test-client"},
	}

	configJSON, err := json.Marshal(x509Config)
	require.NoError(t, err)

	tests := []struct {
		name           string
		body           string
		expectedStatus int
	}{
		{
			name:           "valid X.509 config",
			body:           string(configJSON),
			expectedStatus: http.StatusOK,
		},
		{
			name:           "invalid JSON",
			body:           "invalid json",
			expectedStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, err := http.NewRequest("POST", "/api/x509-auth", strings.NewReader(tt.body))
			require.NoError(t, err)
			req.Header.Set("Content-Type", "application/json")

			rr := httptest.NewRecorder()
			server.handleAPIX509Auth(rr, req)

			assert.Equal(t, tt.expectedStatus, rr.Code)

			if tt.expectedStatus == http.StatusOK {
				var response map[string]interface{}
				err = json.Unmarshal(rr.Body.Bytes(), &response)
				require.NoError(t, err)

				assert.Contains(t, response, "message")
				assert.Contains(t, response, "config")
			}
		})
	}
}

func TestGetCertificatesData(t *testing.T) {
	server := createCertificateTestServer(t)

	// Add test certificates
	certPEM, keyPEM, err := generateDashboardTestCertificate()
	require.NoError(t, err)

	for i := 1; i <= 2; i++ {
		cert := &tlspkg.Certificate{
			ID:      fmt.Sprintf("test-cert-%d", i),
			Name:    fmt.Sprintf("Test Certificate %d", i),
			Type:    tlspkg.CertTypeServer,
			Content: certPEM,
			Key:     keyPEM,
			Enabled: true,
		}
		err = server.certificateManager.AddCertificate(cert)
		require.NoError(t, err)
	}

	data := server.getCertificatesData()

	assert.Contains(t, data, "Title")
	assert.Contains(t, data, "Certificates")
	assert.Contains(t, data, "Count")

	assert.Equal(t, "Certificates - EMQX Go", data["Title"])
	assert.Equal(t, 2, data["Count"])

	certificates, ok := data["Certificates"].([]*tlspkg.Certificate)
	require.True(t, ok)
	assert.Len(t, certificates, 2)
}

func TestGetTLSConfigData(t *testing.T) {
	server := createCertificateTestServer(t)

	data := server.getTLSConfigData()

	assert.Contains(t, data, "Title")
	assert.Contains(t, data, "VerifyModes")
	assert.Contains(t, data, "CertTypes")
	assert.Contains(t, data, "IdentitySources")

	assert.Equal(t, "TLS Configuration - EMQX Go", data["Title"])

	verifyModes, ok := data["VerifyModes"].([]map[string]interface{})
	require.True(t, ok)
	assert.Len(t, verifyModes, 3)

	certTypes, ok := data["CertTypes"].([]map[string]interface{})
	require.True(t, ok)
	assert.Len(t, certTypes, 3)

	identitySources, ok := data["IdentitySources"].([]map[string]interface{})
	require.True(t, ok)
	assert.Len(t, identitySources, 5)
}

func TestAPIMethodNotAllowed(t *testing.T) {
	server := createCertificateTestServer(t)

	tests := []struct {
		name     string
		method   string
		endpoint string
	}{
		{
			name:     "PUT certificates",
			method:   "PUT",
			endpoint: "/api/certificates",
		},
		{
			name:     "PATCH certificate by ID",
			method:   "PATCH",
			endpoint: "/api/certificates/test-cert",
		},
		{
			name:     "GET TLS config",
			method:   "GET",
			endpoint: "/api/tls-config",
		},
		{
			name:     "DELETE X.509 auth",
			method:   "DELETE",
			endpoint: "/api/x509-auth",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, err := http.NewRequest(tt.method, tt.endpoint, nil)
			require.NoError(t, err)

			rr := httptest.NewRecorder()

			// Route to appropriate handler based on endpoint
			switch {
			case strings.HasPrefix(tt.endpoint, "/api/certificates/"):
				server.handleAPICertificateByID(rr, req)
			case tt.endpoint == "/api/certificates":
				server.handleAPICertificates(rr, req)
			case strings.HasPrefix(tt.endpoint, "/api/tls-config/"):
				server.handleAPITLSConfigByID(rr, req)
			case tt.endpoint == "/api/tls-config":
				server.handleAPITLSConfig(rr, req)
			case tt.endpoint == "/api/x509-auth":
				server.handleAPIX509Auth(rr, req)
			}

			assert.Equal(t, http.StatusMethodNotAllowed, rr.Code)
		})
	}
}

func TestCertificateValidationInAPI(t *testing.T) {
	server := createCertificateTestServer(t)

	tests := []struct {
		name           string
		cert           tlspkg.Certificate
		expectedStatus int
	}{
		{
			name: "certificate with invalid content",
			cert: tlspkg.Certificate{
				ID:      "invalid-cert",
				Name:    "Invalid Certificate",
				Type:    tlspkg.CertTypeServer,
				Content: "invalid certificate content",
				Enabled: true,
			},
			expectedStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			certJSON, err := json.Marshal(tt.cert)
			require.NoError(t, err)

			req, err := http.NewRequest("POST", "/api/certificates", bytes.NewReader(certJSON))
			require.NoError(t, err)
			req.Header.Set("Content-Type", "application/json")

			rr := httptest.NewRecorder()
			server.handleAPICertificates(rr, req)

			assert.Equal(t, tt.expectedStatus, rr.Code)
		})
	}
}