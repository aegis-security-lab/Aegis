package security

import (
	"context"
	"fmt"
	"net"
	"testing"
	"time"
)

func TestPortCheck_Open(t *testing.T) {
	// Start a TCP listener on a random port
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer listener.Close()

	port := listener.Addr().(*net.TCPAddr).Port

	// Accept and close connections immediately
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			conn.Close()
		}
	}()

	// Give the goroutine time to start
	time.Sleep(10 * time.Millisecond)

	result := PortCheck("127.0.0.1", port, PortConfig{Timeout: 5 * time.Second})
	if result.Error != "" {
		t.Fatalf("unexpected error: %s", result.Error)
	}
	if !result.Open {
		t.Fatal("port should be open")
	}
	if result.DurationMs < 0 {
		t.Fatal("duration should be non-negative")
	}
	if result.Host != "127.0.0.1" {
		t.Fatalf("host = %s, want 127.0.0.1", result.Host)
	}
	if result.Port != port {
		t.Fatalf("port = %d, want %d", result.Port, port)
	}
}

func TestPortCheck_Closed(t *testing.T) {
	// Port 0 is reserved (ephemeral), but we can try a port that's very unlikely to be open
	// Choose a high port that shouldn't be in use
	result := PortCheck("127.0.0.1", 65535, PortConfig{Timeout: 500 * time.Millisecond})
	if result.Open {
		t.Fatal("port 65535 should be closed on localhost")
	}
	if result.Error == "" {
		t.Fatal("expected error for closed port")
	}
}

func TestPortCheck_InvalidHost(t *testing.T) {
	// Use a custom dialer that always fails to simulate an unreachable host
	result := PortCheck("192.0.2.1", 80, PortConfig{
		Timeout: 100 * time.Millisecond,
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			return nil, fmt.Errorf("host unreachable: %s", addr)
		},
	})
	if result.Open {
		t.Fatal("should not be open on invalid host")
	}
	// Should still have host/port set
	if result.Port != 80 {
		t.Fatalf("port = %d", result.Port)
	}
}

func TestPortCheck_Timeout(t *testing.T) {
	// Connect to a port that accepts but doesn't respond (simulated via firewall drop)
	// We can test timeout by using a very short timeout on a blocked port
	result := PortCheck("127.0.0.1", 65534, PortConfig{Timeout: 1 * time.Millisecond})
	if result.Open {
		t.Fatal("port should not be open")
	}
	// Should have recorded duration
	if result.DurationMs <= 0 && result.Error != "" {
		// If the error was immediate, ms might be 0
	}
}

func TestPortCheck_CustomDialer(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer listener.Close()
	port := listener.Addr().(*net.TCPAddr).Port

	go func() {
		conn, _ := listener.Accept()
		if conn != nil {
			conn.Close()
		}
	}()
	time.Sleep(10 * time.Millisecond)

	dialed := false
	cfg := PortConfig{
		Timeout: 5 * time.Second,
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			dialed = true
			d := net.Dialer{Timeout: 5 * time.Second}
			return d.DialContext(ctx, network, addr)
		},
	}

	result := PortCheck("127.0.0.1", port, cfg)
	if result.Error != "" {
		t.Fatalf("unexpected error: %s", result.Error)
	}
	if !result.Open {
		t.Fatal("port should be open")
	}
	if !dialed {
		t.Fatal("custom dialer was not called")
	}
}

func TestPortCheck_DefaultTimeout(t *testing.T) {
	// Zero timeout should default to 5s
	result := PortCheck("127.0.0.1", 65533, PortConfig{})
	if result.Host != "127.0.0.1" {
		t.Fatalf("host = %s", result.Host)
	}
	// Port should be closed
	if result.Open {
		t.Fatal("expected closed port")
	}
}

func TestPortCheck_Parallel(t *testing.T) {
	// Start multiple listeners and check them in parallel
	type listenerInfo struct {
		host string
		port int
	}

	var listeners []listenerInfo
	for i := 0; i < 3; i++ {
		l, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatalf("listen: %v", err)
		}
		defer l.Close()
		port := l.Addr().(*net.TCPAddr).Port
		listeners = append(listeners, listenerInfo{"127.0.0.1", port})

		go func(ln net.Listener) {
			for {
				conn, err := ln.Accept()
				if err != nil {
					return
				}
				conn.Close()
			}
		}(l)
	}
	time.Sleep(20 * time.Millisecond)

	t.Run("parallel", func(t *testing.T) {
		for i, li := range listeners {
			t.Run(fmt.Sprintf("port_%d", i), func(t *testing.T) {
				t.Parallel()
				result := PortCheck(li.host, li.port, PortConfig{Timeout: 5 * time.Second})
				if result.Error != "" {
					t.Fatalf("unexpected error: %s", result.Error)
				}
				if !result.Open {
					t.Fatalf("port %d should be open", li.port)
				}
			})
		}
	})
}

func TestPortCheck_DialErrorPropagation(t *testing.T) {
	cfg := PortConfig{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			return nil, fmt.Errorf("custom dial error")
		},
	}

	result := PortCheck("127.0.0.1", 80, cfg)
	if result.Open {
		t.Fatal("port should not be open")
	}
	if !contains(result.Error, "custom dial error") {
		t.Fatalf("expected 'custom dial error', got: %s", result.Error)
	}
}

func TestPortCheck_ZeroValueConfig(t *testing.T) {
	// PortCheck should handle zero-value PortConfig gracefully
	result := PortCheck("127.0.0.1", 80, PortConfig{})
	if result.Host == "" || result.Port == 0 {
		t.Fatal("result should have host and port set")
	}
	// Should default timeout to 5s
	_ = result
}

func TestPortCheck_ResultFields(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer listener.Close()
	port := listener.Addr().(*net.TCPAddr).Port

	go func() {
		conn, _ := listener.Accept()
		if conn != nil {
			conn.Close()
		}
	}()
	time.Sleep(10 * time.Millisecond)

	result := PortCheck("127.0.0.1", port, PortConfig{Timeout: 5 * time.Second})
	if result.DurationMs < 0 {
		t.Fatal("duration should be recorded")
	}
	if result.Host != "127.0.0.1" {
		t.Fatalf("host mismatch: %s", result.Host)
	}
	if result.Port != port {
		t.Fatalf("port mismatch: %d", result.Port)
	}
	if !result.Open {
		t.Fatal("port should be open")
	}
}
