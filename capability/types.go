package capability

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/z3r2ne/agentcore"
)

// Kind identifies a capability family. Custom kinds are allowed.
type Kind string

const (
	KindTool  Kind = "tool"
	KindSkill Kind = "skill"
	KindMCP   Kind = "mcp"
	KindPhone Kind = "phone"
	KindWeb   Kind = "web"
)

// Ref is the durable, declarative capability selected by Coordination.
// Config contains non-secret configuration or secret references, never raw
// credentials. Optional refs are ignored when no source is registered.
type Ref struct {
	Kind     Kind           `json:"kind"`
	Name     string         `json:"name"`
	Version  string         `json:"version,omitempty"`
	Config   map[string]any `json:"config,omitempty"`
	Optional bool           `json:"optional,omitempty"`
}

type Descriptor struct {
	Kind    Kind   `json:"kind"`
	Name    string `json:"name"`
	Dynamic bool   `json:"dynamic,omitempty"`
}

// ResolveContext contains execution-scoped identity and policy values. Values
// is intentionally adapter-owned; callers should use stable string keys.
type ResolveContext struct {
	ExecutionID string
	AgentID     string
	SessionID   string
	Workspace   string
	Values      map[string]any
}

// Instruction is trusted runtime-supplied context such as a resolved Skill.
type Instruction struct {
	Source  string `json:"source"`
	Content string `json:"content"`
}

// Snapshot records what was actually materialized for reproducibility.
type Snapshot struct {
	Kind     Kind              `json:"kind"`
	Name     string            `json:"name"`
	Version  string            `json:"version,omitempty"`
	Metadata map[string]string `json:"metadata,omitempty"`
}

// Resolved is one source's concrete contribution.
type Resolved struct {
	Tools        []agentcore.Tool
	Instructions []Instruction
	Closers      []io.Closer
	Snapshot     Snapshot
}

// Source materializes one capability ref for one execution.
type Source interface {
	Resolve(context.Context, ResolveContext, Ref) (Resolved, error)
}

// SourceFunc adapts a function into Source.
type SourceFunc func(context.Context, ResolveContext, Ref) (Resolved, error)

func (f SourceFunc) Resolve(ctx context.Context, execution ResolveContext, ref Ref) (Resolved, error) {
	return f(ctx, execution, ref)
}

// Bundle is the complete capability set for one Agent run.
type Bundle struct {
	Tools        []agentcore.Tool
	Instructions []Instruction
	Snapshots    []Snapshot
	closers      []io.Closer
}

// Close releases resources in reverse resolution order. Repeated calls are
// safe; only the first call reaches the underlying closers.
func (b *Bundle) Close() error {
	if b == nil {
		return nil
	}
	var result error
	for index := len(b.closers) - 1; index >= 0; index-- {
		if b.closers[index] != nil {
			result = errors.Join(result, b.closers[index].Close())
		}
	}
	b.closers = nil
	return result
}

func validateRef(ref Ref) error {
	if strings.TrimSpace(string(ref.Kind)) == "" {
		return errors.New("capability: kind is required")
	}
	if strings.TrimSpace(ref.Name) == "" {
		return errors.New("capability: name is required")
	}
	return nil
}

func sourceKey(kind Kind, name string) string {
	return string(kind) + "\x00" + strings.TrimSpace(name)
}

func validateResolved(ref Ref, resolved Resolved, toolNames map[string]Ref) error {
	for _, tool := range resolved.Tools {
		if tool == nil {
			return fmt.Errorf("capability: %s/%s resolved a nil tool", ref.Kind, ref.Name)
		}
		name := strings.TrimSpace(tool.Definition().Name)
		if name == "" {
			return fmt.Errorf("capability: %s/%s resolved an unnamed tool", ref.Kind, ref.Name)
		}
		if previous, exists := toolNames[name]; exists {
			return fmt.Errorf("capability: tool %q conflicts between %s/%s and %s/%s", name, previous.Kind, previous.Name, ref.Kind, ref.Name)
		}
		toolNames[name] = ref
	}
	return nil
}
