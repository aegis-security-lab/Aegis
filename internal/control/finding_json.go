package control

import "encoding/json"

// UnmarshalJSON keeps old Finding payloads compatible while making auditLevel
// a validated, first-class field for API clients and AI-generated findings.
func (f *Finding) UnmarshalJSON(data []byte) error {
	type findingAlias Finding

	var decoded findingAlias
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}

	hasExplicitAuditLevel := decoded.AuditLevel != ""
	level, err := NormalizeAuditLevel(decoded.AuditLevel, decoded.Severity)
	if err != nil {
		return err
	}
	decoded.AuditLevel = level
	if hasExplicitAuditLevel {
		decoded.Severity = SeverityFromAuditLevel(level)
	}
	*f = Finding(decoded)
	return nil
}
