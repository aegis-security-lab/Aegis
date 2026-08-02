package control

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"time"

	"aegis/agenthost"
	"aegis/coordination"
	"github.com/z3r2ne/agentcore"
)

const coordinationCorrelationValue = "coordination.correlationId"

// CoordinationRunner routes Coordination-owned executions without coupling
// AgentHost to Aegis Issue semantics.
type CoordinationRunner struct {
	Issues    agenthost.Runner
	Subagents agenthost.Runner
}

func (r CoordinationRunner) Run(ctx context.Context, spec agenthost.ExecutionSpec, sink agentcore.EventSink) (agenthost.Result, error) {
	if stringSpecValue(spec, coordinationCorrelationValue) != "" {
		if r.Subagents == nil {
			return agenthost.Result{}, errors.New("control coordination: native subagent runner is unavailable")
		}
		return r.Subagents.Run(ctx, spec, sink)
	}
	if r.Issues == nil {
		return agenthost.Result{}, errors.New("control coordination: Issue runner is unavailable")
	}
	return r.Issues.Run(ctx, spec, sink)
}

// NativeCoordinationRuntime is both the StartSubagent effect adapter and the
// Agent executor for that work. A direct child is a durable Coordination execution,
// not a hidden Board Issue.
type NativeCoordinationRuntime struct {
	Manager      *Manager
	Host         agenthost.Runner
	Bridge       *CoordinationBridge
	PhoneEnabled bool
	Planner      *coordination.CapabilityPlanner
}

// ManagerBoardCoordinator connects Phone Board operations to the durable
// Coordination control plane. It is not an Agent tool surface.
type ManagerBoardCoordinator struct{ Manager *Manager }

func (c ManagerBoardCoordinator) Delegate(ctx context.Context, invocation coordination.Invocation, request coordination.DelegationRequest) error {
	if c.Manager == nil || c.Manager.Coordination() == nil {
		return errors.New("control coordination: runtime is disabled")
	}
	return c.Manager.Coordination().SubmitDelegation(ctx, invocation.EventID, invocation.IssueID, invocation.ExecutionID, invocation.AgentID, request)
}

func (c ManagerBoardCoordinator) Continue(ctx context.Context, invocation coordination.Invocation, request coordination.ContinueRequest) error {
	if c.Manager == nil || c.Manager.Coordination() == nil {
		return errors.New("control coordination: runtime is disabled")
	}
	return c.Manager.Coordination().ContinueTerminalChildIssue(ctx, invocation, request)
}

func (c ManagerBoardCoordinator) Wait(ctx context.Context, invocation coordination.Invocation, request coordination.WaitRequest) error {
	if c.Manager == nil || c.Manager.Coordination() == nil {
		return errors.New("control coordination: runtime is disabled")
	}
	if err := c.Manager.Coordination().SubmitWait(ctx, invocation.EventID, invocation.IssueID, invocation.ExecutionID, invocation.AgentID, request); err != nil {
		return err
	}
	result := c.Manager.store.db.Model(&Issue{}).
		Where("id = ? AND assignee_agent_id = ? AND status = ? AND current_execution_id = ?", invocation.IssueID, invocation.AgentID, "in_progress", invocation.ExecutionID).
		Updates(map[string]any{"execution_phase": "sleeping", "sleep_token": invocation.EventID, "checkout_execution_id": "", "updated_at": time.Now()})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return errors.New("control coordination: Issue cannot enter sleep")
	}
	c.Manager.store.notify()
	return nil
}

func (r *NativeCoordinationRuntime) StartCoordinationSubagent(ctx context.Context, command coordination.StartSubagentCommand) error {
	if r == nil || r.Manager == nil || r.Manager.store == nil || r.Host == nil || r.Bridge == nil {
		return errors.New("control coordination: native subagent runtime is incomplete")
	}
	if strings.TrimSpace(command.CorrelationID) == "" || strings.TrimSpace(command.CoordinationID) == "" {
		return errors.New("control coordination: direct child requires coordination and correlation IDs")
	}
	agent, err := r.Manager.store.GetAgent(command.Child.AgentID)
	if err != nil {
		return err
	}
	if !agent.Enabled || agent.Internal {
		return errors.New("control coordination: direct child Agent is unavailable")
	}
	cfg := r.Manager.store.effectiveAgentConfig(agent)
	if !cfg.Configured {
		return errors.New("Aegis 尚未配置")
	}
	workspace := strings.TrimSpace(command.Child.Workspace)
	if workspace == "" && command.ParentIssueID != "" {
		if parent, loadErr := r.Manager.store.GetIssue(command.ParentIssueID); loadErr == nil {
			workspace = parent.Workspace
		}
	}
	if workspace == "" {
		workspace = cfg.Workspace
	}
	bases, err := r.Manager.store.knowledgeBasesByIDs(agent.KnowledgeBaseIDs)
	if err != nil {
		return err
	}
	systemPrompt := agentKnowledgeSystemPrompt(agent.SystemPrompt, bases)
	systemPrompt = agentSecurityOutcomeGradeSystemPrompt(systemPrompt, agent.Category)
	systemPrompt = agentLanguageSystemPrompt(systemPrompt, cfg.Language)
	systemPrompt = agentPermissionSystemPrompt(systemPrompt, workspace, agent.Permissions)
	systemPrompt = strings.TrimSpace(systemPrompt + "\n\nYou are a direct child Agent. Complete the delegated prompt independently and return a concise, evidence-backed result to the parent Agent. You do not own a Board Issue unless the prompt explicitly asks you to create one.")
	var decision coordination.CapabilityDecision
	if r.Bridge != nil {
		decision, err = r.Bridge.PlanAgentCapabilities(ctx, command.CoordinationID, agent.ID, command.Child.Capabilities, command.Child.CapabilitySelection)
	} else {
		defaults := defaultNativeCapabilities(agent, cfg, r.PhoneEnabled)
		decision, err = planExecutionCapabilities(ctx, r.Manager, r.Planner, command.CoordinationID, defaults, command.Child.Capabilities, nil, command.Child.CapabilitySelection)
	}
	if err != nil {
		return err
	}
	capabilities := decision.Capabilities
	options := map[string]any{}
	if thinking := strings.TrimSpace(cfg.Thinking); thinking != "" && thinking != "off" {
		options["reasoning_effort"] = thinking
	}
	executionID := directExecutionID(command.CorrelationID)
	spec := agenthost.ExecutionSpec{
		ExecutionID: executionID, AgentID: agent.ID, SessionID: executionID, Workspace: workspace,
		Model:        agenthost.ModelRef{Provider: cfg.Provider, Model: cfg.Model, Options: options},
		SystemPrompt: systemPrompt, Prompt: command.Child.Prompt, Capabilities: capabilities,
		Values: map[string]any{
			coordinationCorrelationValue:      command.CorrelationID,
			"control.issueId":                 command.ParentIssueID,
			"coordination.coordinationId":     command.CoordinationID,
			"coordination.parentIssueId":      command.ParentIssueID,
			"coordination.parentExecutionId":  command.ParentExecutionID,
			"coordination.parentAgentId":      command.ParentAgentID,
			"coordination.parentBehavior":     command.ParentBehavior,
			"coordination.resultDelivery":     command.ResultDelivery,
			"coordination.capabilityDecision": decision,
		},
	}
	return r.Bridge.EnqueueExecution(ctx, coordination.Execution{ID: executionID, CoordinationID: command.CoordinationID, MaxAttempts: 3, Spec: spec})
}

func (r *NativeCoordinationRuntime) Run(ctx context.Context, spec agenthost.ExecutionSpec, sink agentcore.EventSink) (agenthost.Result, error) {
	if r == nil || r.Host == nil || r.Bridge == nil {
		return agenthost.Result{}, errors.New("control coordination: native completion bridge is unavailable")
	}
	result, runErr := r.Host.Run(ctx, spec, sink)
	correlationID := stringSpecValue(spec, coordinationCorrelationValue)
	command := coordination.StartSubagentCommand{
		CoordinationID: stringSpecValue(spec, "coordination.coordinationId"),
		ParentIssueID:  stringSpecValue(spec, "coordination.parentIssueId"), ParentExecutionID: stringSpecValue(spec, "coordination.parentExecutionId"),
		ParentAgentID: stringSpecValue(spec, "coordination.parentAgentId"), CorrelationID: correlationID,
		ParentBehavior: stringSpecValue(spec, "coordination.parentBehavior"), ResultDelivery: stringSpecValue(spec, "coordination.resultDelivery"),
	}
	completed := coordination.Completion{Result: lastAssistantText(result.Core.State.Messages), Success: runErr == nil, ChildID: spec.ExecutionID, ParentBehavior: command.ParentBehavior, ResultDelivery: command.ResultDelivery}
	if runErr != nil {
		completed.Error = runErr.Error()
	}
	submitErr := r.Bridge.SubmitExecutionCompleted(context.Background(), "direct-completed:"+correlationID, command, spec.ExecutionID, spec.AgentID, completed)
	if submitErr != nil {
		return result, errors.Join(runErr, submitErr)
	}
	// A failed child is a successfully delivered coordination outcome. The
	// parent decides whether to retry, replace, or integrate the partial result.
	return result, nil
}

func directExecutionID(correlationID string) string {
	digest := sha256.Sum256([]byte(correlationID))
	return "direct-" + hex.EncodeToString(digest[:12])
}

func stringSpecValue(spec agenthost.ExecutionSpec, key string) string {
	value, _ := spec.Values[key].(string)
	return strings.TrimSpace(value)
}

var _ agenthost.Runner = CoordinationRunner{}
var _ agenthost.Runner = (*NativeCoordinationRuntime)(nil)
var _ CoordinationSubagentStarter = (*NativeCoordinationRuntime)(nil)
