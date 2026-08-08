package control

import (
	"fmt"
	"strings"
)

// Audit levels describe exploit impact and preconditions independently from
// conventional risk severity.
const (
	AuditLevelS = "S" // Unconditional RCE with highest privileges.
	AuditLevelA = "A" // Unconditional RCE with limited privileges.
	AuditLevelB = "B" // Conditional RCE.
	AuditLevelC = "C" // Data access or a primitive enabling further exploitation.
	AuditLevelD = "D" // Availability or system impact, such as DoS.
	AuditLevelE = "E" // Limited information disclosure or low-impact weakness.
	AuditLevelI = "I" // Informational or unconfirmed risk.
)

var validAuditLevels = map[string]struct{}{
	AuditLevelS: {},
	AuditLevelA: {},
	AuditLevelB: {},
	AuditLevelC: {},
	AuditLevelD: {},
	AuditLevelE: {},
	AuditLevelI: {},
}

// NormalizeAuditLevel validates an explicit audit level. When omitted, the
// conventional severity supplies the canonical default.
func NormalizeAuditLevel(level, severity string) (string, error) {
	level = strings.ToUpper(strings.TrimSpace(level))
	if level == "" {
		return AuditLevelFromSeverity(severity), nil
	}
	if _, ok := validAuditLevels[level]; !ok {
		return "", fmt.Errorf("invalid audit level %q: expected S, A, B, C, D, E, or I", level)
	}
	return level, nil
}

// AuditLevelFromSeverity is intentionally conservative because severity alone
// cannot distinguish privilege, exploit preconditions, or impact type.
func AuditLevelFromSeverity(severity string) string {
	switch strings.ToLower(strings.TrimSpace(severity)) {
	case "critical":
		return AuditLevelA
	case "high":
		return AuditLevelB
	case "medium":
		return AuditLevelC
	case "low":
		return AuditLevelE
	default:
		return AuditLevelI
	}
}

// SeverityFromAuditLevel projects the audit level onto conventional risk
// severity for filters, scores, and integrations.
func SeverityFromAuditLevel(level string) string {
	switch strings.ToUpper(strings.TrimSpace(level)) {
	case AuditLevelS, AuditLevelA:
		return "critical"
	case AuditLevelB:
		return "high"
	case AuditLevelC:
		return "high"
	case AuditLevelD:
		return "medium"
	case AuditLevelE:
		return "low"
	default:
		return "info"
	}
}
