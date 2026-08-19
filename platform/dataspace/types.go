// Package dataspace defines application-owned persistent data namespaces and
// the execution-scoped grants used to expose them to Agents.
package dataspace

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

type Kind string

const (
	KindSQL      Kind = "sql"
	KindKeyValue Kind = "kv"
	KindDocument Kind = "document"
	KindBlob     Kind = "blob"
	KindVector   Kind = "vector"
)

type RetentionPolicy struct {
	Mode      string        `json:"mode,omitempty"`
	ExpiresIn time.Duration `json:"expiresIn,omitempty"`
}

// Descriptor declares one durable namespace owned by an Application. Schema
// and business semantics remain application-owned even when the platform
// supplies the physical database and migration lifecycle.
type Descriptor struct {
	AppID     string          `json:"appId"`
	Name      string          `json:"name"`
	Version   string          `json:"version"`
	Kinds     []Kind          `json:"kinds"`
	Retention RetentionPolicy `json:"retention,omitempty"`
}

func (d Descriptor) Validate() error {
	if strings.TrimSpace(d.AppID) == "" || strings.TrimSpace(d.Name) == "" || strings.TrimSpace(d.Version) == "" {
		return errors.New("dataspace: app ID, name and version are required")
	}
	if len(d.Kinds) == 0 {
		return errors.New("dataspace: at least one storage kind is required")
	}
	seen := map[Kind]bool{}
	for _, kind := range d.Kinds {
		switch kind {
		case KindSQL, KindKeyValue, KindDocument, KindBlob, KindVector:
		default:
			return fmt.Errorf("dataspace: unsupported storage kind %q", kind)
		}
		if seen[kind] {
			return fmt.Errorf("dataspace: duplicate storage kind %q", kind)
		}
		seen[kind] = true
	}
	return nil
}

type Ref struct {
	AppID    string `json:"appId"`
	Space    string `json:"space"`
	TenantID string `json:"tenantId,omitempty"`
	ScopeID  string `json:"scopeId,omitempty"`
}

// Grant is a snapshot of one subject's access to an application DataSpace.
// An empty expiry is allowed for application services; Agent executions should
// always receive a bounded expiry from admission policy.
type Grant struct {
	Ref
	SubjectID string    `json:"subjectId"`
	Actions   []string  `json:"actions"`
	ExpiresAt time.Time `json:"expiresAt,omitempty"`
}

func (g Grant) Validate() error {
	if strings.TrimSpace(g.AppID) == "" || strings.TrimSpace(g.Space) == "" || strings.TrimSpace(g.SubjectID) == "" {
		return errors.New("dataspace: grant app, space and subject are required")
	}
	if len(g.Actions) == 0 {
		return errors.New("dataspace: grant requires at least one action")
	}
	seen := map[string]bool{}
	for _, action := range g.Actions {
		action = strings.TrimSpace(action)
		if action == "" {
			return errors.New("dataspace: grant action cannot be empty")
		}
		if seen[action] {
			return fmt.Errorf("dataspace: duplicate grant action %q", action)
		}
		seen[action] = true
	}
	return nil
}

func Key(appID, name string) string {
	return strings.TrimSpace(appID) + "\x00" + strings.TrimSpace(name)
}
