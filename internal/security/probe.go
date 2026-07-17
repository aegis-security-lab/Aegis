package security

import (
	"crypto/tls"
	"io"
	"net/http"
	"strings"
	"time"
)

// ProbeConfig controls the HTTP probe behaviour.
// When Client is nil a default client is built from the other fields.
// FollowRedirect defaults to true when the pointer is nil.
type ProbeConfig struct {
	UserAgent      string
	Timeout        time.Duration
	FollowRedirect *bool // nil = follow redirects
	TLSConfig      *tls.Config
	// Client overrides every other setting. When set, the http.Client is used
	// as-is and fields above are ignored.
	Client *http.Client
}

// ProbeResult holds the response captured by HTTPProbe.
type ProbeResult struct {
	StatusCode  int                 `json:"statusCode"`
	Headers     map[string][]string `json:"headers"`
	BodyPreview string              `json:"bodyPreview"`
	DurationMs  int64               `json:"durationMs"`
	Error       string              `json:"error,omitempty"`
}

// HTTPProbe performs a GET request against target and returns a structured result.
// target must be a valid URL (scheme + host, e.g. "https://baidu.com").
func HTTPProbe(target string, cfg ProbeConfig) ProbeResult {
	start := time.Now()

	if cfg.Timeout <= 0 {
		cfg.Timeout = 10 * time.Second
	}
	if cfg.UserAgent == "" {
		cfg.UserAgent = "Aegis-Security-Probe/1.0"
	}

	followRedirect := true
	if cfg.FollowRedirect != nil {
		followRedirect = *cfg.FollowRedirect
	}

	client := cfg.Client
	if client == nil {
		tr := &http.Transport{
			TLSClientConfig: cfg.TLSConfig,
		}
		client = &http.Client{
			Timeout:   cfg.Timeout,
			Transport: tr,
		}
		if !followRedirect {
			client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
				return http.ErrUseLastResponse
			}
		}
	}

	req, err := http.NewRequest(http.MethodGet, target, nil)
	if err != nil {
		return ProbeResult{DurationMs: msSince(start), Error: err.Error()}
	}
	if cfg.UserAgent != "" {
		req.Header.Set("User-Agent", cfg.UserAgent)
	}

	resp, err := client.Do(req)
	if err != nil {
		return ProbeResult{DurationMs: msSince(start), Error: err.Error()}
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 2*1024)) // preview ≤ 2 KB
	preview := strings.TrimSpace(string(body))
	if len(preview) > 500 {
		preview = preview[:500] + "…"
	}

	return ProbeResult{
		StatusCode:  resp.StatusCode,
		Headers:     resp.Header,
		BodyPreview: preview,
		DurationMs:  msSince(start),
	}
}

func msSince(start time.Time) int64 {
	elapsed := time.Since(start).Milliseconds()
	if elapsed < 1 {
		return 1
	}
	return elapsed
}
