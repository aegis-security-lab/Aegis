package agenthost

import (
	"context"

	"aegis/observability"
	"github.com/z3r2ne/agentcore"
)

type executionContextKey struct{}
type authorizedToolsContextKey struct{}

// ExecutionContext is the non-secret identity available to policy, tracing,
// model and tool adapters during one Host run.
type ExecutionContext struct {
	ExecutionID string
	AgentID     string
	SessionID   string
	Workspace   string
}

func ExecutionFromContext(ctx context.Context) (ExecutionContext, bool) {
	if ctx == nil {
		return ExecutionContext{}, false
	}
	execution, ok := ctx.Value(executionContextKey{}).(ExecutionContext)
	return execution, ok
}

// ToolAuthorizedFromContext reports whether the active AgentHost materialized
// a tool from its policy-approved capability bundle for this execution.
func ToolAuthorizedFromContext(ctx context.Context, name string) bool {
	if ctx == nil {
		return false
	}
	tools, _ := ctx.Value(authorizedToolsContextKey{}).(map[string]bool)
	return tools[name]
}

func withAuthorizedTools(ctx context.Context, tools []agentcore.Tool) context.Context {
	allowed := make(map[string]bool, len(tools))
	for _, tool := range tools {
		if tool != nil {
			allowed[tool.Definition().Name] = true
		}
	}
	return context.WithValue(ctx, authorizedToolsContextKey{}, allowed)
}

func withExecution(ctx context.Context, spec ExecutionSpec) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	ctx = observability.WithScope(ctx, observability.Scope{ExecutionID: spec.ExecutionID, AgentID: spec.AgentID, Component: "agenthost"})
	return context.WithValue(ctx, executionContextKey{}, ExecutionContext{
		ExecutionID: spec.ExecutionID, AgentID: spec.AgentID, SessionID: spec.SessionID, Workspace: spec.Workspace,
	})
}
