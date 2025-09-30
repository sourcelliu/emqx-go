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

// Package tls provides TLS certificate management and configuration for EMQX-Go.
// It implements X.509 certificate handling, validation, storage, and management
// functionality compatible with EMQX certificate authentication features.
package tls

import (
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"fmt"
	"sync"
	"time"
)

// CertificateType represents the type of certificate
type CertificateType string

const (
	// CertTypeServer represents server certificates for TLS listeners
	CertTypeServer CertificateType = "server"
	// CertTypeCA represents Certificate Authority certificates
	CertTypeCA CertificateType = "ca"
	// CertTypeClient represents client certificates for mutual TLS
	CertTypeClient CertificateType = "client"
)

// Certificate represents a stored certificate with metadata
type Certificate struct {
	ID          string          `json:"id"`
	Name        string          `json:"name"`
	Type        CertificateType `json:"type"`
	Content     string          `json:"content"`
	Key         string          `json:"key,omitempty"`
	Chain       []string        `json:"chain,omitempty"`
	Fingerprint string          `json:"fingerprint"`
	Subject     string          `json:"subject"`
	Issuer      string          `json:"issuer"`
	NotBefore   time.Time       `json:"not_before"`
	NotAfter    time.Time       `json:"not_after"`
	CreatedAt   time.Time       `json:"created_at"`
	UpdatedAt   time.Time       `json:"updated_at"`
	Enabled     bool            `json:"enabled"`
}

// CertificateInfo contains parsed certificate information
type CertificateInfo struct {
	Subject     string
	Issuer      string
	SerialNumber string
	NotBefore   time.Time
	NotAfter    time.Time
	DNSNames    []string
	IPAddresses []string
	Fingerprint string
	Extensions  map[string]string
}

// TLSVerifyMode defines TLS certificate verification mode
type TLSVerifyMode string

const (
	// VerifyNone - No certificate verification
	VerifyNone TLSVerifyMode = "none"
	// VerifyPeer - Verify peer certificate
	VerifyPeer TLSVerifyMode = "verify_peer"
	// VerifyPeerFailIfNoCert - Verify peer certificate and fail if not provided
	VerifyPeerFailIfNoCert TLSVerifyMode = "verify_peer_fail_if_no_peer_cert"
)

// TLSConfig represents TLS configuration for listeners
type TLSConfig struct {
	Enabled          bool          `json:"enabled"`
	Port             int           `json:"port"`
	CertFile         string        `json:"certfile"`
	KeyFile          string        `json:"keyfile"`
	CACertFile       string        `json:"cacertfile"`
	Verify           TLSVerifyMode `json:"verify"`
	Ciphers          []string      `json:"ciphers"`
	Versions         []string      `json:"versions"`
	ClientCertAuth   bool          `json:"client_cert_auth"`
	ReuseSession     bool          `json:"reuse_session"`
	HonorCipherOrder bool          `json:"honor_cipher_order"`
	ServerName       string        `json:"server_name,omitempty"`
	CertificateID    string        `json:"certificate_id,omitempty"`
	CACertificateID  string        `json:"ca_certificate_id,omitempty"`
}

// CertificateManager manages certificates and TLS configurations
type CertificateManager struct {
	certificates map[string]*Certificate
	configs      map[string]*TLSConfig
	mu           sync.RWMutex
}

// NewCertificateManager creates a new certificate manager
func NewCertificateManager() *CertificateManager {
	return &CertificateManager{
		certificates: make(map[string]*Certificate),
		configs:      make(map[string]*TLSConfig),
	}
}

// AddCertificate adds a new certificate to the manager
func (cm *CertificateManager) AddCertificate(cert *Certificate) error {
	if cert.ID == "" {
		return errors.New("certificate ID cannot be empty")
	}

	cm.mu.Lock()
	defer cm.mu.Unlock()

	// Validate certificate content
	info, err := cm.parseCertificate(cert.Content)
	if err != nil {
		return fmt.Errorf("invalid certificate content: %v", err)
	}

	// Update certificate metadata
	cert.Subject = info.Subject
	cert.Issuer = info.Issuer
	cert.NotBefore = info.NotBefore
	cert.NotAfter = info.NotAfter
	cert.Fingerprint = info.Fingerprint
	cert.UpdatedAt = time.Now()

	if cert.CreatedAt.IsZero() {
		cert.CreatedAt = time.Now()
	}

	cm.certificates[cert.ID] = cert
	return nil
}

// GetCertificate retrieves a certificate by ID
func (cm *CertificateManager) GetCertificate(id string) (*Certificate, error) {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	cert, exists := cm.certificates[id]
	if !exists {
		return nil, fmt.Errorf("certificate with ID %s not found", id)
	}

	return cert, nil
}

// ListCertificates returns all certificates
func (cm *CertificateManager) ListCertificates() []*Certificate {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	certs := make([]*Certificate, 0, len(cm.certificates))
	for _, cert := range cm.certificates {
		certs = append(certs, cert)
	}

	return certs
}

// DeleteCertificate removes a certificate by ID
func (cm *CertificateManager) DeleteCertificate(id string) error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	if _, exists := cm.certificates[id]; !exists {
		return fmt.Errorf("certificate with ID %s not found", id)
	}

	delete(cm.certificates, id)
	return nil
}

// ValidateCertificate validates a certificate against CA certificates
func (cm *CertificateManager) ValidateCertificate(certContent string, caCerts ...string) error {
	clientCert, err := cm.parseCertificate(certContent)
	if err != nil {
		return fmt.Errorf("invalid client certificate: %v", err)
	}

	// Check certificate expiration
	now := time.Now()
	if now.Before(clientCert.NotBefore) {
		return errors.New("certificate is not yet valid")
	}
	if now.After(clientCert.NotAfter) {
		return errors.New("certificate has expired")
	}

	// If no CA certificates provided, skip chain validation
	if len(caCerts) == 0 {
		return nil
	}

	// Create certificate pool from CA certificates
	pool := x509.NewCertPool()
	for _, caCert := range caCerts {
		block, _ := pem.Decode([]byte(caCert))
		if block == nil {
			continue
		}

		cert, err := x509.ParseCertificate(block.Bytes)
		if err != nil {
			continue
		}

		pool.AddCert(cert)
	}

	// Parse client certificate for validation
	block, _ := pem.Decode([]byte(certContent))
	if block == nil {
		return errors.New("failed to parse certificate PEM")
	}

	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return fmt.Errorf("failed to parse certificate: %v", err)
	}

	// Verify certificate chain
	opts := x509.VerifyOptions{
		Roots: pool,
	}

	_, err = cert.Verify(opts)
	if err != nil {
		return fmt.Errorf("certificate verification failed: %v", err)
	}

	return nil
}

// GetTLSConfig creates a Go TLS config from certificate manager settings
func (cm *CertificateManager) GetTLSConfig(configID string) (*tls.Config, error) {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	config, exists := cm.configs[configID]
	if !exists {
		return nil, fmt.Errorf("TLS config with ID %s not found", configID)
	}

	tlsConfig := &tls.Config{
		ServerName: config.ServerName,
	}

	// Configure client certificate authentication
	switch config.Verify {
	case VerifyNone:
		tlsConfig.ClientAuth = tls.NoClientCert
	case VerifyPeer:
		tlsConfig.ClientAuth = tls.VerifyClientCertIfGiven
	case VerifyPeerFailIfNoCert:
		tlsConfig.ClientAuth = tls.RequireAndVerifyClientCert
	}

	// Load server certificate
	if config.CertificateID != "" {
		cert, exists := cm.certificates[config.CertificateID]
		if !exists {
			return nil, fmt.Errorf("server certificate with ID %s not found", config.CertificateID)
		}

		tlsCert, err := tls.X509KeyPair([]byte(cert.Content), []byte(cert.Key))
		if err != nil {
			return nil, fmt.Errorf("failed to load server certificate: %v", err)
		}

		tlsConfig.Certificates = []tls.Certificate{tlsCert}
	}

	// Load CA certificates
	if config.CACertificateID != "" && tlsConfig.ClientAuth != tls.NoClientCert {
		caCert, exists := cm.certificates[config.CACertificateID]
		if !exists {
			return nil, fmt.Errorf("CA certificate with ID %s not found", config.CACertificateID)
		}

		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM([]byte(caCert.Content)) {
			return nil, errors.New("failed to parse CA certificate")
		}

		tlsConfig.ClientCAs = pool
	}

	return tlsConfig, nil
}

// SetTLSConfig stores a TLS configuration
func (cm *CertificateManager) SetTLSConfig(id string, config *TLSConfig) {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	cm.configs[id] = config
}

// parseCertificate parses a PEM certificate and extracts information
func (cm *CertificateManager) parseCertificate(certPEM string) (*CertificateInfo, error) {
	block, _ := pem.Decode([]byte(certPEM))
	if block == nil {
		return nil, errors.New("failed to parse certificate PEM")
	}

	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse certificate: %v", err)
	}

	// Calculate fingerprint
	fingerprint := sha256.Sum256(cert.Raw)

	info := &CertificateInfo{
		Subject:      cert.Subject.String(),
		Issuer:       cert.Issuer.String(),
		SerialNumber: cert.SerialNumber.String(),
		NotBefore:    cert.NotBefore,
		NotAfter:     cert.NotAfter,
		DNSNames:     cert.DNSNames,
		Fingerprint:  hex.EncodeToString(fingerprint[:]),
		Extensions:   make(map[string]string),
	}

	// Extract IP addresses
	for _, ip := range cert.IPAddresses {
		info.IPAddresses = append(info.IPAddresses, ip.String())
	}

	return info, nil
}

// IsExpiringSoon checks if a certificate expires within the given duration
func (cm *CertificateManager) IsExpiringSoon(certID string, within time.Duration) bool {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	cert, exists := cm.certificates[certID]
	if !exists {
		return false
	}

	return time.Until(cert.NotAfter) <= within
}

// GetExpiringCertificates returns certificates expiring within the given duration
func (cm *CertificateManager) GetExpiringCertificates(within time.Duration) []*Certificate {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	var expiring []*Certificate
	threshold := time.Now().Add(within)

	for _, cert := range cm.certificates {
		if cert.NotAfter.Before(threshold) {
			expiring = append(expiring, cert)
		}
	}

	return expiring
}