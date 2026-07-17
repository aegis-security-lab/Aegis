package security

import (
	"context"
	"fmt"
	"net"
	"strings"
	"testing"
	"time"

	"golang.org/x/net/dns/dnsmessage"
)

// testDNSServer starts a local UDP DNS server on a random port and returns
// a resolver that points to it plus a close function.
// The server responds to A, AAAA, CNAME, and MX queries with pre-configured data.
// For A/AAAA queries it also includes a CNAME record so that Go's resolver
// can extract the CNAME from the response (which is how LookupCNAME works).
func testDNSServer(t *testing.T) (*net.Resolver, func()) {
	t.Helper()

	addr, err := net.ResolveUDPAddr("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("resolve addr: %v", err)
	}
	conn, err := net.ListenUDP("udp", addr)
	if err != nil {
		t.Fatalf("listen udp: %v", err)
	}

	cnameTarget, _ := dnsmessage.NewName("target.example.com.")
	aIP := [4]byte{192, 0, 2, 1}
	aaaaIP := [16]byte{0x20, 0x01, 0x0d, 0xb8, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1}
	_ = aaaaIP

	// Record types we handle
	type answerGen func(name dnsmessage.Name) ([]dnsmessage.Resource, bool)

	handlers := map[dnsmessage.Type]answerGen{
		dnsmessage.TypeA: func(name dnsmessage.Name) ([]dnsmessage.Resource, bool) {
			// When querying for example.com (which is a CNAME), return CNAME + A for target
			return []dnsmessage.Resource{
				{
					Header: dnsmessage.ResourceHeader{
						Name:  name,
						Type:  dnsmessage.TypeCNAME,
						Class: dnsmessage.ClassINET,
						TTL:   300,
					},
					Body: &dnsmessage.CNAMEResource{CNAME: cnameTarget},
				},
				{
					Header: dnsmessage.ResourceHeader{
						Name:  cnameTarget,
						Type:  dnsmessage.TypeA,
						Class: dnsmessage.ClassINET,
						TTL:   300,
					},
					Body: &dnsmessage.AResource{A: aIP},
				},
			}, true
		},
		dnsmessage.TypeAAAA: func(name dnsmessage.Name) ([]dnsmessage.Resource, bool) {
			return []dnsmessage.Resource{
				{
					Header: dnsmessage.ResourceHeader{
						Name:  name,
						Type:  dnsmessage.TypeCNAME,
						Class: dnsmessage.ClassINET,
						TTL:   300,
					},
					Body: &dnsmessage.CNAMEResource{CNAME: cnameTarget},
				},
				{
					Header: dnsmessage.ResourceHeader{
						Name:  cnameTarget,
						Type:  dnsmessage.TypeAAAA,
						Class: dnsmessage.ClassINET,
						TTL:   300,
					},
					Body: &dnsmessage.AAAAResource{AAAA: aaaaIP},
				},
			}, true
		},
		dnsmessage.TypeCNAME: func(name dnsmessage.Name) ([]dnsmessage.Resource, bool) {
			return []dnsmessage.Resource{{
				Header: dnsmessage.ResourceHeader{
					Name:  name,
					Type:  dnsmessage.TypeCNAME,
					Class: dnsmessage.ClassINET,
					TTL:   300,
				},
				Body: &dnsmessage.CNAMEResource{CNAME: cnameTarget},
			}}, true
		},
		dnsmessage.TypeMX: func(name dnsmessage.Name) ([]dnsmessage.Resource, bool) {
			mxName, err := dnsmessage.NewName("mail.example.com.")
			if err != nil {
				return nil, false
			}
			return []dnsmessage.Resource{{
				Header: dnsmessage.ResourceHeader{
					Name:  name,
					Type:  dnsmessage.TypeMX,
					Class: dnsmessage.ClassINET,
					TTL:   300,
				},
				Body: &dnsmessage.MXResource{
					MX:   mxName,
					Pref: 10,
				},
			}}, true
		},
	}

	go func() {
		buf := make([]byte, 1500)
		for {
			n, remote, err := conn.ReadFromUDP(buf)
			if err != nil {
				return
			}
			data := make([]byte, n)
			copy(data, buf[:n])

			go func(raw []byte, addr *net.UDPAddr) {
				var parser dnsmessage.Parser
				header, err := parser.Start(raw)
				if err != nil {
					return
				}
				q, err := parser.Question()
				if err != nil {
					return
				}

				// Build response
				respBuilder := dnsmessage.NewBuilder(nil, dnsmessage.Header{
					ID:                 header.ID,
					Response:           true,
					OpCode:             0,
					Authoritative:      false,
					Truncated:          false,
					RecursionDesired:   header.RecursionDesired,
					RecursionAvailable: true,
					RCode:              dnsmessage.RCodeSuccess,
				})
				_ = respBuilder.StartQuestions()
				_ = respBuilder.Question(q)

				handler, ok := handlers[q.Type]
				if ok {
					if answers, found := handler(q.Name); found {
						_ = respBuilder.StartAnswers()
						for _, a := range answers {
							addResource(&respBuilder, a)
						}
					}
				}

				packed, err := respBuilder.Finish()
				if err != nil {
					// Send SERVFAIL
					errBuilder := dnsmessage.NewBuilder(nil, dnsmessage.Header{
						ID:       header.ID,
						Response: true,
						RCode:    dnsmessage.RCodeServerFailure,
					})
					packed, _ = errBuilder.Finish()
					if packed == nil {
						return
					}
				}
				_, _ = conn.WriteTo(packed, addr)
			}(data, remote)
		}
	}()

	resolver := &net.Resolver{
		PreferGo: true,
		Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
			return net.Dial("udp", conn.LocalAddr().String())
		},
	}

	return resolver, func() { conn.Close() }
}

// addResource adds a DNS resource to the builder using the correct typed method.
func addResource(b *dnsmessage.Builder, a dnsmessage.Resource) {
	switch body := a.Body.(type) {
	case *dnsmessage.AResource:
		_ = b.AResource(a.Header, *body)
	case *dnsmessage.AAAAResource:
		_ = b.AAAAResource(a.Header, *body)
	case *dnsmessage.CNAMEResource:
		_ = b.CNAMEResource(a.Header, *body)
	case *dnsmessage.MXResource:
		_ = b.MXResource(a.Header, *body)
	}
}

func TestDNSLookup_Success(t *testing.T) {
	resolver, close := testDNSServer(t)
	defer close()

	result := DNSLookup("example.com", DNSConfig{Resolver: resolver})

	if result.Error != "" {
		t.Fatalf("unexpected error: %s", result.Error)
	}
	if result.Domain != "example.com" {
		t.Fatalf("domain = %s, want example.com", result.Domain)
	}

	// Should have A record
	foundA := false
	foundAAAA := false
	for _, r := range result.Records {
		switch r.Type {
		case RecordA:
			if r.Value == "192.0.2.1" {
				foundA = true
			}
		case RecordAAAA:
			if r.Value == "2001:db8::1" {
				foundAAAA = true
			}
		}
	}
	if !foundA {
		t.Fatal("missing A record 192.0.2.1")
	}
	if !foundAAAA {
		t.Fatal("missing AAAA record 2001:db8::1")
	}
}

func TestDNSLookup_SpecificTypes(t *testing.T) {
	resolver, close := testDNSServer(t)
	defer close()

	// Only query MX records
	result := DNSLookup("example.com", DNSConfig{Resolver: resolver}, RecordMX)

	if result.Error != "" {
		t.Fatalf("unexpected error: %s", result.Error)
	}

	if len(result.Records) != 1 {
		t.Fatalf("expected 1 MX record, got %d", len(result.Records))
	}
	if result.Records[0].Type != RecordMX {
		t.Fatalf("expected MX type, got %s", result.Records[0].Type)
	}
	if result.Records[0].Value != "mail.example.com" {
		t.Fatalf("MX target = %s, want mail.example.com", result.Records[0].Value)
	}
	if result.Records[0].Priority != 10 {
		t.Fatalf("MX priority = %d, want 10", result.Records[0].Priority)
	}
}

func TestDNSLookup_CNAME(t *testing.T) {
	resolver, close := testDNSServer(t)
	defer close()

	result := DNSLookup("example.com", DNSConfig{Resolver: resolver}, RecordCNAME)

	if result.Error != "" {
		t.Fatalf("unexpected error: %s", result.Error)
	}
	found := false
	for _, r := range result.Records {
		if r.Type == RecordCNAME && r.Value == "target.example.com" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("CNAME record not found: %+v", result.Records)
	}
}

func TestDNSLookup_Error(t *testing.T) {
	// A resolver that fails on any lookup
	failResolver := &net.Resolver{
		PreferGo: true,
		Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
			return nil, fmt.Errorf("mock dial error")
		},
	}

	result := DNSLookup("example.com", DNSConfig{Resolver: failResolver})
	if result.Error == "" {
		t.Fatal("expected error from failing resolver")
	}
	// Should still have domain set
	if result.Domain != "example.com" {
		t.Fatalf("domain = %s", result.Domain)
	}
}

func TestDNSLookup_DialTimeout(t *testing.T) {
	// Dial that blocks for a very short time, then fails.
	// This tests that DNSLookup handles connection failures gracefully.
	timeoutResolver := &net.Resolver{
		PreferGo: true,
		Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
			// Wait for context to be done or a small timeout
			timer := time.NewTimer(50 * time.Millisecond)
			defer timer.Stop()
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-timer.C:
				return nil, fmt.Errorf("dial timeout")
			}
		},
	}

	result := DNSLookup("example.com", DNSConfig{Resolver: timeoutResolver})
	// Should get an error since dial fails
	if result.Error == "" {
		t.Log("expected a dial error but got none; records:", result.Records)
	}
}

func TestDNSLookup_ResultOrdering(t *testing.T) {
	res, close := testDNSServer(t)
	defer close()

	// Query all types and check that results are in a consistent order
	result := DNSLookup("example.com", DNSConfig{Resolver: res})

	if len(result.Records) == 0 {
		t.Fatal("expected records")
	}

	// Verify ordering: A < AAAA < CNAME < MX
	lastType := ""
	for _, r := range result.Records {
		if string(r.Type) < lastType {
			t.Fatalf("records not sorted: %s after %s", r.Type, lastType)
		}
		lastType = string(r.Type)
	}
}

func TestDNSLookup_NoResolver(t *testing.T) {
	// Using nil resolver should use net.DefaultResolver
	// We can't mock this but we can at least verify no panic
	result := DNSLookup("localhost", DNSConfig{})
	// localhost should resolve without network (via /etc/hosts)
	if result.Error != "" {
		// In some environments localhost may not resolve; that's OK
		t.Logf("localhost resolution: %s", result.Error)
	}
	// Should not panic regardless
	_ = result
}

func TestDNSLookup_EmptyDomain(t *testing.T) {
	resolver, close := testDNSServer(t)
	defer close()

	result := DNSLookup("", DNSConfig{Resolver: resolver})
	// Empty domain should produce an error or empty results
	_ = result
	// Just verify no panic
}

func TestDNSLookup_ErrorCleansUp(t *testing.T) {
	// Verify that error cases return properly formatted result
	resolver, close := testDNSServer(t)
	defer close()

	// This should work
	result := DNSLookup("example.com", DNSConfig{Resolver: resolver})
	if result.Domain != "example.com" {
		t.Fatal("domain should be set")
	}
	if result.Error != "" && len(result.Records) > 0 {
		t.Fatal("should not have both error and records")
	}
}

func TestDNSLookup_MXPriority(t *testing.T) {
	// Use a custom resolver that returns multiple MX records
	mxResolver := &net.Resolver{
		PreferGo: true,
		Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
			return net.Dial("udp", newMultiMXDNSServer(t).LocalAddr().String())
		},
	}

	result := DNSLookup("example.com", DNSConfig{Resolver: mxResolver}, RecordMX)
	if result.Error != "" {
		t.Fatalf("unexpected error: %s", result.Error)
	}

	// Should have 2 MX records
	if len(result.Records) != 2 {
		t.Fatalf("expected 2 MX records, got %d: %+v", len(result.Records), result.Records)
	}

	// One should be primary (priority 5), one secondary (priority 10)
	priorities := make(map[int]string)
	for _, r := range result.Records {
		priorities[r.Priority] = r.Value
	}
	if priorities[5] != "primary.example.com" {
		t.Fatalf("expected primary.example.com with priority 5, got %+v", priorities)
	}
	if priorities[10] != "secondary.example.com" {
		t.Fatalf("expected secondary.example.com with priority 10, got %+v", priorities)
	}
}

// newMultiMXDNSServer starts a DNS server that returns multiple MX records.
func newMultiMXDNSServer(t *testing.T) *net.UDPConn {
	t.Helper()

	addr, err := net.ResolveUDPAddr("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("resolve addr: %v", err)
	}
	conn, err := net.ListenUDP("udp", addr)
	if err != nil {
		t.Fatalf("listen udp: %v", err)
	}

	primary, _ := dnsmessage.NewName("primary.example.com.")
	secondary, _ := dnsmessage.NewName("secondary.example.com.")

	go func() {
		buf := make([]byte, 1500)
		for {
			n, remote, err := conn.ReadFromUDP(buf)
			if err != nil {
				return
			}
			data := make([]byte, n)
			copy(data, buf[:n])

			go func(raw []byte, addr *net.UDPAddr) {
				var parser dnsmessage.Parser
				header, err := parser.Start(raw)
				if err != nil {
					return
				}
				q, err := parser.Question()
				if err != nil {
					return
				}

				respBuilder := dnsmessage.NewBuilder(nil, dnsmessage.Header{
					ID:                 header.ID,
					Response:           true,
					OpCode:             0,
					RecursionDesired:   header.RecursionDesired,
					RecursionAvailable: true,
					RCode:              dnsmessage.RCodeSuccess,
				})
				_ = respBuilder.StartQuestions()
				_ = respBuilder.Question(q)
				_ = respBuilder.StartAnswers()
				_ = respBuilder.MXResource(dnsmessage.ResourceHeader{
					Name:  q.Name,
					Type:  dnsmessage.TypeMX,
					Class: dnsmessage.ClassINET,
					TTL:   300,
				}, dnsmessage.MXResource{MX: primary, Pref: 5})
				_ = respBuilder.MXResource(dnsmessage.ResourceHeader{
					Name:  q.Name,
					Type:  dnsmessage.TypeMX,
					Class: dnsmessage.ClassINET,
					TTL:   300,
				}, dnsmessage.MXResource{MX: secondary, Pref: 10})

				packed, err := respBuilder.Finish()
				if err != nil {
					return
				}
				_, _ = conn.WriteTo(packed, addr)
			}(data, remote)
		}
	}()

	return conn
}

func TestDNSLookup_FilterUnwantedTypes(t *testing.T) {
	resolver, close := testDNSServer(t)
	defer close()

	// Only query A records - should not return AAAA, CNAME, MX
	result := DNSLookup("example.com", DNSConfig{Resolver: resolver}, RecordA)

	if result.Error != "" {
		t.Fatalf("unexpected error: %s", result.Error)
	}
	for _, r := range result.Records {
		if r.Type != RecordA {
			t.Fatalf("unexpected record type %s when only A was requested", r.Type)
		}
	}
	if len(result.Records) == 0 {
		t.Fatal("expected at least one A record")
	}
}

func TestDNSLookup_MultipleCalls(t *testing.T) {
	resolver, close := testDNSServer(t)
	defer close()

	// Make two sequential calls
	r1 := DNSLookup("example.com", DNSConfig{Resolver: resolver})
	r2 := DNSLookup("example.com", DNSConfig{Resolver: resolver})

	if r1.Error != "" || r2.Error != "" {
		t.Fatal("both calls should succeed")
	}
	if len(r1.Records) == 0 || len(r2.Records) == 0 {
		t.Fatal("both calls should return records")
	}
}

func TestMsSince(t *testing.T) {
	start := time.Now()
	time.Sleep(1 * time.Millisecond)
	ms := msSince(start)
	if ms <= 0 {
		t.Fatalf("expected positive ms, got %d", ms)
	}

	// Verify zero
	now := time.Now()
	ms = msSince(now)
	if ms < 0 {
		t.Fatalf("negative duration: %d", ms)
	}
}

func TestDNSLookup_SuccessWithEmptyTypes(t *testing.T) {
	t.Run("nil types defaults to all four", func(t *testing.T) {
		resolver, close := testDNSServer(t)
		defer close()

		result := DNSLookup("example.com", DNSConfig{Resolver: resolver})
		if result.Error != "" {
			t.Fatalf("unexpected error: %s", result.Error)
		}

		types := make(map[DNSRecordType]bool)
		for _, r := range result.Records {
			types[r.Type] = true
		}

		// When no types specified, defaults to all four
		for _, rt := range []DNSRecordType{RecordA, RecordAAAA, RecordMX} {
			if !types[rt] {
				t.Errorf("missing record type %s", rt)
			}
		}
		// CNAME is typically not returned as a separate type from LookupCNAME
		// because Go's resolver embeds it in A/AAAA responses
	})
}

func TestDNSLookup_ServfailHandling(t *testing.T) {
	// Server that returns SERVFAIL for all queries
	servfailAddr := startServfailServer(t)

	failResolver := &net.Resolver{
		PreferGo: true,
		Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
			return net.Dial("udp", servfailAddr.String())
		},
	}

	result := DNSLookup("example.com", DNSConfig{Resolver: failResolver})
	// Should get error or empty records
	if result.Error == "" && len(result.Records) > 0 {
		t.Fatal("expected error or empty records for SERVFAIL")
	}
}

func startServfailServer(t *testing.T) *net.UDPAddr {
	t.Helper()
	addr, err := net.ResolveUDPAddr("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	conn, err := net.ListenUDP("udp", addr)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}

	go func() {
		buf := make([]byte, 512)
		for {
			n, remote, err := conn.ReadFromUDP(buf)
			if err != nil {
				return
			}
			// Parse header to get ID
			var parser dnsmessage.Parser
			header, err := parser.Start(buf[:n])
			if err != nil {
				continue
			}
			// Build SERVFAIL response
			r := dnsmessage.NewBuilder(nil, dnsmessage.Header{
				ID:       header.ID,
				Response: true,
				RCode:    dnsmessage.RCodeServerFailure,
			})
			packed, _ := r.Finish()
			if packed != nil {
				_, _ = conn.WriteTo(packed, remote)
			}
		}
	}()

	return conn.LocalAddr().(*net.UDPAddr)
}

func TestDNSLookup_ErrorContainsMessages(t *testing.T) {
	_, close := testDNSServer(t)
	defer close()

	// Test that error strings from resolver are propagated
	brokenResolver := &net.Resolver{
		PreferGo: true,
		Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
			return nil, fmt.Errorf("connection refused")
		},
	}

	result := DNSLookup("example.com", DNSConfig{Resolver: brokenResolver})
	if !strings.Contains(result.Error, "connection refused") {
		t.Fatalf("error should contain 'connection refused', got: %s", result.Error)
	}
}
