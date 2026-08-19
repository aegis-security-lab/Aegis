package control

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"aegis/agenthost"
	boardcoordination "aegis/apps/board/coordination"
	"aegis/apps/board/identity"
	"aegis/capability"
	"aegis/coordination"
	coordinationsqlite "aegis/coordination/sqlitestore"
	"aegis/observability"
	"aegis/platform/dataspace"
	platformwork "aegis/platform/work"
	workcoordination "aegis/platform/work/coordinationadapter"
	worksqlite "aegis/platform/work/sqlitestore"
	"aegis/storage"
	storagesqlite "aegis/storage/sqlitestore"
	"github.com/z3r2ne/agentcore"
	"gorm.io/gorm"
)

type CoordinationDelivery interface {
	DeliverCoordinationMessage(context.Context, boardcoordination.AgentCommand) error
}

type CoordinationSubagentStarter interface {
	StartCoordinationSubagent(context.Context, boardcoordination.StartSubagentCommand) error
}

const (
	controlIssueIDValue             = "control.issueId"
	controlPreparedExecutionIDValue = "control.preparedExecutionId"
	controlPreparedPromptValue      = "control.preparedPrompt"
	controlWakeupIDValue            = "control.wakeupId"
)

// CoordinationBridge adapts Aegis Board/Relay/Issue state to the independent
// coordination Engine. Mode code never imports control or touches GORM.
type CoordinationBridge struct {
	manager      *Manager
	repository   coordination.Repository
	modes        *coordination.Registry
	runtime      *coordination.Runtime
	executions   *coordination.ExecutionQueue
	works        *platformwork.Gateway
	workInbox    *platformwork.Inbox
	events       storage.EventStore
	delivery     CoordinationDelivery
	subagents    CoordinationSubagentStarter
	defaultMode  string
	planner      *coordination.CapabilityPlanner
	phoneEnabled bool
}

type CoordinationBridgeOptions struct {
	WorkerID     string
	DefaultMode  string
	Delivery     CoordinationDelivery
	Subagents    CoordinationSubagentStarter
	Executor     agenthost.Runner
	Concurrency  int
	Planner      *coordination.CapabilityPlanner
	PhoneEnabled bool
}

func (b *CoordinationBridge) SetCapabilityPlanner(planner *coordination.CapabilityPlanner, phoneEnabled bool) {
	if b == nil {
		return
	}
	b.planner = planner
	b.phoneEnabled = phoneEnabled
}

func (b *CoordinationBridge) CapabilityCatalog() []capability.Descriptor {
	if b == nil || b.planner == nil || b.planner.Catalog == nil {
		return nil
	}
	if catalog, ok := b.planner.Catalog.(interface {
		Descriptors() []capability.Descriptor
	}); ok {
		return catalog.Descriptors()
	}
	return nil
}

func (b *CoordinationBridge) PlanAgentCapabilities(ctx context.Context, coordinationID, agentID string, requested []capability.Ref, selection coordination.CapabilitySelection) (coordination.CapabilityDecision, error) {
	if b == nil || b.manager == nil {
		return coordination.CapabilityDecision{}, errors.New("control coordination: runtime is disabled")
	}
	agent, err := b.manager.store.GetAgent(strings.TrimSpace(agentID))
	if err != nil {
		return coordination.CapabilityDecision{}, err
	}
	cfg := b.manager.store.effectiveAgentConfig(agent)
	defaults := defaultNativeCapabilities(agent, cfg, b.phoneEnabled)
	required := []capability.Ref{
		workspaceCapabilityRef(agent.Permissions),
		{Kind: capability.KindTool, Name: "delivery"},
	}
	if b.phoneEnabled {
		required = append(required, capability.Ref{Kind: capability.KindPhone, Name: "default"})
	}
	decision, err := planExecutionCapabilities(ctx, b.manager, b.planner, coordinationID, defaults, requested, required, selection)
	if err != nil {
		observability.Default().Warn(ctx, "coordination.capability_plan.rejected", slog.String("coordination_id", coordinationID), slog.String("agent_id", agentID), slog.Int("requested_count", len(requested)), slog.String("error", err.Error()))
		observability.DefaultMetrics().AddCounter("coordination_capability_plans_total", 1, observability.Labels{"status": "rejected"})
		return coordination.CapabilityDecision{}, err
	}
	observability.Default().Info(ctx, "coordination.capability_plan.approved", slog.String("coordination_id", coordinationID), slog.String("agent_id", agentID), slog.String("selection", string(decision.Selection)), slog.Int("capability_count", len(decision.Capabilities)))
	observability.DefaultMetrics().AddCounter("coordination_capability_plans_total", 1, observability.Labels{"status": "approved"})
	return decision, nil
}

func workspaceCapabilityRef(permissions PermissionBoundary) capability.Ref {
	return capability.Ref{
		Kind: capability.KindTool,
		Name: "workspace",
		Config: map[string]any{
			"allowShell": permissions.AllowShell,
			"allowWrite": permissions.AllowWrite,
		},
	}
}

func (b *CoordinationBridge) PreviewAgentCapabilities(ctx context.Context, issueID string, child boardcoordination.ChildWork) (coordination.CapabilityDecision, error) {
	if b == nil || b.manager == nil {
		return coordination.CapabilityDecision{}, errors.New("control coordination: runtime is disabled")
	}
	issue, err := b.manager.store.GetIssue(strings.TrimSpace(issueID))
	if err != nil {
		return coordination.CapabilityDecision{}, err
	}
	root, err := b.manager.store.taskRoot(issue)
	if err != nil {
		return coordination.CapabilityDecision{}, err
	}
	return b.PlanAgentCapabilities(ctx, root.ID, child.AgentID, child.Capabilities, child.CapabilitySelection)
}

func (m *Manager) SetCoordination(bridge *CoordinationBridge) {
	if m == nil {
		return
	}
	m.coordinationMu.Lock()
	m.coordination = bridge
	m.coordinationMu.Unlock()
	// Recovery may enqueue prepared executions, so it must not race ahead of
	// Coordination construction during process startup.
	if bridge != nil && m.store != nil && m.store.Config().Configured {
		m.recoveryOnce.Do(func() { go m.resumeWork() })
	}
}

func (m *Manager) Coordination() *CoordinationBridge {
	if m == nil {
		return nil
	}
	m.coordinationMu.RLock()
	bridge := m.coordination
	m.coordinationMu.RUnlock()
	return bridge
}

func NewCoordinationBridge(manager *Manager, options CoordinationBridgeOptions) (*CoordinationBridge, error) {
	if manager == nil || manager.store == nil {
		return nil, errors.New("control coordination: manager is required")
	}
	if options.Executor == nil {
		return nil, errors.New("control coordination: Agent executor is required")
	}
	repository, err := coordinationsqlite.New(manager.store.db)
	if err != nil {
		return nil, err
	}
	modes := coordination.NewRegistry()
	if err := boardcoordination.Register(modes); err != nil {
		return nil, err
	}
	defaultMode := strings.TrimSpace(options.DefaultMode)
	if defaultMode == "" {
		defaultMode = "board_autonomy"
	}
	if _, err := modes.Resolve(defaultMode, "1"); err != nil {
		return nil, err
	}
	delivery := options.Delivery
	if delivery == nil {
		delivery = unavailableCoordinationDelivery{}
	}
	events, err := storagesqlite.New(manager.store.db)
	if err != nil {
		return nil, err
	}
	workerID := fallback(strings.TrimSpace(options.WorkerID), "control-coordination")
	bridge := &CoordinationBridge{
		manager: manager, repository: repository, modes: modes, delivery: delivery, subagents: options.Subagents,
		defaultMode: defaultMode, planner: options.Planner, phoneEnabled: options.PhoneEnabled, events: events,
	}
	router := coordination.NewEffectRouter()
	engine := &coordination.Engine{Repository: repository, Modes: modes, Snapshots: coordination.SnapshotLoaderFunc(bridge.snapshot), WorkerID: workerID + "-decisions"}
	effects := &coordination.EffectWorker{Repository: repository, Handler: router, WorkerID: workerID + "-effects"}
	bridge.executions = &coordination.ExecutionQueue{Repository: repository, DefaultMaxAttempts: 3}
	applicationWorkStore, err := worksqlite.New(manager.store.db)
	if err != nil {
		return nil, err
	}
	bridge.works = &platformwork.Gateway{
		Repository: applicationWorkStore, Events: applicationWorkStore,
		Scheduler: platformwork.SchedulerFunc(bridge.scheduleBoardWork),
	}
	bridge.workInbox = &platformwork.Inbox{Events: applicationWorkStore, Subscriptions: applicationWorkStore}
	applicationLifecycle := &workcoordination.Lifecycle{Repository: applicationWorkStore, Events: applicationWorkStore}
	concurrency := options.Concurrency
	if concurrency <= 0 {
		concurrency = manager.store.Config().Concurrency
	}
	if concurrency <= 0 {
		concurrency = 1
	}
	// The configured workers remain available for ordinary Issue work. One
	// additional control-plane worker only claims urgent wake/resume executions,
	// so a saturated child wave can never starve its sleeping parent.
	workers := make([]*coordination.ExecutionWorker, concurrency+1)
	for index := 0; index < concurrency; index++ {
		workers[index] = &coordination.ExecutionWorker{Repository: repository, Executor: options.Executor, Lifecycle: applicationLifecycle, WorkerID: fmt.Sprintf("%s-execution-%d", workerID, index+1)}
		workers[index].Sinks = coordination.ExecutionEventSinkFactoryFunc(func(_ context.Context, execution coordination.Execution) (agentcore.EventSink, error) {
			return storage.EventSink(events, execution.ID, execution.Attempt, nil), nil
		})
	}
	workers[concurrency] = &coordination.ExecutionWorker{
		Repository: repository, Executor: options.Executor, WorkerID: workerID + "-wakeups",
		Lifecycle:       applicationLifecycle,
		MinimumPriority: coordination.ExecutionPriorityWakeup,
		Sinks: coordination.ExecutionEventSinkFactoryFunc(func(_ context.Context, execution coordination.Execution) (agentcore.EventSink, error) {
			return storage.EventSink(events, execution.ID, execution.Attempt, nil), nil
		}),
	}
	runtime, err := coordination.NewRuntime(coordination.RuntimeConfig{Engine: engine, Effects: effects, Executions: bridge.executions, ExecutionWorkers: workers})
	if err != nil {
		return nil, err
	}
	bridge.runtime = runtime
	registrations := map[coordination.EffectType]coordination.EffectHandler{
		boardcoordination.EffectEnqueueIssue:   coordination.EffectHandlerFunc(bridge.enqueueIssue),
		boardcoordination.EffectCreateIssue:    coordination.EffectHandlerFunc(bridge.createIssue),
		boardcoordination.EffectAssignIssue:    coordination.EffectHandlerFunc(bridge.assignIssue),
		boardcoordination.EffectSendRelay:      coordination.EffectHandlerFunc(bridge.sendRelay),
		boardcoordination.EffectDeliverMessage: coordination.EffectHandlerFunc(bridge.deliverMessage),
		boardcoordination.EffectResumeAgent:    coordination.EffectHandlerFunc(bridge.deliverMessage),
		boardcoordination.EffectSuspendAgent:   coordination.EffectHandlerFunc(bridge.suspendAgent),
		boardcoordination.EffectCancelWork:     coordination.EffectHandlerFunc(bridge.cancelWork),
		boardcoordination.EffectStartSubagent:  coordination.EffectHandlerFunc(bridge.startSubagent),
		coordination.EffectScheduleWakeup:      coordination.WakeupHandler{Submit: runtime.Submit},
	}
	for kind, handler := range registrations {
		if err := router.Register(kind, handler); err != nil {
			return nil, err
		}
	}
	return bridge, nil
}

type unavailableCoordinationDelivery struct{}

func (unavailableCoordinationDelivery) DeliverCoordinationMessage(context.Context, boardcoordination.AgentCommand) error {
	return errors.New("control coordination: task-local Agent delivery is unavailable")
}

func (b *CoordinationBridge) Start(ctx context.Context) { b.runtime.Start(ctx) }
func (b *CoordinationBridge) Close()                    { b.runtime.Close() }

func (b *CoordinationBridge) Modes() []coordination.ModeDescriptor { return b.modes.Modes() }

// RegisterAgentWorkSubscription creates a durable Board inbox cursor for one
// task scope. Pull and Ack are deliberately separate so Board can commit its
// domain projection before advancing delivery.
func (b *CoordinationBridge) RegisterAgentWorkSubscription(ctx context.Context, subscription platformwork.Subscription) error {
	if b == nil || b.workInbox == nil {
		return errors.New("control coordination: application event inbox is unavailable")
	}
	if subscription.Filter.AppID != identity.AppID {
		return errors.New("control coordination: subscription must belong to the Board application")
	}
	return b.workInbox.Register(ctx, subscription)
}

func (b *CoordinationBridge) PullAgentWorkEvents(ctx context.Context, subscriptionID string, limit int) (platformwork.Delivery, error) {
	if b == nil || b.workInbox == nil {
		return platformwork.Delivery{}, errors.New("control coordination: application event inbox is unavailable")
	}
	return b.workInbox.Pull(ctx, subscriptionID, limit)
}

func (b *CoordinationBridge) AckAgentWorkEvents(ctx context.Context, subscriptionID string, cursor platformwork.Cursor) error {
	if b == nil || b.workInbox == nil {
		return errors.New("control coordination: application event inbox is unavailable")
	}
	return b.workInbox.Ack(ctx, subscriptionID, cursor)
}

func (b *CoordinationBridge) DirectSubagentsAvailable() bool { return b != nil && b.subagents != nil }

func (b *CoordinationBridge) BindIssue(ctx context.Context, issueID, mode, version string, config json.RawMessage) (coordination.Binding, error) {
	issue, err := b.manager.store.GetIssue(issueID)
	if err != nil {
		return coordination.Binding{}, err
	}
	root, err := b.manager.store.taskRoot(issue)
	if err != nil {
		return coordination.Binding{}, err
	}
	if version == "" {
		version = "1"
	}
	if _, err := b.modes.Resolve(mode, version); err != nil {
		return coordination.Binding{}, err
	}
	binding := coordination.Binding{CoordinationID: root.ID, Mode: mode, Version: version, Config: append([]byte(nil), config...), UpdatedAt: time.Now().UTC()}
	if _, err := coordination.CapabilityPolicyFromBinding(binding); err != nil {
		return coordination.Binding{}, err
	}
	if err := b.repository.SaveBinding(ctx, binding); err != nil {
		return coordination.Binding{}, err
	}
	payload, _ := json.Marshal(map[string]string{"mode": mode, "version": version})
	_, err = b.runtime.Submit(ctx, coordination.Event{ID: fmt.Sprintf("coordination-mode:%s:%d", root.ID, binding.UpdatedAt.UnixNano()), Type: coordination.EventModeChanged, CoordinationID: root.ID, ScopeID: root.ID, SubjectID: issue.ID, OccurredAt: binding.UpdatedAt, Payload: payload})
	if err != nil {
		return coordination.Binding{}, err
	}
	// Re-evaluate currently assignable work under the new mode.
	if (issue.Status == "todo") && issue.AssigneeAgentID != "" {
		_ = b.SubmitIssueAssigned(ctx, issue)
	}
	return binding, nil
}

func (b *CoordinationBridge) BindingForIssue(ctx context.Context, issueID string) (coordination.Binding, error) {
	issue, err := b.manager.store.GetIssue(issueID)
	if err != nil {
		return coordination.Binding{}, err
	}
	root, err := b.manager.store.taskRoot(issue)
	if err != nil {
		return coordination.Binding{}, err
	}
	return b.ensureBinding(ctx, root.ID)
}

func (b *CoordinationBridge) SubmitIssueCreated(ctx context.Context, issue Issue) error {
	return b.submitSchedulableIssue(ctx, boardcoordination.EventIssueCreated, issue, "created")
}

func (b *CoordinationBridge) SubmitIssueAssigned(ctx context.Context, issue Issue) error {
	return b.submitSchedulableIssue(ctx, boardcoordination.EventIssueAssigned, issue, fmt.Sprintf("assigned:%s:%d", issue.AssigneeAgentID, issue.UpdatedAt.UnixNano()))
}

// DispatchIssue is the only public Issue execution gateway.
// AgentHost is an executor behind Coordination, never a public source of
// orchestration decisions.
func (b *CoordinationBridge) DispatchIssue(ctx context.Context, issueID string) error {
	if b == nil || b.manager == nil {
		return errors.New("control coordination: nil dispatch gateway")
	}
	issue, err := b.manager.store.GetIssue(strings.TrimSpace(issueID))
	if err != nil {
		return err
	}
	return b.SubmitIssueAssigned(ctx, issue)
}

// submitSchedulableIssue reserves dispatch ownership before publishing the
// asynchronous Coordination event. Without this transition, a caller that
// scans todo Issues can publish thousands of unique assignment events before
// the first enqueue effect has a chance to mark the Issue scheduled.
func (b *CoordinationBridge) submitSchedulableIssue(ctx context.Context, eventType coordination.EventType, issue Issue, suffix string) error {
	if strings.TrimSpace(issue.AssigneeAgentID) == "" {
		return b.submitIssue(ctx, eventType, issue, suffix)
	}
	if issue.Status != "todo" {
		return errors.New("Issue 当前不可调度")
	}
	if issue.ExecutionPhase == "scheduled" {
		return nil
	}
	previousPhase := fallback(strings.TrimSpace(issue.ExecutionPhase), "active")
	now := time.Now()
	reserved := b.manager.store.db.Model(&Issue{}).
		Where("id = ? AND status IN ? AND execution_phase = ?", issue.ID, []string{"todo"}, issue.ExecutionPhase).
		Updates(map[string]any{"execution_phase": "scheduled", "updated_at": now})
	if reserved.Error != nil {
		return reserved.Error
	}
	if reserved.RowsAffected != 1 {
		current, err := b.manager.store.GetIssue(issue.ID)
		if err == nil && current.ExecutionPhase == "scheduled" && (current.Status == "todo") {
			return nil
		}
		return errors.New("Issue 派发状态已发生变化")
	}
	if err := b.submitIssue(ctx, eventType, issue, suffix); err != nil {
		_ = b.manager.store.db.Model(&Issue{}).
			Where("id = ? AND status IN ? AND execution_phase = ? AND checkout_execution_id = ''", issue.ID, []string{"todo"}, "scheduled").
			Updates(map[string]any{"execution_phase": previousPhase, "updated_at": time.Now()}).Error
		return err
	}
	b.manager.store.notify()
	return nil
}

func (b *CoordinationBridge) SubmitIssueCompleted(ctx context.Context, issue Issue) error {
	root, parent, parentAgent, parentTaskAgent, err := b.issueContext(issue)
	if err != nil {
		return err
	}
	if _, err := b.ensureBinding(ctx, root.ID); err != nil {
		return err
	}
	payload, _ := json.Marshal(boardcoordination.Completion{Result: issue.Result, Success: issue.Status == "done" || issue.Status == "in_review", Status: issue.Status, Error: issue.Error, ChildID: issue.ID})
	anchor := issue.UpdatedAt.UnixNano()
	if issue.CompletedAt != nil {
		anchor = issue.CompletedAt.UnixNano()
	}
	_, err = b.runtime.Submit(ctx, coordination.Event{
		ID: fmt.Sprintf("issue-completed:%s:%s:%d", issue.ID, issue.Status, anchor), Type: boardcoordination.EventIssueCompleted,
		CoordinationID: root.ID, ScopeID: root.ID, SubjectID: issue.ID, ParentSubjectID: parent.ID,
		ExecutionID: issue.CurrentExecutionID, ActorID: issue.AssigneeAgentID, ActorInstanceID: issue.AssigneeTaskAgentID,
		ParentActorID: parentAgent, ParentActorInstanceID: parentTaskAgent,
		OccurredAt: issue.UpdatedAt.UTC(), Payload: payload,
	})
	return err
}

// ContinueTerminalChildIssue is the explicit parent decision that converts a
// failed or budget-exceeded child back into schedulable work. It never mutates
// or reuses the previous Agent Execution; normal assignment creates a new one.
func (b *CoordinationBridge) ContinueTerminalChildIssue(ctx context.Context, invocation boardcoordination.Invocation, request boardcoordination.ContinueRequest) error {
	if b == nil || b.manager == nil || b.manager.store == nil {
		return errors.New("control coordination: runtime is disabled")
	}
	request.ChildIssueID = strings.TrimSpace(request.ChildIssueID)
	request.Reason = strings.TrimSpace(request.Reason)
	if request.ChildIssueID == "" || request.Reason == "" {
		return errors.New("control coordination: childIssueId and reason are required")
	}
	parent, err := b.manager.store.GetIssue(strings.TrimSpace(invocation.IssueID))
	if err != nil {
		return err
	}
	child, err := b.manager.store.GetIssue(request.ChildIssueID)
	if err != nil {
		return err
	}
	if child.ParentID != parent.ID {
		return errors.New("control coordination: only a direct parent may continue a budget-exceeded child")
	}
	if child.Status != "done" || !hasIssueLabel(child, issueLabelFailed, issueLabelBudgetExceeded) {
		return errors.New("control coordination: child Issue is neither failed nor budget_exceeded")
	}
	root, err := b.manager.store.taskRoot(parent)
	if err != nil {
		return err
	}
	if _, _, remaining, configured := taskWallClockRemaining(root, time.Now()); configured && remaining <= 0 {
		return errors.New("control coordination: Task wall-clock budget is exhausted")
	}
	now := time.Now()
	err = b.manager.store.db.Transaction(func(tx *gorm.DB) error {
		updated := tx.Model(&Issue{}).
			Where("id = ? AND parent_id = ? AND status = ?", child.ID, parent.ID, "done").
			Updates(map[string]any{
				"status": "todo", "labels": issueLabelsColumn(withoutIssueLabels(child, issueLabelFailed, issueLabelBudgetExceeded)), "execution_phase": "active", "checkout_execution_id": "",
				"completed_at": nil, "cancelled_at": nil, "error": "", "updated_at": now,
			})
		if updated.Error != nil {
			return updated.Error
		}
		if updated.RowsAffected != 1 {
			return errors.New("control coordination: child Issue changed before continuation")
		}
		if child.AssigneeTaskAgentID != "" {
			if err := tx.Model(&TaskAgent{}).Where("id = ?", child.AssigneeTaskAgentID).Updates(map[string]any{"status": "active", "updated_at": now}).Error; err != nil {
				return err
			}
		}
		return createSystemComment(tx, child.ID, "## 父 Issue 批准恢复执行\n\n"+request.Reason+"\n\n系统将创建新的 Execution；失败的旧 Execution 保持不变，单次 Execution 预算重新计数，Task 总时钟墙预算不变。", now)
	})
	if err != nil {
		return err
	}
	b.manager.store.addEvent(child.CurrentExecutionID, child.ID, "coordination", "父 Issue 已批准恢复子 Issue", request.Reason)
	b.manager.store.notify()
	reopened, err := b.manager.store.GetIssue(child.ID)
	if err != nil {
		return err
	}
	return b.SubmitIssueAssigned(ctx, reopened)
}

func (b *CoordinationBridge) SubmitDelegation(ctx context.Context, eventID, issueID, executionID, agentID string, request boardcoordination.DelegationRequest) error {
	issue, err := b.manager.store.GetIssue(issueID)
	if err != nil {
		return err
	}
	root, err := b.manager.store.taskRoot(issue)
	if err != nil {
		return err
	}
	if _, err := b.ensureBinding(ctx, root.ID); err != nil {
		return err
	}
	for index, child := range request.Children {
		if _, err := b.PlanAgentCapabilities(ctx, root.ID, child.AgentID, child.Capabilities, child.CapabilitySelection); err != nil {
			return fmt.Errorf("control coordination: child %d capability plan: %w", index, err)
		}
	}
	payload, err := json.Marshal(request)
	if err != nil {
		return err
	}
	if strings.TrimSpace(eventID) == "" {
		return errors.New("control coordination: delegation event ID is required")
	}
	_, err = b.runtime.Submit(ctx, coordination.Event{ID: eventID, Type: boardcoordination.EventDelegationRequested, CoordinationID: root.ID, ScopeID: root.ID, SubjectID: issue.ID, ParentSubjectID: issue.ParentID, ExecutionID: executionID, ActorID: agentID, ActorInstanceID: issue.AssigneeTaskAgentID, OccurredAt: time.Now().UTC(), Payload: payload})
	return err
}

// InvokeAgent is the operator/service-facing entry point for proactive work.
// It goes through the same durable mode decision and outbox as Agent-authored
// Phone Board delegation and service calls share this path, so no caller
// bypasses Coordination policy.
func (b *CoordinationBridge) InvokeAgent(ctx context.Context, invocation boardcoordination.AgentInvocation) (boardcoordination.InvocationReceipt, error) {
	if b == nil || b.manager == nil {
		return boardcoordination.InvocationReceipt{}, errors.New("control coordination: runtime is disabled")
	}
	invocation.IssueID = strings.TrimSpace(invocation.IssueID)
	invocation.Child.AgentID = strings.TrimSpace(invocation.Child.AgentID)
	invocation.Child.Prompt = strings.TrimSpace(invocation.Child.Prompt)
	if invocation.IssueID == "" || invocation.Child.AgentID == "" || invocation.Child.Prompt == "" {
		return boardcoordination.InvocationReceipt{}, errors.New("control coordination: issueId, child.agentId and child.prompt are required")
	}
	issue, err := b.manager.store.GetIssue(invocation.IssueID)
	if err != nil {
		return boardcoordination.InvocationReceipt{}, err
	}
	root, err := b.manager.store.taskRoot(issue)
	if err != nil {
		return boardcoordination.InvocationReceipt{}, err
	}
	eventID := "proactive-invoke:" + observability.NewID(16)
	request := boardcoordination.DelegationRequest{Children: []boardcoordination.ChildWork{invocation.Child}, ParentBehavior: invocation.ParentBehavior, ResultDelivery: invocation.ResultDelivery}
	if err := b.SubmitDelegation(ctx, eventID, issue.ID, strings.TrimSpace(invocation.ParentExecutionID), fallback(strings.TrimSpace(invocation.ParentAgentID), "operator"), request); err != nil {
		return boardcoordination.InvocationReceipt{}, err
	}
	return boardcoordination.InvocationReceipt{EventID: eventID, CoordinationID: root.ID, IssueID: issue.ID, AgentID: invocation.Child.AgentID, Accepted: true}, nil
}

func (b *CoordinationBridge) SubmitWait(ctx context.Context, eventID, issueID, executionID, agentID string, request boardcoordination.WaitRequest) error {
	issue, err := b.manager.store.GetIssue(issueID)
	if err != nil {
		return err
	}
	root, err := b.manager.store.taskRoot(issue)
	if err != nil {
		return err
	}
	if _, err := b.ensureBinding(ctx, root.ID); err != nil {
		return err
	}
	payload, err := json.Marshal(request)
	if err != nil {
		return err
	}
	if strings.TrimSpace(eventID) == "" {
		return errors.New("control coordination: wait event ID is required")
	}
	_, err = b.runtime.Submit(ctx, coordination.Event{ID: eventID, Type: boardcoordination.EventWaitRequested, CoordinationID: root.ID, ScopeID: root.ID, SubjectID: issue.ID, ParentSubjectID: issue.ParentID, ExecutionID: executionID, ActorID: agentID, ActorInstanceID: issue.AssigneeTaskAgentID, OccurredAt: time.Now().UTC(), Payload: payload})
	return err
}

func (b *CoordinationBridge) SubmitExecutionCompleted(ctx context.Context, eventID string, command boardcoordination.StartSubagentCommand, executionID, childAgentID string, completed boardcoordination.Completion) error {
	if strings.TrimSpace(eventID) == "" || strings.TrimSpace(command.CoordinationID) == "" || strings.TrimSpace(command.CorrelationID) == "" {
		return errors.New("control coordination: completion event, coordination and correlation IDs are required")
	}
	if _, err := b.ensureBinding(ctx, command.CoordinationID); err != nil {
		return err
	}
	payload, err := json.Marshal(completed)
	if err != nil {
		return err
	}
	_, err = b.runtime.Submit(ctx, coordination.Event{
		ID: eventID, Type: boardcoordination.EventExecutionCompleted, CoordinationID: command.CoordinationID, ScopeID: command.CoordinationID,
		ParentSubjectID: command.ParentIssueID, ExecutionID: executionID, ActorID: childAgentID, ParentActorID: command.ParentAgentID,
		CorrelationID: command.CorrelationID, OccurredAt: time.Now().UTC(), Payload: payload,
	})
	return err
}

func (b *CoordinationBridge) SubmitRelay(ctx context.Context, message RelayMessage) error {
	if len(message.RecipientIDs) == 0 {
		return nil
	}
	coordinationID := ""
	if message.IssueID != "" {
		if issue, err := b.manager.store.GetIssue(message.IssueID); err == nil {
			if root, rootErr := b.manager.store.taskRoot(issue); rootErr == nil {
				coordinationID = root.ID
			}
		}
	}
	if coordinationID == "" {
		return errors.New("control coordination: Relay messages must belong to a task")
	}
	if _, err := b.ensureBinding(ctx, coordinationID); err != nil {
		return err
	}
	senderID := message.SenderID
	senderTaskAgentID, senderName := "", ""
	if identity, err := b.manager.store.taskAgent(coordinationID, message.SenderID); err == nil {
		senderID, senderTaskAgentID, senderName = identity.AgentID, identity.ID, identity.Name
	}
	for _, recipientID := range message.RecipientIDs {
		recipientAgentID, recipientTaskAgentID := recipientID, ""
		if identity, err := b.manager.store.taskAgent(coordinationID, recipientID); err == nil {
			recipientAgentID, recipientTaskAgentID = identity.AgentID, identity.ID
		}
		var recipientIssue Issue
		query := b.manager.store.db.Where("hidden = ?", false)
		if recipientTaskAgentID != "" {
			query = query.Where("assignee_task_agent_id = ?", recipientTaskAgentID)
		} else {
			query = query.Where("assignee_agent_id = ?", recipientAgentID)
		}
		var candidates []Issue
		if err := query.Order("updated_at desc").Find(&candidates).Error; err != nil {
			return err
		}
		for _, candidate := range candidates {
			if issueStatusTerminal(candidate.Status) {
				continue
			}
			root, rootErr := b.manager.store.taskRoot(candidate)
			if rootErr == nil && root.ID == coordinationID {
				recipientIssue = candidate
				break
			}
		}
		if recipientIssue.ID == "" {
			return fmt.Errorf("control coordination: no Issue for Relay recipient %s in task %s", recipientID, coordinationID)
		}
		parentAgentID, parentTaskAgentID := "", ""
		if recipientIssue.ParentID != "" {
			if parent, err := b.manager.store.GetIssue(recipientIssue.ParentID); err == nil {
				parentAgentID, parentTaskAgentID = parent.AssigneeAgentID, parent.AssigneeTaskAgentID
			}
		}
		payload, _ := json.Marshal(boardcoordination.RelayReceived{Channel: "relay", MessageID: message.ID, SenderID: senderID, SenderTaskAgentID: senderTaskAgentID, SenderName: senderName, Body: message.Body})
		if _, err := b.runtime.Submit(ctx, coordination.Event{
			ID: "relay-received:" + message.ID + ":" + recipientID, Type: boardcoordination.EventRelayReceived,
			CoordinationID: coordinationID, ScopeID: coordinationID, SubjectID: recipientIssue.ID, ParentSubjectID: recipientIssue.ParentID,
			ActorID: recipientAgentID, ActorInstanceID: recipientTaskAgentID, ParentActorID: parentAgentID, ParentActorInstanceID: parentTaskAgentID,
			OccurredAt: message.CreatedAt.UTC(), Payload: payload,
		}); err != nil {
			return err
		}
	}
	return nil
}

// SubmitIssueComment routes a durable Board comment to the exact TaskAgent
// assigned to that Issue. The Mode decides how it steers or wakes the loop.
func (b *CoordinationBridge) SubmitIssueComment(ctx context.Context, comment IssueComment) error {
	if b == nil || b.runtime == nil {
		return errors.New("control coordination: runtime is disabled")
	}
	issue, err := b.manager.store.GetIssue(comment.IssueID)
	if err != nil {
		return err
	}
	if issue.AssigneeAgentID == "" {
		return nil
	}
	if issue.AssigneeTaskAgentID == "" {
		issue, err = b.ensureIssueTaskAgent(issue)
		if err != nil {
			return err
		}
	}
	var activeExecutions int64
	if err := b.manager.store.db.Model(&Execution{}).Where("issue_id = ? AND task_agent_id = ? AND status IN ?", issue.ID, issue.AssigneeTaskAgentID, activeExecutionStatuses).Count(&activeExecutions).Error; err != nil {
		return err
	}
	if activeExecutions == 0 {
		// A comment on a non-running Issue is new work rather than a temporary
		// reply. A continuation wakeup permanently reopens every inactive state.
		wakeup := AgentWakeup{
			ID: nextID("wakeup"), IssueID: issue.ID, CommentID: comment.ID,
			AgentID: issue.AssigneeAgentID, Reason: "issue_comment_resume",
			Status: "queued", CreatedAt: comment.CreatedAt,
		}
		if err := b.manager.store.db.Create(&wakeup).Error; err != nil {
			return err
		}
		go b.manager.dispatchWakeup(wakeup.ID)
		b.manager.store.notify()
		return nil
	}
	root, err := b.manager.store.taskRoot(issue)
	if err != nil {
		return err
	}
	if _, err = b.ensureBinding(ctx, root.ID); err != nil {
		return err
	}
	body := comment.Body
	var objective IssueObjective
	if err := b.manager.store.db.Where("source_comment_id = ?", comment.ID).First(&objective).Error; err == nil {
		body += fmt.Sprintf("\n\n[Authoritative objective update · v%d]\n%s\n\nUse this as the current Issue objective for all subsequent work and delivery.", objective.Version, objective.Content)
	}
	payload, _ := json.Marshal(boardcoordination.RelayReceived{Channel: "board_comment", MessageID: comment.ID, SenderID: comment.AuthorID, SenderName: "操作员", Body: body})
	_, err = b.runtime.Submit(ctx, coordination.Event{
		ID: "issue-comment:" + comment.ID, Type: boardcoordination.EventRelayReceived,
		CoordinationID: root.ID, ScopeID: root.ID, SubjectID: issue.ID, ParentSubjectID: issue.ParentID,
		ActorID: issue.AssigneeAgentID, ActorInstanceID: issue.AssigneeTaskAgentID,
		OccurredAt: comment.CreatedAt.UTC(), Payload: payload,
	})
	return err
}

func (b *CoordinationBridge) ensureIssueTaskAgent(issue Issue) (Issue, error) {
	agent, err := b.manager.store.executionAgent(issue.AssigneeAgentID)
	if err != nil {
		return Issue{}, err
	}
	root, err := b.manager.store.taskRoot(issue)
	if err != nil {
		return Issue{}, err
	}
	err = b.manager.store.db.Transaction(func(tx *gorm.DB) error {
		// Recheck under the write transaction so simultaneous comments cannot
		// create two task-local identities for the same Issue.
		var current Issue
		if err := tx.First(&current, "id = ?", issue.ID).Error; err != nil {
			return err
		}
		if current.AssigneeTaskAgentID != "" {
			issue = current
			return nil
		}
		identity, err := claimTaskAgentTx(tx, root.ID, current.AssigneeAgentID, agent.Name, "")
		if err != nil {
			return err
		}
		if err := tx.Model(&Issue{}).Where("id = ? AND assignee_task_agent_id = ''", current.ID).Updates(map[string]any{
			"assignee_task_agent_id": identity.ID,
			"updated_at":             time.Now(),
		}).Error; err != nil {
			return err
		}
		current.AssigneeTaskAgentID = identity.ID
		issue = current
		return nil
	})
	return issue, err
}

func (b *CoordinationBridge) submitIssue(ctx context.Context, kind coordination.EventType, issue Issue, discriminator string) error {
	root, parent, parentAgent, parentTaskAgent, err := b.issueContext(issue)
	if err != nil {
		return err
	}
	if _, err := b.ensureBinding(ctx, root.ID); err != nil {
		return err
	}
	_, err = b.runtime.Submit(ctx, coordination.Event{
		ID: fmt.Sprintf("%s:%s:%s", kind, issue.ID, discriminator), Type: kind,
		CoordinationID: root.ID, ScopeID: root.ID, SubjectID: issue.ID, ParentSubjectID: parent.ID,
		ExecutionID: issue.CurrentExecutionID, ActorID: issue.AssigneeAgentID, ActorInstanceID: issue.AssigneeTaskAgentID,
		ParentActorID: parentAgent, ParentActorInstanceID: parentTaskAgent,
		OccurredAt: issue.UpdatedAt.UTC(),
	})
	return err
}

func (b *CoordinationBridge) issueContext(issue Issue) (Issue, Issue, string, string, error) {
	root, err := b.manager.store.taskRoot(issue)
	if err != nil {
		return Issue{}, Issue{}, "", "", err
	}
	parent := Issue{}
	parentAgent := issue.CreatedBy
	parentTaskAgent := ""
	if issue.ParentID != "" {
		if parent, err = b.manager.store.GetIssue(issue.ParentID); err != nil {
			return Issue{}, Issue{}, "", "", err
		}
		parentAgent = parent.AssigneeAgentID
		parentTaskAgent = parent.AssigneeTaskAgentID
	}
	return root, parent, parentAgent, parentTaskAgent, nil
}

func (b *CoordinationBridge) ensureBinding(ctx context.Context, id string) (coordination.Binding, error) {
	binding, err := b.repository.Binding(ctx, id)
	if err == nil {
		return binding, nil
	}
	if !errors.Is(err, coordination.ErrNotFound) {
		return coordination.Binding{}, err
	}
	heartbeat := normalizeIssueHeartbeat(b.manager.store.Config().IssueHeartbeat)
	config, _ := json.Marshal(map[string]int64{"heartbeatIntervalSeconds": int64(heartbeat.IntervalSeconds)})
	binding = coordination.Binding{CoordinationID: id, Mode: b.defaultMode, Version: "1", Config: config, UpdatedAt: time.Now().UTC()}
	return binding, b.repository.SaveBinding(ctx, binding)
}

// Binding returns the durable policy for one coordination root, creating the
// default binding when this is the first control-plane operation for it.
func (b *CoordinationBridge) Binding(ctx context.Context, coordinationID string) (coordination.Binding, error) {
	if b == nil || strings.TrimSpace(coordinationID) == "" {
		return coordination.Binding{}, errors.New("control coordination: coordination ID is required")
	}
	return b.ensureBinding(ctx, strings.TrimSpace(coordinationID))
}

func (b *CoordinationBridge) snapshot(_ context.Context, event coordination.Event) (coordination.Snapshot, error) {
	if event.SubjectID == "" {
		return coordination.Snapshot{}, nil
	}
	issue, err := b.manager.store.GetIssue(event.SubjectID)
	if err != nil {
		return coordination.Snapshot{}, err
	}
	snapshot := coordination.Snapshot{Current: b.workItem(issue)}
	parentID := issue.ParentID
	if parentID == "" {
		parentID = event.ParentSubjectID
	}
	if parentID != "" {
		parent, err := b.manager.store.GetIssue(parentID)
		if err != nil {
			return coordination.Snapshot{}, err
		}
		snapshot.Parent = b.workItem(parent)
	}
	var children []Issue
	if err := b.manager.store.db.Where("parent_id = ?", issue.ID).Order("number asc").Find(&children).Error; err != nil {
		return coordination.Snapshot{}, err
	}
	for _, child := range children {
		snapshot.Children = append(snapshot.Children, *b.workItem(child))
	}
	return snapshot, nil
}

func (b *CoordinationBridge) workItem(issue Issue) *coordination.WorkItem {
	status := coordination.WorkPending
	switch issue.Status {
	case "in_progress":
		if !hasIssueLabel(issue, issueLabelBlocked) {
			status = coordination.WorkRunning
		}
	case "in_review":
		status = coordination.WorkRunning
	case "done":
		if hasIssueLabel(issue, issueLabelFailed, issueLabelBudgetExceeded) {
			status = coordination.WorkFailed
		} else {
			status = coordination.WorkSucceeded
		}
	case "cancelled":
		status = coordination.WorkCancelled
	}
	if issue.ExecutionPhase == "waiting_children" {
		status = coordination.WorkWaiting
	}
	item := &coordination.WorkItem{ID: issue.ID, ParentID: issue.ParentID, Title: issue.Title, AssigneeID: issue.AssigneeAgentID, AssigneeInstanceID: issue.AssigneeTaskAgentID, Status: status, ExecutionPhase: issue.ExecutionPhase, SleepToken: issue.SleepToken, UpdatedAt: issue.UpdatedAt}
	if issue.AssigneeTaskAgentID != "" {
		var identity TaskAgent
		if err := b.manager.store.db.First(&identity, "id = ?", issue.AssigneeTaskAgentID).Error; err == nil {
			item.AssigneeInstanceName = identity.Name
		}
	}
	var progress ExecutionProgress
	if err := b.manager.store.db.Where("issue_id = ?", issue.ID).Order("created_at desc, id desc").First(&progress).Error; err == nil {
		item.ProgressSummary = progress.Summary
		item.CurrentActivity = progress.CurrentActivity
	}
	return item
}

func (b *CoordinationBridge) enqueueIssue(ctx context.Context, effect coordination.Effect) error {
	var command boardcoordination.EnqueueIssueCommand
	if err := json.Unmarshal(effect.Payload, &command); err != nil {
		return err
	}
	return b.EnqueueIssueExecution(ctx, command.IssueID)
}

func (b *CoordinationBridge) EnqueueIssueExecution(ctx context.Context, issueID string) error {
	if b == nil || b.works == nil || b.manager == nil || b.manager.store == nil {
		return errors.New("control coordination: Agent Work gateway is unavailable")
	}
	issue, err := b.manager.store.GetIssue(issueID)
	if err != nil {
		return err
	}
	root, err := b.manager.store.taskRoot(issue)
	if err != nil {
		return err
	}
	subjectID := fallback(strings.TrimSpace(issue.AssigneeTaskAgentID), strings.TrimSpace(issue.AssigneeAgentID))
	_, err = b.works.Request(ctx, platformwork.Request{
		AppID: identity.AppID, ScopeID: root.ID, CorrelationID: issue.ID,
		IdempotencyKey: "board:issue-run:" + issue.ID + ":" + strings.TrimSpace(issue.CurrentExecutionID),
		AgentProfileID: issue.AssigneeAgentID, SessionKey: issue.AssigneeTaskAgentID, Prompt: issue.Title,
		Priority: issueExecutionPriority(issue.Priority), Capabilities: append([]capability.Ref(nil), issue.Capabilities...),
		DataSpaces:    []dataspace.Grant{{Ref: dataspace.Ref{AppID: identity.AppID, Space: identity.PrimaryDataSpace, ScopeID: root.ID}, SubjectID: subjectID, Actions: []string{"read", "write"}}},
		InstalledApps: []string{"aegis.board", "aegis.relay"},
		Metadata:      map[string]string{"board.issueId": issue.ID, "board.taskId": root.ID},
	})
	return err
}

func (b *CoordinationBridge) scheduleBoardWork(ctx context.Context, scheduled platformwork.ScheduledWork) (string, error) {
	if scheduled.Request.AppID != identity.AppID {
		return "", fmt.Errorf("control coordination: unsupported application %q", scheduled.Request.AppID)
	}
	return b.enqueueIssueExecutionDirect(ctx, scheduled)
}

func (b *CoordinationBridge) enqueueIssueExecutionDirect(ctx context.Context, scheduled platformwork.ScheduledWork) (string, error) {
	if b == nil || b.executions == nil {
		return "", errors.New("control coordination: execution runtime is unavailable")
	}
	issueID := scheduled.Request.CorrelationID
	coordinationExecutionID := issueID
	if existing, lookupErr := b.executions.Get(ctx, issueID); lookupErr == nil {
		if existing.Status == coordination.ExecutionQueued || existing.Status == coordination.ExecutionRunning {
			return existing.ID, nil
		}
		// The stable first execution ID preserves simple Issue lookup. A reopened
		// budget outcome gets a new durable Coordination execution and therefore a
		// genuinely fresh Agent Execution budget.
		coordinationExecutionID = "issue-run-" + observability.NewID(16)
	} else if !errors.Is(lookupErr, coordination.ErrExecutionNotFound) {
		return "", lookupErr
	}
	issue, err := b.manager.store.GetIssue(issueID)
	if err != nil {
		return "", err
	}
	if issue.Status != "todo" {
		return "", errors.New("Issue 当前不可调度")
	}
	taskID := issue.ID
	if root, rootErr := b.manager.store.taskRoot(issue); rootErr == nil {
		taskID = root.ID
	}
	execution := coordination.Execution{ID: coordinationExecutionID, CoordinationID: taskID, MaxAttempts: 1, Priority: issueExecutionPriority(issue.Priority), Origin: coordination.ExecutionOrigin{
		AppID: scheduled.Request.AppID, TenantID: scheduled.Request.TenantID, ScopeID: scheduled.Request.ScopeID,
		CorrelationID: scheduled.Request.CorrelationID, RequestID: scheduled.RequestID, WorkID: scheduled.WorkID,
	}, Spec: agenthost.ExecutionSpec{
		ExecutionID: coordinationExecutionID, AgentID: issue.AssigneeAgentID, Workspace: issue.Workspace,
		Model: agenthost.ModelRef{Provider: "coordination", Model: "issue"}, Prompt: issue.Title,
		Values: map[string]any{controlIssueIDValue: issue.ID, "control.taskId": taskID, "control.taskAgentId": issue.AssigneeTaskAgentID},
	}}
	if _, err = b.manager.EnsureTaskPhone(ctx, taskID, issue); err != nil {
		return "", fmt.Errorf("provision task Phone: %w", err)
	}
	if err = b.executions.Enqueue(ctx, execution); err != nil {
		if errors.Is(err, coordination.ErrExecutionConflict) {
			return coordinationExecutionID, nil
		}
		return "", err
	}
	updated := b.manager.store.db.Model(&Issue{}).Where("id = ? AND status IN ?", issue.ID, []string{"todo"}).Updates(map[string]any{"execution_phase": "scheduled", "updated_at": time.Now()})
	if updated.Error != nil || updated.RowsAffected != 1 {
		_ = b.executions.Cancel(ctx, coordinationExecutionID, "Issue became unavailable before Coordination execution ownership was recorded")
		if updated.Error != nil {
			return "", updated.Error
		}
		return "", errors.New("Issue 当前不可调度")
	}
	b.manager.store.notify()
	return coordinationExecutionID, nil
}

func issueExecutionPriority(priority string) int {
	switch priority {
	case "high":
		return coordination.ExecutionPriorityIssueHigh
	case "low":
		return coordination.ExecutionPriorityIssueLow
	default:
		return coordination.ExecutionPriorityIssueMiddle
	}
}

// EnqueuePreparedIssueExecution schedules a durable domain Execution whose
// Issue transition has already been committed (validation, rework, recovery,
// or a wakeup). AgentCore materialization still happens only after a
// Coordination worker claims this envelope.
func (b *CoordinationBridge) EnqueuePreparedIssueExecution(ctx context.Context, issue Issue, execution Execution, prompt, wakeupID string, priority int) error {
	if b == nil || b.executions == nil || b.manager == nil {
		return errors.New("control coordination: execution runtime is unavailable")
	}
	prompt = strings.TrimSpace(prompt)
	if strings.TrimSpace(issue.ID) == "" || strings.TrimSpace(execution.ID) == "" || prompt == "" {
		return errors.New("control coordination: prepared Issue, Execution and prompt are required")
	}
	if execution.IssueID != issue.ID {
		return errors.New("control coordination: prepared Execution does not belong to Issue")
	}
	coordinationID := issue.ID
	if root, rootErr := b.manager.store.taskRoot(issue); rootErr == nil {
		coordinationID = root.ID
	}
	coordinationExecutionID := "prepared-" + execution.ID + "-" + observability.NewID(8)
	values := map[string]any{
		controlIssueIDValue: issue.ID, "control.taskId": coordinationID,
		"control.taskAgentId":           issue.AssigneeTaskAgentID,
		controlPreparedExecutionIDValue: execution.ID,
		controlPreparedPromptValue:      prompt,
	}
	if wakeupID = strings.TrimSpace(wakeupID); wakeupID != "" {
		values[controlWakeupIDValue] = wakeupID
	}
	spec := agenthost.ExecutionSpec{
		ExecutionID: coordinationExecutionID, AgentID: execution.AgentID, SessionID: execution.SessionID,
		Workspace: issue.Workspace, Model: agenthost.ModelRef{Provider: "coordination", Model: "prepared-issue"},
		Prompt: prompt, Values: values,
	}
	if execution.Kind != "validation" {
		if _, err := b.manager.EnsureTaskPhone(ctx, coordinationID, issue); err != nil {
			return fmt.Errorf("provision task Phone: %w", err)
		}
	}
	return b.EnqueueExecution(ctx, coordination.Execution{
		ID: coordinationExecutionID, CoordinationID: coordinationID, MaxAttempts: 1, Priority: priority, Spec: spec,
	})
}

// EnqueueIssueResumeExecution turns a durable wake/steer command into a new
// Coordination execution. A sleeping Agent never occupies an ExecutionWorker;
// the stable task Session is restored by NativeSessionDelivery when this new
// turn is claimed.
func (b *CoordinationBridge) EnqueueIssueResumeExecution(ctx context.Context, command boardcoordination.AgentCommand) error {
	if b == nil || b.executions == nil || b.manager == nil || b.manager.store == nil {
		return errors.New("control coordination: execution runtime is unavailable")
	}
	command.CommandID = strings.TrimSpace(command.CommandID)
	command.IssueID = strings.TrimSpace(command.IssueID)
	command.Message = strings.TrimSpace(command.Message)
	if command.CommandID == "" || command.IssueID == "" || command.Message == "" {
		return errors.New("control coordination: wake command ID, Issue ID and message are required")
	}
	if command.Delivery == "assignment" {
		// Assignment is informational; EffectEnqueueIssue owns the initial run.
		// It must never be converted into a later resume execution.
		return nil
	}
	issue, err := b.manager.store.GetIssue(command.IssueID)
	if err != nil {
		return err
	}
	if issueStatusTerminal(issue.Status) || issue.Status == "todo" {
		// Assignment delivery is redundant with the initial Issue prompt, and a
		// terminal Issue cannot be resumed.
		return nil
	}
	if command.AgentID != "" && issue.AssigneeAgentID != command.AgentID {
		return errors.New("control coordination: wake command Agent does not own Issue")
	}
	if command.TaskAgentID != "" && issue.AssigneeTaskAgentID != command.TaskAgentID {
		return errors.New("control coordination: wake command TaskAgent does not own Issue")
	}
	if issue.Status == "in_progress" && issue.ExecutionPhase == "resuming" && issue.CheckoutExecutionID == "" && strings.TrimSpace(issue.SleepToken) != "" && issue.SleepToken != command.CommandID {
		// A wake execution already owns the transition but has not started. Merge
		// later child completion/failure/budget messages into that queued prompt
		// instead of retrying until the outbox gives up.
		pendingID := "resume-" + issue.SleepToken
		if appendErr := b.executions.AppendPrompt(ctx, pendingID, command.CommandID, command.Message); appendErr != nil {
			return appendErr
		}
		b.manager.store.addEvent(issue.CurrentExecutionID, issue.ID, "coordination", "已合并新的父 Agent 唤醒消息", "排队中的恢复 Execution 将携带最新子 Issue 状态。")
		b.manager.store.notify()
		return nil
	}
	executionID := "resume-" + command.CommandID
	if _, existingErr := b.executions.Get(ctx, executionID); existingErr == nil {
		return nil
	} else if !errors.Is(existingErr, coordination.ErrExecutionNotFound) {
		return existingErr
	}
	recoveringInterruptedEnqueue := issue.Status == "in_progress" && issue.ExecutionPhase == "resuming" && issue.SleepToken == command.CommandID && issue.CheckoutExecutionID == ""
	if !recoveringInterruptedEnqueue && (issue.Status != "in_progress" || (issue.ExecutionPhase != "sleeping" && issue.ExecutionPhase != "waiting_children") || issue.CheckoutExecutionID != "") {
		return errors.New("control coordination: task-local Agent is not ready for a new turn; delivery will retry")
	}

	coordinationID := issue.ID
	if root, rootErr := b.manager.store.taskRoot(issue); rootErr == nil {
		coordinationID = root.ID
	}
	if !recoveringInterruptedEnqueue {
		updated := b.manager.store.db.Model(&Issue{}).
			Where("id = ? AND status = ? AND execution_phase = ? AND checkout_execution_id = ''", issue.ID, "in_progress", issue.ExecutionPhase).
			Updates(map[string]any{"execution_phase": "resuming", "sleep_token": command.CommandID, "updated_at": time.Now()})
		if updated.Error != nil {
			return updated.Error
		}
		if updated.RowsAffected != 1 {
			return errors.New("control coordination: Issue wake ownership changed; delivery will retry")
		}
	}
	execution := coordination.Execution{ID: executionID, CoordinationID: coordinationID, MaxAttempts: 3, Priority: coordination.ExecutionPriorityWakeup, Spec: agenthost.ExecutionSpec{
		ExecutionID: executionID, AgentID: issue.AssigneeAgentID, SessionID: "task-session-" + issue.AssigneeTaskAgentID,
		Workspace: issue.Workspace, Model: agenthost.ModelRef{Provider: "coordination", Model: "issue-resume"}, Prompt: coordinationWakePrompt(command.CommandID, command.Message),
		Values: map[string]any{
			controlIssueIDValue: issue.ID, "control.taskId": coordinationID, "control.taskAgentId": issue.AssigneeTaskAgentID,
			"control.resume": true, "control.resumePrompt": coordinationWakePrompt(command.CommandID, command.Message), "control.wakeCommandId": command.CommandID,
		},
	}}
	if err = b.executions.Enqueue(ctx, execution); err != nil && !errors.Is(err, coordination.ErrExecutionConflict) {
		if !recoveringInterruptedEnqueue {
			_ = b.manager.store.db.Model(&Issue{}).
				Where("id = ? AND status = ? AND execution_phase = ? AND sleep_token = ? AND checkout_execution_id = ''", issue.ID, "in_progress", "resuming", command.CommandID).
				Updates(map[string]any{"execution_phase": issue.ExecutionPhase, "sleep_token": issue.SleepToken, "updated_at": time.Now()}).Error
		}
		return err
	}
	b.manager.store.notify()
	return nil
}

func coordinationWakePrompt(key, message string) string {
	return fmt.Sprintf("<coordination_update id=%q>\n%s\n</coordination_update>", strings.TrimSpace(key), strings.TrimSpace(message))
}

func (b *CoordinationBridge) EnqueueExecution(ctx context.Context, execution coordination.Execution) error {
	if b == nil || b.executions == nil {
		return errors.New("control coordination: execution runtime is unavailable")
	}
	if _, err := b.executions.Get(ctx, execution.ID); err == nil {
		return nil
	} else if !errors.Is(err, coordination.ErrExecutionNotFound) {
		return err
	}
	if err := b.executions.Enqueue(ctx, execution); err != nil && !errors.Is(err, coordination.ErrExecutionConflict) {
		return err
	}
	return nil
}

func (b *CoordinationBridge) Execution(ctx context.Context, id string) (coordination.Execution, error) {
	if b == nil || b.executions == nil {
		return coordination.Execution{}, errors.New("control coordination: execution runtime is unavailable")
	}
	return b.executions.Get(ctx, id)
}

func (b *CoordinationBridge) ExecutionEvents(ctx context.Context, id string, after uint64, limit int) ([]storage.ExecutionEvent, error) {
	if b == nil || b.events == nil {
		return nil, errors.New("control coordination: execution event store is unavailable")
	}
	return b.events.ExecutionEvents(ctx, id, after, limit)
}

func (b *CoordinationBridge) CancelCoordinationExecutions(ctx context.Context, coordinationID, reason string) (int64, error) {
	if b == nil || b.executions == nil {
		return 0, errors.New("control coordination: execution runtime is unavailable")
	}
	return b.executions.CancelCoordination(ctx, coordinationID, reason)
}

func (b *CoordinationBridge) CancelExecution(ctx context.Context, executionID, reason string) error {
	if b == nil || b.executions == nil {
		return errors.New("control coordination: execution runtime is unavailable")
	}
	return b.executions.Cancel(ctx, executionID, reason)
}

func (b *CoordinationBridge) createIssue(ctx context.Context, effect coordination.Effect) error {
	var command boardcoordination.CreateIssueCommand
	if err := json.Unmarshal(effect.Payload, &command); err != nil {
		return err
	}
	requestedID := "coord-" + strings.TrimPrefix(effect.ID, "coord-effect-")
	if _, err := b.manager.store.GetIssue(requestedID); err == nil {
		return nil
	}
	issue, err := b.manager.store.CreateIssue(CreateIssueInput{RequestedID: requestedID, ParentID: command.ParentIssueID, Title: fallback(command.Child.Title, "Delegated work"), Objective: command.Child.Prompt, Description: command.Child.Prompt, Priority: fallback(command.Child.Priority, "middle"), Status: "todo", WorkMode: "autonomous", AssigneeAgentID: command.Child.AgentID, AssigneeTaskAgentID: command.Child.TaskAgentID, Capabilities: command.Child.Capabilities, CapabilitySelection: string(command.Child.CapabilitySelection), CreatedBy: fallback(command.ParentTaskAgentID, command.ParentAgentID)})
	if err != nil {
		return err
	}
	return b.SubmitIssueCreated(ctx, issue)
}

func (b *CoordinationBridge) assignIssue(ctx context.Context, effect coordination.Effect) error {
	var command boardcoordination.EnqueueIssueCommand
	if err := json.Unmarshal(effect.Payload, &command); err != nil {
		return err
	}
	issue, err := b.manager.store.UpdateIssue(command.IssueID, UpdateIssueInput{AssigneeAgentID: &command.AgentID})
	if err != nil {
		return err
	}
	return b.SubmitIssueAssigned(ctx, issue)
}

func (b *CoordinationBridge) sendRelay(ctx context.Context, effect coordination.Effect) error {
	var command boardcoordination.RelayCommand
	if err := json.Unmarshal(effect.Payload, &command); err != nil {
		return err
	}
	senderRoutingID := fallback(command.SenderTaskAgentID, fallback(command.SenderID, "system"))
	message, err := b.manager.store.SendRelayMessage("coordination", senderRoutingID, SendRelayMessageInput{RecipientID: command.RecipientID, RecipientTaskAgentID: command.RecipientTaskAgentID, Body: command.Body, IssueID: command.IssueID, TaskID: effect.CoordinationID, IdempotencyKey: effect.IdempotencyKey})
	if err != nil {
		return err
	}
	return b.SubmitRelay(ctx, message)
}

func (b *CoordinationBridge) deliverMessage(ctx context.Context, effect coordination.Effect) error {
	var command boardcoordination.AgentCommand
	if err := json.Unmarshal(effect.Payload, &command); err != nil {
		return err
	}
	command.CommandID = effect.ID
	return b.delivery.DeliverCoordinationMessage(ctx, command)
}

func (b *CoordinationBridge) suspendAgent(_ context.Context, effect coordination.Effect) error {
	var command boardcoordination.AgentCommand
	if err := json.Unmarshal(effect.Payload, &command); err != nil {
		return err
	}
	if command.IssueID == "" {
		return nil
	}
	result := b.manager.store.db.Model(&Issue{}).Where("id = ? AND status = ?", command.IssueID, "in_progress").Updates(map[string]any{"execution_phase": "waiting_children", "checkout_execution_id": "", "updated_at": time.Now()})
	return result.Error
}

func (b *CoordinationBridge) cancelWork(_ context.Context, effect coordination.Effect) error {
	var command boardcoordination.AgentCommand
	if err := json.Unmarshal(effect.Payload, &command); err != nil {
		return err
	}
	if command.IssueID == "" {
		return nil
	}
	_, err := b.manager.archiveBoardIssue(command.IssueID, fallback(command.Message, "coordination mode cancelled work"))
	return err
}

func (b *CoordinationBridge) startSubagent(ctx context.Context, effect coordination.Effect) error {
	if b.subagents == nil {
		return errors.New("control coordination: direct subagent starter is not configured")
	}
	var command boardcoordination.StartSubagentCommand
	if err := json.Unmarshal(effect.Payload, &command); err != nil {
		return err
	}
	return b.subagents.StartCoordinationSubagent(ctx, command)
}
