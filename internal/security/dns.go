package security

import (
	"context"
	"net"
	"sort"
	"strings"
	"time"
)

// DNSRecordType represents a DNS record type.
type DNSRecordType string

const (
	RecordA     DNSRecordType = "A"
	RecordAAAA  DNSRecordType = "AAAA"
	RecordCNAME DNSRecordType = "CNAME"
	RecordMX    DNSRecordType = "MX"
)

// DNSRecord holds one resolved record.
type DNSRecord struct {
	Type     DNSRecordType `json:"type"`
	Value    string        `json:"value"`
	TTL      uint32        `json:"ttl,omitempty"`
	Priority int           `json:"priority,omitempty"` // only for MX
}

// DNSResult holds all resolved records for a domain.
type DNSResult struct {
	Domain  string      `json:"domain"`
	Records []DNSRecord `json:"records"`
	Error   string      `json:"error,omitempty"`
}

// DNSConfig allows injecting a custom resolver for testing.
// When Resolver is nil, net.DefaultResolver is used.
type DNSConfig struct {
	Resolver *net.Resolver
}

// DNSLookup performs DNS lookups for the given domain and requested record types.
// If types is empty, all four types (A, AAAA, CNAME, MX) are queried.
func DNSLookup(domain string, cfg DNSConfig, types ...DNSRecordType) DNSResult {
	if len(types) == 0 {
		types = []DNSRecordType{RecordA, RecordAAAA, RecordCNAME, RecordMX}
	}

	resolver := cfg.Resolver
	if resolver == nil {
		resolver = net.DefaultResolver
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var records []DNSRecord
	var errs []string

	typeMap := make(map[DNSRecordType]bool)
	for _, t := range types {
		typeMap[t] = true
	}

	// Sort for deterministic order
	sorted := make([]DNSRecordType, 0, len(types))
	for _, t := range types {
		sorted = append(sorted, t)
	}
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })

	for _, rt := range sorted {
		switch rt {
		case RecordA:
			ips, err := resolver.LookupIP(ctx, "ip4", domain)
			if err != nil {
				errs = append(errs, err.Error())
				continue
			}
			for _, ip := range ips {
				records = append(records, DNSRecord{Type: RecordA, Value: ip.String()})
			}

		case RecordAAAA:
			ips, err := resolver.LookupIP(ctx, "ip6", domain)
			if err != nil {
				errs = append(errs, err.Error())
				continue
			}
			for _, ip := range ips {
				records = append(records, DNSRecord{Type: RecordAAAA, Value: ip.String()})
			}

		case RecordCNAME:
			cname, err := resolver.LookupCNAME(ctx, domain)
			if err != nil {
				errs = append(errs, err.Error())
				continue
			}
			if cname != "" && !strings.EqualFold(cname, domain+".") {
				records = append(records, DNSRecord{Type: RecordCNAME, Value: strings.TrimSuffix(cname, ".")})
			}

		case RecordMX:
			mxs, err := resolver.LookupMX(ctx, domain)
			if err != nil {
				errs = append(errs, err.Error())
				continue
			}
			for _, mx := range mxs {
				records = append(records, DNSRecord{
					Type:     RecordMX,
					Value:    strings.TrimSuffix(mx.Host, "."),
					Priority: int(mx.Pref),
				})
			}
		}
	}

	result := DNSResult{Domain: domain, Records: records}
	if len(errs) > 0 {
		result.Error = strings.Join(errs, "; ")
	}
	return result
}
