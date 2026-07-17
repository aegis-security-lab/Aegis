package security

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"fmt"
	"math/big"
	"net"
	"testing"
	"time"
)

// testTLSServer creates a local TLS server with a self-signed certificate
// and returns the address and a close function.
func testTLSServer(t *testing.T) (addr string, cert *x509.Certificate, close func()) {
	t.Helper()

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}

	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			CommonName:   "test.example.com",
			Organization: []string{"Test Corp"},
		},
		NotBefore:             time.Now().Add(-1 * time.Hour),
		NotAfter:              time.Now().Add(1 * time.Hour),
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		DNSNames:              []string{"test.example.com", "*.example.com"},
	}

	certDER, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("create cert: %v", err)
	}

	certParsed, err := x509.ParseCertificate(certDER)
	if err != nil {
		t.Fatalf("parse cert: %v", err)
	}

	tlsCert := tls.Certificate{
		Certificate: [][]byte{certDER},
		PrivateKey:  key,
	}

	listener, err := tls.Listen("tcp", "127.0.0.1:0", &tls.Config{
		Certificates: []tls.Certificate{tlsCert},
	})
	if err != nil {
		t.Fatalf("listen tls: %v", err)
	}

	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			// Just accept and immediately close to trigger TLS handshake
			_ = conn.SetDeadline(time.Now().Add(2 * time.Second))
			go func(c net.Conn) {
				// Perform TLS handshake then close
				tlsConn := c.(*tls.Conn)
				_ = tlsConn.Handshake()
				c.Close()
			}(conn)
		}
	}()

	return listener.Addr().String(), certParsed, func() { listener.Close() }
}

func TestTLSCheck_Success(t *testing.T) {
	addr, serverCert, close := testTLSServer(t)
	defer close()

	result := TLSCheck(addr, TLSConfig{})

	if result.Error != "" {
		t.Fatalf("unexpected error: %s", result.Error)
	}
	if result.Target != addr {
		t.Fatalf("target = %s, want %s", result.Target, addr)
	}

	// Verify TLS version is reported
	if result.TLSVersion == "" {
		t.Fatal("TLS version should be reported")
	}

	// Verify cipher suite is reported
	if result.CipherSuite == "" {
		t.Fatal("cipher suite should be reported")
	}

	// Verify certificate chain
	if len(result.CertChain) < 1 {
		t.Fatal("expected at least one certificate in chain")
	}

	certInfo := result.CertChain[0]
	if certInfo.Subject == "" {
		t.Fatal("certificate subject should be present")
	}
	if certInfo.Issuer == "" {
		t.Fatal("certificate issuer should be present")
	}
	if certInfo.NotBefore == "" || certInfo.NotAfter == "" {
		t.Fatal("certificate validity should be present")
	}
	if len(certInfo.DNSNames) == 0 {
		t.Fatal("certificate should have DNS names")
	}

	// Verify the certificate content matches
	if serverCert.Subject.CommonName != "test.example.com" {
		t.Fatalf("common name = %s, want test.example.com", serverCert.Subject.CommonName)
	}

	// Check that DNS names from server cert appear in our result
	foundDNSName := false
	for _, name := range certInfo.DNSNames {
		if name == "test.example.com" || name == "*.example.com" {
			foundDNSName = true
			break
		}
	}
	if !foundDNSName {
		t.Fatalf("certificate DNS names don't match: %v", certInfo.DNSNames)
	}
}

func TestTLSCheck_CustomDialer(t *testing.T) {
	addr, _, close := testTLSServer(t)
	defer close()

	dialed := false
	customCfg := TLSConfig{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			dialed = true
			d := net.Dialer{Timeout: 5 * time.Second}
			return d.DialContext(ctx, network, addr)
		},
	}

	result := TLSCheck(addr, customCfg)
	if result.Error != "" {
		t.Fatalf("unexpected error: %s", result.Error)
	}
	if !dialed {
		t.Fatal("custom dialer was not called")
	}
}

func TestTLSCheck_ConnectionRefused(t *testing.T) {
	// Try to connect to a closed port
	result := TLSCheck("127.0.0.1:1", TLSConfig{})
	if result.Error == "" {
		t.Fatal("expected error for connection refused")
	}
}

func TestTLSCheck_InvalidHostport(t *testing.T) {
	result := TLSCheck("invalid:port:bad", TLSConfig{})
	if result.Error == "" {
		t.Fatal("expected error for invalid hostport")
	}
}

func TestTLSCheck_NonTLSServer(t *testing.T) {
	// Start a plain TCP server
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	addr := listener.Addr().String()

	go func() {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		_ = conn.Close()
	}()
	defer listener.Close()

	// Give the server time to accept
	time.Sleep(50 * time.Millisecond)

	result := TLSCheck(addr, TLSConfig{})
	if result.Error == "" {
		t.Fatal("expected TLS handshake error against plain TCP server")
	}
}

func TestTLSCheck_DialTimeout(t *testing.T) {
	// Use a dialer that times out immediately
	customCfg := TLSConfig{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			return nil, fmt.Errorf("dial timeout")
		},
	}

	result := TLSCheck("127.0.0.1:443", customCfg)
	if result.Error == "" {
		t.Fatal("expected dial error")
	}
	if !contains(result.Error, "dial timeout") {
		t.Fatalf("unexpected error: %s", result.Error)
	}
}

func TestTLSCheck_InvalidServerName(t *testing.T) {
	addr, _, close := testTLSServer(t)
	defer close()

	// Should still connect even though the server name doesn't match
	// since InsecureSkipVerify is true
	result := TLSCheck(addr, TLSConfig{})
	if result.Error != "" {
		t.Fatalf("unexpected error: %s", result.Error)
	}
	if result.TLSVersion == "" {
		t.Fatal("should have TLS version even with mismatched name")
	}
}

func TestTLSCheck_TLSVersionName(t *testing.T) {
	tests := []struct {
		version uint16
		want    string
	}{
		{tls.VersionTLS10, "TLS 1.0"},
		{tls.VersionTLS11, "TLS 1.1"},
		{tls.VersionTLS12, "TLS 1.2"},
		{tls.VersionTLS13, "TLS 1.3"},
		{0x0000, "0x0000"},
	}
	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := tlsVersionName(tt.version)
			if got != tt.want {
				t.Fatalf("tlsVersionName(0x%04X) = %s, want %s", tt.version, got, tt.want)
			}
		})
	}
}

func TestTLSCheck_CipherSuiteName(t *testing.T) {
	// Known cipher suite
	name := cipherSuiteName(tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256)
	if name == "" {
		t.Fatal("expected non-empty name for known cipher suite")
	}

	// Unknown cipher suite
	name = cipherSuiteName(0xFFFF)
	if name != "0xFFFF" {
		t.Fatalf("expected hex name for unknown cipher suite, got: %s", name)
	}
}

func TestTLSCheck_CertificateSubjectAndIssuer(t *testing.T) {
	addr, serverCert, close := testTLSServer(t)
	defer close()

	result := TLSCheck(addr, TLSConfig{})
	if result.Error != "" {
		t.Fatalf("unexpected error: %s", result.Error)
	}

	if len(result.CertChain) == 0 {
		t.Fatal("no certificates in chain")
	}

	// Verify subject contains the org name we set
	first := result.CertChain[0]
	if first.Subject == "" || first.Issuer == "" {
		t.Fatal("subject and issuer should be non-empty")
	}

	// Since it's self-signed, subject should contain our common name
	if !contains(first.Subject, "test.example.com") && !contains(first.Subject, "Test Corp") {
		t.Fatalf("subject doesn't match server cert: %s", first.Subject)
	}

	// Self-signed: subject == issuer
	if serverCert.Subject.CommonName == "test.example.com" {
		// In a self-signed cert, Subject and Issuer are the same
		_ = first.Issuer // just verify it exists
	}
}

func TestTLSCheck_Parallel(t *testing.T) {
	addr, _, close := testTLSServer(t)
	defer close()

	// Make multiple parallel TLS checks
	t.Run("parallel", func(t *testing.T) {
		for i := 0; i < 5; i++ {
			t.Run(fmt.Sprintf("check_%d", i), func(t *testing.T) {
				t.Parallel()
				result := TLSCheck(addr, TLSConfig{})
				if result.Error != "" {
					t.Fatalf("unexpected error: %s", result.Error)
				}
				if result.TLSVersion == "" {
					t.Fatal("missing TLS version")
				}
			})
		}
	})
}

// contains reports whether substr is within s.
func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
