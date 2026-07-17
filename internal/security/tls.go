package security

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"time"
)

// CertInfo holds a summary of one certificate in the chain.
type CertInfo struct {
	Subject   string   `json:"subject"`
	Issuer    string   `json:"issuer"`
	NotBefore string   `json:"notBefore"`
	NotAfter  string   `json:"notAfter"`
	DNSNames  []string `json:"dnsNames"`
}

// TLSResult holds the information collected from a TLS handshake.
type TLSResult struct {
	Target      string     `json:"target"`
	TLSVersion  string     `json:"tlsVersion"`
	CipherSuite string     `json:"cipherSuite"`
	CertChain   []CertInfo `json:"certChain"`
	Error       string     `json:"error,omitempty"`
}

// TLSConfig allows injecting a custom dial function for testing.
// When DialContext is nil, net.Dialer is used.
type TLSConfig struct {
	DialContext func(ctx context.Context, network, addr string) (net.Conn, error)
}

// tlsVersionName maps the tls.Version* constants to human-readable names.
func tlsVersionName(version uint16) string {
	switch version {
	case tls.VersionTLS10:
		return "TLS 1.0"
	case tls.VersionTLS11:
		return "TLS 1.1"
	case tls.VersionTLS12:
		return "TLS 1.2"
	case tls.VersionTLS13:
		return "TLS 1.3"
	default:
		return fmt.Sprintf("0x%04X", version)
	}
}

// cipherSuiteName returns the IANA name for a cipher suite.
func cipherSuiteName(id uint16) string {
	if name := tls.CipherSuiteName(id); name != "" {
		return name
	}
	return fmt.Sprintf("0x%04X", id)
}

// TLSCheck connects to hostport (e.g. "baidu.com:443"), performs a TLS handshake,
// and returns certificate chain, TLS version, and cipher suite information.
func TLSCheck(hostport string, cfg TLSConfig) TLSResult {
	dial := cfg.DialContext
	if dial == nil {
		d := &net.Dialer{Timeout: 10 * time.Second}
		dial = d.DialContext
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	rawConn, err := dial(ctx, "tcp", hostport)
	if err != nil {
		return TLSResult{Target: hostport, Error: err.Error()}
	}
	defer rawConn.Close()

	host, _, _ := net.SplitHostPort(hostport)
	tlsConn := tls.Client(rawConn, &tls.Config{ServerName: host, InsecureSkipVerify: true})
	if err := tlsConn.HandshakeContext(ctx); err != nil {
		return TLSResult{Target: hostport, Error: err.Error()}
	}

	state := tlsConn.ConnectionState()

	certs := make([]CertInfo, 0, len(state.PeerCertificates))
	for _, c := range state.PeerCertificates {
		certs = append(certs, CertInfo{
			Subject:   c.Subject.String(),
			Issuer:    c.Issuer.String(),
			NotBefore: c.NotBefore.Format(time.RFC3339),
			NotAfter:  c.NotAfter.Format(time.RFC3339),
			DNSNames:  c.DNSNames,
		})
	}

	return TLSResult{
		Target:      hostport,
		TLSVersion:  tlsVersionName(state.Version),
		CipherSuite: cipherSuiteName(state.CipherSuite),
		CertChain:   certs,
	}
}
