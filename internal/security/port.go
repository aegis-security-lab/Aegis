package security

import (
	"context"
	"fmt"
	"net"
	"time"
)

// PortResult holds the result of a TCP port check.
type PortResult struct {
	Host       string `json:"host"`
	Port       int    `json:"port"`
	Open       bool   `json:"open"`
	DurationMs int64  `json:"durationMs"`
	Error      string `json:"error,omitempty"`
}

// PortConfig allows injecting a custom dial function for testing.
// When DialContext is nil, net.Dialer is used.
type PortConfig struct {
	Timeout     time.Duration
	DialContext func(ctx context.Context, network, addr string) (net.Conn, error)
}

// PortCheck attempts a TCP connection to host:port without sending any payload.
// It only checks whether the port is open (i.e. TCP handshake succeeds).
func PortCheck(host string, port int, cfg PortConfig) PortResult {
	start := time.Now()
	addr := net.JoinHostPort(host, fmt.Sprint(port))

	if cfg.Timeout <= 0 {
		cfg.Timeout = 5 * time.Second
	}

	dial := cfg.DialContext
	if dial == nil {
		d := &net.Dialer{Timeout: cfg.Timeout}
		dial = d.DialContext
	}

	ctx, cancel := context.WithTimeout(context.Background(), cfg.Timeout)
	defer cancel()

	conn, err := dial(ctx, "tcp", addr)
	if err != nil {
		return PortResult{
			Host:       host,
			Port:       port,
			Open:       false,
			DurationMs: msSince(start),
			Error:      err.Error(),
		}
	}
	conn.Close()

	return PortResult{
		Host:       host,
		Port:       port,
		Open:       true,
		DurationMs: msSince(start),
	}
}
