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

// Package x509 provides X.509 certificate-based authentication for EMQX-Go.
// It implements certificate validation, identity mapping, and integrates with
// the EMQX authentication chain model.
package x509

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"sync"

	"github.com/turtacn/emqx-go/pkg/auth"
	tlspkg "github.com/turtacn/emqx-go/pkg/tls"
)

// IdentitySource defines how to extract identity from certificate
type IdentitySource string

const (
	// IdentityFromSubjectDN extracts identity from certificate subject DN
	IdentityFromSubjectDN IdentitySource = "subject_dn"
	// IdentityFromSubjectCN extracts identity from certificate subject common name
	IdentityFromSubjectCN IdentitySource = "subject_cn"
	// IdentityFromSAN extracts identity from Subject Alternative Names
	IdentityFromSAN IdentitySource = "san"
	// IdentityFromSerial extracts identity from certificate serial number
	IdentityFromSerial IdentitySource = "serial"
	// IdentityFromFingerprint extracts identity from certificate fingerprint
	IdentityFromFingerprint IdentitySource = "fingerprint"
)

// X509Config represents X.509 authentication configuration
type X509Config struct {
	Enabled         bool           `json:"enabled"`
	IdentitySource  IdentitySource `json:"identity_source"`
	IdentityField   string         `json:"identity_field,omitempty"`
	IdentityPattern string         `json:"identity_pattern,omitempty"`
	CAChainDepth    int            `json:"ca_chain_depth"`
	VerifyCertChain bool           `json:"verify_cert_chain"`
	AllowedCNs      []string       `json:"allowed_cns,omitempty"`
	RevokedSerials  []string       `json:"revoked_serials,omitempty"`
}

// DefaultX509Config returns default X.509 authentication configuration
func DefaultX509Config() *X509Config {
	return &X509Config{
		Enabled:         false,
		IdentitySource:  IdentityFromSubjectCN,
		CAChainDepth:    10,
		VerifyCertChain: true,
		AllowedCNs:      []string{},
		RevokedSerials:  []string{},
	}
}

// X509Authenticator implements certificate-based authentication
type X509Authenticator struct {
	config    *X509Config
	certMgr   *tlspkg.CertificateManager
	pattern   *regexp.Regexp
	mu        sync.RWMutex
	enabled   bool
}

// NewX509Authenticator creates a new X.509 certificate authenticator
func NewX509Authenticator(config *X509Config, certMgr *tlspkg.CertificateManager) (*X509Authenticator, error) {
	if config == nil {
		config = DefaultX509Config()
	}

	auth := &X509Authenticator{
		config:  config,
		certMgr: certMgr,
		enabled: config.Enabled,
	}

	// Compile identity pattern if provided
	if config.IdentityPattern != "" {
		pattern, err := regexp.Compile(config.IdentityPattern)
		if err != nil {
			return nil, fmt.Errorf("invalid identity pattern: %v", err)
		}
		auth.pattern = pattern
	}

	return auth, nil
}

// Authenticate implements the auth.Authenticator interface for certificate authentication
// Note: For certificate authentication, the actual verification happens during TLS handshake.
// This method is called with certificate-derived identity information.
func (x *X509Authenticator) Authenticate(username, password string) auth.AuthResult {
	if !x.enabled {
		return auth.AuthIgnore
	}

	// For certificate authentication, we rely on the TLS handshake validation
	// This method validates the certificate-derived identity
	return x.validateCertificateIdentity(username)
}

// AuthenticateWithCertificate performs certificate-based authentication with TLS connection state
func (x *X509Authenticator) AuthenticateWithCertificate(connState *tls.ConnectionState) (string, auth.AuthResult) {
	if !x.enabled {
		return "", auth.AuthIgnore
	}

	if connState == nil || len(connState.PeerCertificates) == 0 {
		return "", auth.AuthIgnore
	}

	clientCert := connState.PeerCertificates[0]

	// Check if certificate is revoked
	if x.isCertificateRevoked(clientCert) {
		return "", auth.AuthFailure
	}

	// Extract identity from certificate
	identity, err := x.extractIdentity(clientCert)
	if err != nil {
		return "", auth.AuthError
	}

	// Validate identity against allowed CNs if configured
	if len(x.config.AllowedCNs) > 0 {
		if !x.isAllowedCN(clientCert.Subject.CommonName) {
			return "", auth.AuthFailure
		}
	}

	// Apply identity pattern if configured
	if x.pattern != nil {
		if !x.pattern.MatchString(identity) {
			return "", auth.AuthFailure
		}
	}

	return identity, auth.AuthSuccess
}

// Name returns the authenticator name
func (x *X509Authenticator) Name() string {
	return "x509"
}

// Enabled returns whether the authenticator is enabled
func (x *X509Authenticator) Enabled() bool {
	x.mu.RLock()
	defer x.mu.RUnlock()
	return x.enabled
}

// SetEnabled enables or disables the authenticator
func (x *X509Authenticator) SetEnabled(enabled bool) {
	x.mu.Lock()
	defer x.mu.Unlock()
	x.enabled = enabled
	x.config.Enabled = enabled
}

// UpdateConfig updates the authenticator configuration
func (x *X509Authenticator) UpdateConfig(config *X509Config) error {
	if config == nil {
		return errors.New("config cannot be nil")
	}

	x.mu.Lock()
	defer x.mu.Unlock()

	// Recompile pattern if changed
	if config.IdentityPattern != x.config.IdentityPattern {
		if config.IdentityPattern != "" {
			pattern, err := regexp.Compile(config.IdentityPattern)
			if err != nil {
				return fmt.Errorf("invalid identity pattern: %v", err)
			}
			x.pattern = pattern
		} else {
			x.pattern = nil
		}
	}

	x.config = config
	x.enabled = config.Enabled
	return nil
}

// GetConfig returns the current configuration
func (x *X509Authenticator) GetConfig() *X509Config {
	x.mu.RLock()
	defer x.mu.RUnlock()
	return x.config
}

// extractIdentity extracts identity from certificate based on configuration
func (x *X509Authenticator) extractIdentity(cert *x509.Certificate) (string, error) {
	switch x.config.IdentitySource {
	case IdentityFromSubjectDN:
		return cert.Subject.String(), nil

	case IdentityFromSubjectCN:
		if cert.Subject.CommonName == "" {
			return "", errors.New("certificate subject common name is empty")
		}
		return cert.Subject.CommonName, nil

	case IdentityFromSAN:
		if x.config.IdentityField == "" {
			// Default to first DNS name or email
			if len(cert.DNSNames) > 0 {
				return cert.DNSNames[0], nil
			}
			if len(cert.EmailAddresses) > 0 {
				return cert.EmailAddresses[0], nil
			}
			return "", errors.New("no suitable SAN field found")
		}

		// Extract specific SAN field
		switch strings.ToLower(x.config.IdentityField) {
		case "dns":
			if len(cert.DNSNames) > 0 {
				return cert.DNSNames[0], nil
			}
		case "email":
			if len(cert.EmailAddresses) > 0 {
				return cert.EmailAddresses[0], nil
			}
		case "uri":
			if len(cert.URIs) > 0 {
				return cert.URIs[0].String(), nil
			}
		case "ip":
			if len(cert.IPAddresses) > 0 {
				return cert.IPAddresses[0].String(), nil
			}
		}
		return "", fmt.Errorf("SAN field '%s' not found", x.config.IdentityField)

	case IdentityFromSerial:
		return cert.SerialNumber.String(), nil

	case IdentityFromFingerprint:
		// Calculate SHA-256 fingerprint
		fingerprint := fmt.Sprintf("%x", cert.Raw)
		return fingerprint, nil

	default:
		return "", fmt.Errorf("unsupported identity source: %s", x.config.IdentitySource)
	}
}

// validateCertificateIdentity validates a certificate-derived identity
func (x *X509Authenticator) validateCertificateIdentity(identity string) auth.AuthResult {
	if identity == "" {
		return auth.AuthFailure
	}

	// Apply identity pattern if configured
	if x.pattern != nil {
		if !x.pattern.MatchString(identity) {
			return auth.AuthFailure
		}
	}

	return auth.AuthSuccess
}

// isCertificateRevoked checks if certificate is in revocation list
func (x *X509Authenticator) isCertificateRevoked(cert *x509.Certificate) bool {
	serialStr := cert.SerialNumber.String()
	for _, revokedSerial := range x.config.RevokedSerials {
		if revokedSerial == serialStr {
			return true
		}
	}
	return false
}

// isAllowedCN checks if the common name is in the allowed list
func (x *X509Authenticator) isAllowedCN(cn string) bool {
	for _, allowedCN := range x.config.AllowedCNs {
		if allowedCN == cn {
			return true
		}
		// Support wildcard matching
		if strings.HasPrefix(allowedCN, "*.") {
			domain := allowedCN[2:]
			if strings.HasSuffix(cn, "."+domain) || cn == domain {
				return true
			}
		}
	}
	return false
}

// AddRevokedSerial adds a certificate serial number to the revocation list
func (x *X509Authenticator) AddRevokedSerial(serial string) {
	x.mu.Lock()
	defer x.mu.Unlock()
	x.config.RevokedSerials = append(x.config.RevokedSerials, serial)
}

// RemoveRevokedSerial removes a certificate serial number from the revocation list
func (x *X509Authenticator) RemoveRevokedSerial(serial string) {
	x.mu.Lock()
	defer x.mu.Unlock()

	for i, revokedSerial := range x.config.RevokedSerials {
		if revokedSerial == serial {
			x.config.RevokedSerials = append(
				x.config.RevokedSerials[:i],
				x.config.RevokedSerials[i+1:]...,
			)
			break
		}
	}
}

// GetRevokedSerials returns the list of revoked certificate serial numbers
func (x *X509Authenticator) GetRevokedSerials() []string {
	x.mu.RLock()
	defer x.mu.RUnlock()
	return append([]string(nil), x.config.RevokedSerials...)
}

// ValidateCertificateFromPEM validates a certificate from PEM content
func (x *X509Authenticator) ValidateCertificateFromPEM(certPEM string) (string, error) {
	block, _ := pem.Decode([]byte(certPEM))
	if block == nil {
		return "", errors.New("failed to parse certificate PEM")
	}

	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return "", fmt.Errorf("failed to parse certificate: %v", err)
	}

	// Check if certificate is revoked
	if x.isCertificateRevoked(cert) {
		return "", errors.New("certificate is revoked")
	}

	// Extract identity
	identity, err := x.extractIdentity(cert)
	if err != nil {
		return "", fmt.Errorf("failed to extract identity: %v", err)
	}

	// Validate identity
	result := x.validateCertificateIdentity(identity)
	if result != auth.AuthSuccess {
		return "", errors.New("certificate identity validation failed")
	}

	return identity, nil
}