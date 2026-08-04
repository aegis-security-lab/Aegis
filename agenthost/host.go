package agenthost

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"unicode"

	"aegis/capability"
	"aegis/observability"
	"github.com/z3r2ne/agentcore"
)

// Host resolves an ExecutionSpec and runs it through agentcore. AgentDefaults
// carries runtime-owned retry, context, hook, and tool policies; Model, Tools,
// SystemPrompt, and ModelOptions are replaced by the resolved execution.
type Host struct {
	Models        ModelResolver
	Capabilities  *capability.Registry
	AgentDefaults agentcore.Config
	Configurer    AgentConfigurer
}

func New(models ModelResolver, capabilities *capability.Registry) (*Host, error) {
	if models == nil {
		return nil, errors.New("agenthost: model resolver is required")
	}
	if capabilities == nil {
		capabilities = capability.NewRegistry()
	}
	return &Host{Models: models, Capabilities: capabilities}, nil
}

// Run resolves all concrete resources for one execution and always closes the
// capability bundle before returning.
func (h *Host) Run(ctx context.Context, spec ExecutionSpec, sink agentcore.EventSink) (result Result, err error) {
	return h.RunState(ctx, spec, agentcore.State{}, []agentcore.Message{agentcore.TextMessage(agentcore.RoleUser, spec.Prompt)}, sink)
}

// RunState executes one prompt against an existing AgentCore state. It is
// intended for bounded continuations that must retain prior model/tool
// messages when a model stops before satisfying a higher-level contract.
func (h *Host) RunState(ctx context.Context, spec ExecutionSpec, state agentcore.State, messages []agentcore.Message, sink agentcore.EventSink) (result Result, err error) {
	ctx = withExecution(ctx, spec)
	ctx, span := (&observability.Tracer{Logger: observability.Default(), Metrics: observability.DefaultMetrics()}).Start(ctx, "agenthost.run", slog.String("provider", spec.Model.Provider), slog.String("model", spec.Model.Model), slog.Int("capability_count", len(spec.Capabilities)))
	defer func() {
		span.End(err, slog.String("stop_reason", string(result.Core.StopReason)), slog.Int("turns", result.Core.Turns))
	}()
	materialized, err := h.materialize(ctx, spec)
	if err != nil {
		return Result{}, err
	}
	defer func() {
		if closeErr := materialized.bundle.Close(); closeErr != nil {
			err = errors.Join(err, fmt.Errorf("agenthost: close capabilities: %w", closeErr))
		}
	}()
	coreResult, err := materialized.agent.Prompt(materialized.ctx, state, messages, sink)
	result = Result{Core: coreResult, Capabilities: append([]capability.Snapshot(nil), materialized.bundle.Snapshots...)}
	return result, err
}

type materializedAgent struct {
	ctx    context.Context
	agent  *agentcore.Agent
	bundle capability.Bundle
}

func (h *Host) materialize(ctx context.Context, spec ExecutionSpec) (materializedAgent, error) {
	if h == nil || h.Models == nil {
		return materializedAgent{}, errors.New("agenthost: nil host")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ValidateSpec(spec); err != nil {
		return materializedAgent{}, err
	}
	ctx = withExecution(ctx, spec)
	model, err := h.Models.ResolveModel(ctx, spec.Model)
	if err != nil {
		return materializedAgent{}, fmt.Errorf("agenthost: resolve model: %w", err)
	}
	if model == nil {
		return materializedAgent{}, errors.New("agenthost: model resolver returned nil")
	}
	bundle, err := h.Capabilities.Resolve(ctx, capability.ResolveContext{
		ExecutionID: spec.ExecutionID, AgentID: spec.AgentID, SessionID: spec.SessionID,
		Workspace: spec.Workspace, Values: cloneValues(spec.Values),
	}, spec.Capabilities)
	if err != nil {
		return materializedAgent{}, fmt.Errorf("agenthost: resolve capabilities: %w", err)
	}
	ctx = withAuthorizedTools(ctx, bundle.Tools)
	config := h.AgentDefaults
	config.Model = model
	config.Tools = append([]agentcore.Tool(nil), bundle.Tools...)
	config.SystemPrompt = composeSystemPrompt(spec.SystemPrompt, bundle.Instructions)
	config.ModelOptions = cloneValues(spec.Model.Options)
	config.Hooks = observabilityHooks(config.Hooks)
	if h.Configurer != nil {
		if err := h.Configurer.ConfigureAgent(ctx, spec, &config); err != nil {
			_ = bundle.Close()
			return materializedAgent{}, fmt.Errorf("agenthost: configure agent: %w", err)
		}
	}
	agent, err := agentcore.New(config)
	if err != nil {
		_ = bundle.Close()
		return materializedAgent{}, fmt.Errorf("agenthost: create agent: %w", err)
	}
	return materializedAgent{ctx: ctx, agent: agent, bundle: bundle}, nil
}

// ValidateSpec validates the durable execution contract before capabilities
// or provider resources are resolved.
func ValidateSpec(spec ExecutionSpec) error {
	if strings.TrimSpace(spec.ExecutionID) == "" {
		return errors.New("agenthost: execution ID is required")
	}
	if strings.TrimSpace(spec.AgentID) == "" {
		return errors.New("agenthost: agent ID is required")
	}
	if strings.TrimSpace(spec.Prompt) == "" {
		return errors.New("agenthost: prompt is required")
	}
	if strings.TrimSpace(spec.Model.Provider) == "" || strings.TrimSpace(spec.Model.Model) == "" {
		return errors.New("agenthost: model provider and name are required")
	}
	return nil
}

func composeSystemPrompt(base string, instructions []capability.Instruction) string {
	var result strings.Builder
	result.WriteString(strings.TrimSpace(base))
	for _, instruction := range instructions {
		content := strings.TrimSpace(instruction.Content)
		if content == "" {
			continue
		}
		if result.Len() > 0 {
			result.WriteString("\n\n")
		}
		result.WriteString("<capability_instruction source=\"")
		result.WriteString(safeSource(instruction.Source))
		result.WriteString("\">\n")
		result.WriteString(content)
		result.WriteString("\n</capability_instruction>")
	}
	return result.String()
}

func safeSource(source string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '-' || r == '_' || r == '.' || r == '/' {
			return r
		}
		return '_'
	}, strings.TrimSpace(source))
}

func cloneValues(values map[string]any) map[string]any {
	if values == nil {
		return nil
	}
	result := make(map[string]any, len(values))
	for key, value := range values {
		result[key] = value
	}
	return result
}
