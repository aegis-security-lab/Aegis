package agenthost

import (
	"context"

	"aegis/capability"
	"github.com/z3r2ne/agentcore"
)

// ModelRef is a durable provider/model selection. Options must contain only
// non-secret values or references resolved by the ModelResolver.
type ModelRef struct {
	Provider string         `json:"provider"`
	Model    string         `json:"model"`
	Options  map[string]any `json:"options,omitempty"`
}

// ExecutionSpec is the declarative input supplied by Coordination.
type ExecutionSpec struct {
	ExecutionID string `json:"executionId"`
	AgentID     string `json:"agentId"`
	SessionID   string `json:"sessionId,omitempty"`
	Workspace   string `json:"workspace,omitempty"`

	Model        ModelRef         `json:"model"`
	SystemPrompt string           `json:"systemPrompt,omitempty"`
	Prompt       string           `json:"prompt"`
	Capabilities []capability.Ref `json:"capabilities,omitempty"`
	Values       map[string]any   `json:"values,omitempty"`

	// Runtime carries process-local execution extensions. It is deliberately
	// excluded from the durable contract and is typically an interceptor that
	// configures one materialized Agent.
	Runtime any `json:"-"`
}

// AgentConfigurer customizes one materialized AgentCore config after its
// model and capabilities have been resolved and before validation/New.
type AgentConfigurer interface {
	ConfigureAgent(context.Context, ExecutionSpec, *agentcore.Config) error
}

type AgentConfigurerFunc func(context.Context, ExecutionSpec, *agentcore.Config) error

func (f AgentConfigurerFunc) ConfigureAgent(ctx context.Context, spec ExecutionSpec, config *agentcore.Config) error {
	return f(ctx, spec, config)
}

// ModelResolver turns a durable ModelRef into a concrete streaming model.
type ModelResolver interface {
	ResolveModel(context.Context, ModelRef) (agentcore.Model, error)
}

// ModelResolverFunc adapts a function into ModelResolver.
type ModelResolverFunc func(context.Context, ModelRef) (agentcore.Model, error)

func (f ModelResolverFunc) ResolveModel(ctx context.Context, ref ModelRef) (agentcore.Model, error) {
	return f(ctx, ref)
}

// Result combines core output with the exact resolved capability snapshot.
type Result struct {
	Core         agentcore.Result      `json:"core"`
	Capabilities []capability.Snapshot `json:"capabilities,omitempty"`
}

// Runner is the Coordination-facing Agent execution boundary.
type Runner interface {
	Run(context.Context, ExecutionSpec, agentcore.EventSink) (Result, error)
}
