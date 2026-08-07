package control

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"aegis/agenthost"
	"aegis/capability"
	"aegis/coordination"
	"github.com/z3r2ne/agentcore"
)

type hostRunnerFunc func(context.Context, agenthost.ExecutionSpec, agentcore.EventSink) (agenthost.Result, error)

func (f hostRunnerFunc) Run(ctx context.Context, spec agenthost.ExecutionSpec, sink agentcore.EventSink) (agenthost.Result, error) {
	return f(ctx, spec, sink)
}

func TestNativeIssueRunnerAcceptsInternalValidationExecution(t *testing.T) {
	store := configuredStore(t)
	issue, err := store.CreateIssue(CreateIssueInput{
		Title: "Validate prepared delivery", Objective: "Verify the submitted evidence.", Priority: "high",
		WorkMode: "autonomous", AssigneeAgentID: "backend-engineer",
	})
	if err != nil {
		t.Fatal(err)
	}
	execution, _, err := store.createInternalExecution(issue, "acceptance-validator", "validation")
	if err != nil {
		t.Fatal(err)
	}
	runner := NativeIssueRunner{Manager: &Manager{store: store, sessions: map[string]*PiSession{}}}
	prepared, err := runner.prepareExistingExecution(issue, execution.ID, "Validate the delivery.")
	if err != nil {
		t.Fatalf("internal validation execution was rejected: %v", err)
	}
	if prepared.execution.ID != execution.ID || prepared.agent.ID != "acceptance-validator" || !prepared.agent.Internal {
		t.Fatalf("unexpected prepared validation: %+v", prepared)
	}
}

func TestNativeIssueRunnerReconcilesPreparedValidationStartupFailure(t *testing.T) {
	store := configuredStore(t)
	issue, err := store.CreateIssue(CreateIssueInput{
		Title: "Failed validation startup", Objective: "Verify the submitted evidence.", Priority: "high",
		WorkMode: "autonomous", AssigneeAgentID: "backend-engineer",
	})
	if err != nil {
		t.Fatal(err)
	}
	execution, _, err := store.createInternalExecution(issue, "acceptance-validator", "validation")
	if err != nil {
		t.Fatal(err)
	}
	if err = store.updateExecution(execution.ID, map[string]any{"status": "starting"}); err != nil {
		t.Fatal(err)
	}
	if err = store.db.Model(&Issue{}).Where("id = ?", issue.ID).Updates(map[string]any{
		"status": "in_progress", "execution_phase": "validating", "current_execution_id": execution.ID,
		"validation_execution_id": execution.ID,
	}).Error; err != nil {
		t.Fatal(err)
	}
	validation := IssueValidation{
		ID: nextID("validation"), IssueID: issue.ID, SourceExecutionID: "source-execution",
		ValidationExecutionID: execution.ID, Attempt: 1, Objective: issue.Objective,
		CandidateResult: "candidate", Status: "running", CreatedAt: time.Now(),
	}
	if err = store.db.Create(&validation).Error; err != nil {
		t.Fatal(err)
	}
	manager := &Manager{store: store, sessions: map[string]*PiSession{}}
	host := hostRunnerFunc(func(context.Context, agenthost.ExecutionSpec, agentcore.EventSink) (agenthost.Result, error) {
		t.Fatal("AgentHost must not run when the prepared prompt is invalid")
		return agenthost.Result{}, nil
	})
	runner := NativeIssueRunner{Manager: manager, Host: host}
	_, runErr := runner.Run(context.Background(), agenthost.ExecutionSpec{
		ExecutionID: "prepared-invalid-validation",
		Values: map[string]any{
			controlIssueIDValue:             issue.ID,
			controlPreparedExecutionIDValue: execution.ID,
			controlPreparedPromptValue:      "",
		},
	}, nil)
	if runErr == nil {
		t.Fatal("invalid prepared validation unexpectedly started")
	}
	if err = store.db.First(&execution, "id = ?", execution.ID).Error; err != nil || execution.Status != "failed" {
		t.Fatalf("prepared Execution was not failed: err=%v execution=%+v", err, execution)
	}
	if err = store.db.First(&validation, "id = ?", validation.ID).Error; err != nil || validation.Status != "error" || validation.CompletedAt == nil {
		t.Fatalf("active validation was not reconciled: err=%v validation=%+v", err, validation)
	}
	failedIssue, err := store.GetIssue(issue.ID)
	if err != nil || failedIssue.Status != "done" || !hasIssueLabel(failedIssue, issueLabelFailed) || failedIssue.ExecutionPhase != "completed" {
		t.Fatalf("Issue remained stuck after startup failure: err=%v issue=%+v", err, failedIssue)
	}
}

func TestNativeIssueRunnerCompletesWorkThroughAgentHost(t *testing.T) {
	store, manager := bridgeTestManager(t)
	bridge, err := newTestCoordinationBridge(manager, "native-runner", "board_autonomy", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	manager.SetCoordination(bridge)
	t.Cleanup(func() { bridge.Close(); manager.SetCoordination(nil) })
	issue, err := store.CreateIssue(CreateIssueInput{
		Title: "Native work", Objective: "Return native delivery", Priority: "medium", WorkMode: "autonomous", AssigneeAgentID: "backend-engineer",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.db.Model(&Issue{}).Where("id = ?", issue.ID).Updates(map[string]any{
		"container_profile_id": "", "container_id": "", "validation_disabled": true,
	}).Error; err != nil {
		t.Fatal(err)
	}
	hostCalls := 0
	host := hostRunnerFunc(func(ctx context.Context, spec agenthost.ExecutionSpec, sink agentcore.EventSink) (agenthost.Result, error) {
		hostCalls++
		if !strings.HasPrefix(spec.ExecutionID, "execution-") || spec.AgentID != "backend-engineer" || spec.Model.Provider != "test" || spec.Model.Model != "test-model" || len(spec.Capabilities) != 5 || spec.Capabilities[3].Name != "workspace" || spec.Capabilities[4].Name != "delivery" {
			t.Fatalf("spec = %+v", spec)
		}
		if sink != nil {
			if err := sink(ctx, agentcore.Event{Type: agentcore.EventToolExecutionStart, ToolName: "native_tool", Arguments: []byte(`{}`)}); err != nil {
				return agenthost.Result{}, err
			}
			if err := sink(ctx, agentcore.Event{Type: agentcore.EventToolExecutionEnd, ToolName: "native_tool", Success: true}); err != nil {
				return agenthost.Result{}, err
			}
		}
		messages := []agentcore.Message{
			agentcore.TextMessage(agentcore.RoleUser, spec.Prompt),
			agentcore.TextMessage(agentcore.RoleAssistant, "native delivery"),
		}
		return agenthost.Result{Core: agentcore.Result{
			State: agentcore.State{Messages: messages}, NewMessages: messages, StopReason: agentcore.StopReasonStop,
			Usage: agentcore.Usage{InputTokens: 10, OutputTokens: 4},
		}}, nil
	})
	runner := NativeIssueRunner{Manager: manager, Host: host}
	result, err := runner.Run(context.Background(), agenthost.ExecutionSpec{ExecutionID: issue.ID, Values: map[string]any{controlIssueIDValue: issue.ID}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if hostCalls != 1 || lastAssistantText(result.Core.State.Messages) != "native delivery" {
		t.Fatalf("hostCalls=%d result=%+v", hostCalls, result)
	}
	completed, err := store.GetIssue(issue.ID)
	if err != nil {
		t.Fatal(err)
	}
	if completed.Status != "done" || completed.Result != "native delivery" || completed.ExecutionPhase != "completed" {
		t.Fatalf("issue = %+v", completed)
	}
	var execution Execution
	if err := store.db.Where("issue_id = ?", issue.ID).Order("started_at desc").First(&execution).Error; err != nil {
		t.Fatal(err)
	}
	if execution.RuntimeType != "agentcore" || execution.Status != "completed" || execution.InputTokens != 10 || execution.OutputTokens != 4 || execution.MessageCount != 2 {
		t.Fatalf("execution = %+v", execution)
	}
	var messages []Message
	if err := store.db.Where("execution_id = ?", execution.ID).Order("created_at asc").Find(&messages).Error; err != nil {
		t.Fatal(err)
	}
	if len(messages) != 2 || messages[0].Role != "user" || messages[1].Content != "native delivery" {
		t.Fatalf("messages = %+v", messages)
	}
}

func TestNativeIssueRunnerTreatsTerminatedSleepAsSuspension(t *testing.T) {
	store, manager := bridgeTestManager(t)
	bridge, err := newTestCoordinationBridge(manager, "native-sleep", "board_autonomy", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	manager.SetCoordination(bridge)
	t.Cleanup(func() { bridge.Close(); manager.SetCoordination(nil) })
	issue, err := store.CreateIssue(CreateIssueInput{Title: "Sleep", Objective: "resume later", Priority: "medium", WorkMode: "autonomous", AssigneeAgentID: "backend-engineer"})
	if err != nil {
		t.Fatal(err)
	}
	if err = store.db.Model(&Issue{}).Where("id = ?", issue.ID).Updates(map[string]any{"container_profile_id": "", "container_id": "", "validation_disabled": true}).Error; err != nil {
		t.Fatal(err)
	}
	host := hostRunnerFunc(func(_ context.Context, spec agenthost.ExecutionSpec, _ agentcore.EventSink) (agenthost.Result, error) {
		if err := store.db.Model(&Issue{}).Where("id = ?", issue.ID).Updates(map[string]any{
			"execution_phase": "sleeping", "sleep_token": "sleep-token", "checkout_execution_id": "",
		}).Error; err != nil {
			return agenthost.Result{}, err
		}
		message := agentcore.TextMessage(agentcore.RoleAssistant, "sleep requested")
		return agenthost.Result{Core: agentcore.Result{State: agentcore.State{Messages: []agentcore.Message{message}}, NewMessages: []agentcore.Message{message}, StopReason: agentcore.StopReasonTerminated}}, nil
	})
	runner := NativeIssueRunner{Manager: manager, Host: host}
	result, err := runner.Run(context.Background(), agenthost.ExecutionSpec{Values: map[string]any{controlIssueIDValue: issue.ID}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if result.Core.StopReason != agentcore.StopReasonTerminated {
		t.Fatalf("stop reason=%s", result.Core.StopReason)
	}
	current, err := store.GetIssue(issue.ID)
	if err != nil {
		t.Fatal(err)
	}
	if current.Status != "in_progress" || current.ExecutionPhase != "sleeping" || current.CheckoutExecutionID != "" || current.SleepToken != "sleep-token" {
		t.Fatalf("sleeping Issue was completed or kept checked out: %+v", current)
	}
	var execution Execution
	if err = store.db.Where("issue_id = ?", issue.ID).Order("started_at desc").First(&execution).Error; err != nil {
		t.Fatal(err)
	}
	if execution.Status != "completed" {
		t.Fatalf("sleep turn execution=%+v", execution)
	}
}

func TestNativeIssueRunnerRecoversCancelledWorkerContextWithoutFailingIssue(t *testing.T) {
	store, manager := bridgeTestManager(t)
	bridge, err := newTestCoordinationBridge(manager, "native-recovery", "board_autonomy", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	manager.SetCoordination(bridge)
	t.Cleanup(func() { bridge.Close(); manager.SetCoordination(nil) })
	issue, err := store.CreateIssue(CreateIssueInput{
		Title: "Recover native work", Objective: "Continue after worker restart", Priority: "medium", WorkMode: "autonomous", AssigneeAgentID: "backend-engineer",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err = store.db.Model(&Issue{}).Where("id = ?", issue.ID).Updates(map[string]any{
		"container_profile_id": "", "container_id": "", "validation_disabled": true,
	}).Error; err != nil {
		t.Fatal(err)
	}

	started := make(chan struct{})
	var calls atomic.Int32
	var firstExecutionID, firstSessionID string
	host := hostRunnerFunc(func(ctx context.Context, spec agenthost.ExecutionSpec, _ agentcore.EventSink) (agenthost.Result, error) {
		if calls.Add(1) == 1 {
			firstExecutionID, firstSessionID = spec.ExecutionID, spec.SessionID
			close(started)
			<-ctx.Done()
			return agenthost.Result{}, ctx.Err()
		}
		if spec.ExecutionID != firstExecutionID || spec.SessionID != firstSessionID {
			t.Fatalf("recovery changed execution/session: first=%s/%s recovered=%s/%s", firstExecutionID, firstSessionID, spec.ExecutionID, spec.SessionID)
		}
		if !strings.Contains(spec.Prompt, "service restarted") && !strings.Contains(spec.Prompt, "服务") {
			t.Fatalf("recovery prompt=%q", spec.Prompt)
		}
		message := agentcore.TextMessage(agentcore.RoleAssistant, "recovered delivery")
		return agenthost.Result{Core: agentcore.Result{State: agentcore.State{Messages: []agentcore.Message{message}}, NewMessages: []agentcore.Message{message}, StopReason: agentcore.StopReasonStop}}, nil
	})
	runner := NativeIssueRunner{Manager: manager, Host: host}
	scheduled := agenthost.ExecutionSpec{ExecutionID: issue.ID, Values: map[string]any{controlIssueIDValue: issue.ID}}
	ctx, cancel := context.WithCancel(context.Background())
	firstDone := make(chan error, 1)
	go func() {
		_, runErr := runner.Run(ctx, scheduled, nil)
		firstDone <- runErr
	}()
	<-started
	cancel()
	if runErr := <-firstDone; !errors.Is(runErr, context.Canceled) {
		t.Fatalf("cancelled run error=%v", runErr)
	}

	interrupted, err := store.GetIssue(issue.ID)
	if err != nil {
		t.Fatal(err)
	}
	if interrupted.Status != "todo" || interrupted.ExecutionPhase != "recovering" || interrupted.RecoveryExecutionID != firstExecutionID || interrupted.CheckoutExecutionID != "" {
		t.Fatalf("Issue was failed instead of suspended: %+v", interrupted)
	}
	var execution Execution
	if err = store.db.First(&execution, "id = ?", firstExecutionID).Error; err != nil {
		t.Fatal(err)
	}
	if execution.Status != "disconnected" || execution.Error != "" || execution.RuntimeType != "agentcore" {
		t.Fatalf("interrupted execution=%+v", execution)
	}

	reclaimedPrepared := scheduled
	reclaimedPrepared.Values = map[string]any{
		controlIssueIDValue:             issue.ID,
		controlPreparedExecutionIDValue: firstExecutionID,
		controlPreparedPromptValue:      "stale prompt from before restart",
	}
	if _, err = runner.Run(context.Background(), reclaimedPrepared, nil); err != nil {
		t.Fatal(err)
	}
	completed, err := store.GetIssue(issue.ID)
	if err != nil {
		t.Fatal(err)
	}
	if completed.Status != "done" || completed.Result != "recovered delivery" || calls.Load() != 2 {
		t.Fatalf("native recovery did not complete: calls=%d issue=%+v", calls.Load(), completed)
	}
	var count int64
	if err = store.db.Model(&Execution{}).Where("issue_id = ?", issue.ID).Count(&count).Error; err != nil || count != 1 {
		t.Fatalf("recovery created a replacement execution: count=%d err=%v", count, err)
	}
}

func TestNativeIssueRunnerReleasesParentWhileChildrenRun(t *testing.T) {
	store, manager := bridgeTestManager(t)
	bridge, err := newTestCoordinationBridge(manager, "native-child-wait", "board_autonomy", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	manager.SetCoordination(bridge)
	t.Cleanup(func() { bridge.Close(); manager.SetCoordination(nil) })
	parent, err := store.CreateIssue(CreateIssueInput{Title: "Parent", Objective: "integrate children", Priority: "medium", WorkMode: "autonomous", AssigneeAgentID: "backend-engineer"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.CreateIssue(CreateIssueInput{ParentID: parent.ID, Title: "Child", Objective: "work", Priority: "medium", WorkMode: "autonomous", AssigneeAgentID: "frontend-engineer"}); err != nil {
		t.Fatal(err)
	}
	if err = store.db.Model(&Issue{}).Where("id = ?", parent.ID).Updates(map[string]any{"container_profile_id": "", "container_id": "", "validation_disabled": true}).Error; err != nil {
		t.Fatal(err)
	}
	host := hostRunnerFunc(func(_ context.Context, _ agenthost.ExecutionSpec, _ agentcore.EventSink) (agenthost.Result, error) {
		message := agentcore.TextMessage(agentcore.RoleAssistant, "parent work is exhausted until child returns")
		return agenthost.Result{Core: agentcore.Result{State: agentcore.State{Messages: []agentcore.Message{message}}, NewMessages: []agentcore.Message{message}, StopReason: agentcore.StopReasonStop}}, nil
	})
	runner := NativeIssueRunner{Manager: manager, Host: host}
	if _, err = runner.Run(context.Background(), agenthost.ExecutionSpec{Values: map[string]any{controlIssueIDValue: parent.ID}}, nil); err != nil {
		t.Fatal(err)
	}
	current, err := store.GetIssue(parent.ID)
	if err != nil {
		t.Fatal(err)
	}
	if current.Status != "in_progress" || current.ExecutionPhase != "waiting_children" || current.CheckoutExecutionID != "" {
		t.Fatalf("parent did not release its worker while children run: %+v", current)
	}
}

func TestDurableSleepStartsNewCoordinationExecutionOnWake(t *testing.T) {
	store, manager := bridgeTestManager(t)
	issue, err := store.CreateIssue(CreateIssueInput{Title: "Durable sleep", Objective: "wake in a new turn", Priority: "medium", WorkMode: "autonomous", AssigneeAgentID: "backend-engineer"})
	if err != nil {
		t.Fatal(err)
	}
	if err = store.db.Model(&Issue{}).Where("id = ?", issue.ID).Updates(map[string]any{"container_profile_id": "", "container_id": "", "validation_disabled": true}).Error; err != nil {
		t.Fatal(err)
	}
	var calls atomic.Int32
	firstReturned := make(chan struct{})
	host := hostRunnerFunc(func(ctx context.Context, spec agenthost.ExecutionSpec, _ agentcore.EventSink) (agenthost.Result, error) {
		call := calls.Add(1)
		if call == 1 {
			client := ManagerBoardCoordinator{Manager: manager}
			err := client.Wait(ctx, coordination.Invocation{EventID: "durable-sleep-event", IssueID: issue.ID, ExecutionID: spec.ExecutionID, AgentID: spec.AgentID}, coordination.WaitRequest{WakeAfterSeconds: 1, Message: "durable timer wake"})
			if err != nil {
				return agenthost.Result{}, err
			}
			close(firstReturned)
			message := agentcore.TextMessage(agentcore.RoleAssistant, "sleeping")
			return agenthost.Result{Core: agentcore.Result{State: agentcore.State{Messages: []agentcore.Message{message}}, NewMessages: []agentcore.Message{message}, StopReason: agentcore.StopReasonTerminated}}, nil
		}
		if !strings.Contains(spec.Prompt, "durable timer wake") {
			t.Fatalf("resume prompt=%q", spec.Prompt)
		}
		message := agentcore.TextMessage(agentcore.RoleAssistant, "completed after durable wake")
		return agenthost.Result{Core: agentcore.Result{State: agentcore.State{Messages: []agentcore.Message{message}}, NewMessages: []agentcore.Message{message}, StopReason: agentcore.StopReasonStop}}, nil
	})
	nativeDelivery := NewNativeSessionDelivery(manager)
	runner := NativeIssueRunner{Manager: manager, Host: host}
	bridge, err := NewCoordinationBridge(manager, CoordinationBridgeOptions{WorkerID: "durable-sleep", DefaultMode: "board_autonomy", Delivery: nativeDelivery, Executor: runner})
	if err != nil {
		t.Fatal(err)
	}
	bridge.runtime.Poll = 5 * time.Millisecond
	manager.SetCoordination(bridge)
	ctx, cancel := context.WithCancel(context.Background())
	bridge.Start(ctx)
	t.Cleanup(func() { cancel(); bridge.Close(); manager.SetCoordination(nil) })
	if err = manager.DispatchIssue(issue.ID); err != nil {
		t.Fatal(err)
	}
	select {
	case <-firstReturned:
	case <-time.After(2 * time.Second):
		t.Fatal("initial turn did not enter durable sleep")
	}
	select {
	case <-time.After(300 * time.Millisecond):
		if calls.Load() != 1 {
			t.Fatalf("model was polled while sleeping: calls=%d", calls.Load())
		}
	}
	deadline := time.Now().Add(4 * time.Second)
	for time.Now().Before(deadline) {
		current, _ := store.GetIssue(issue.ID)
		if current.Status == "done" {
			if calls.Load() != 2 || current.Result != "completed after durable wake" {
				t.Fatalf("calls=%d issue=%+v", calls.Load(), current)
			}
			var executions []Execution
			if err = store.db.Where("issue_id = ?", issue.ID).Order("started_at asc").Find(&executions).Error; err != nil {
				t.Fatal(err)
			}
			if len(executions) != 2 || executions[0].Kind != "work" || executions[1].Kind != "wakeup" || executions[0].ID == executions[1].ID {
				t.Fatalf("executions=%+v", executions)
			}
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	current, _ := store.GetIssue(issue.ID)
	var executions []Execution
	_ = store.db.Where("issue_id = ?", issue.ID).Order("started_at asc").Find(&executions).Error
	t.Fatalf("durable wake did not finish: calls=%d issue=%+v executions=%+v", calls.Load(), current, executions)
}

func TestNativeIssueRunnerUsesCoordinationCapabilityPolicy(t *testing.T) {
	store, manager := bridgeTestManager(t)
	bridge, err := newTestCoordinationBridge(manager, "capability-policy", "board_autonomy", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	manager.SetCoordination(bridge)
	t.Cleanup(func() { bridge.Close(); manager.SetCoordination(nil) })
	issue, err := store.CreateIssue(CreateIssueInput{
		Title: "Policy controlled work", Objective: "use only selected software", Priority: "medium", WorkMode: "autonomous", AssigneeAgentID: "backend-engineer",
		CapabilitySelection: "replace", Capabilities: []capability.Ref{{Kind: capability.KindPhone, Name: "default", Config: map[string]any{"installedApps": []any{"aegis.board"}}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err = store.db.Model(&Issue{}).Where("id = ?", issue.ID).Updates(map[string]any{"container_profile_id": "", "container_id": "", "validation_disabled": true}).Error; err != nil {
		t.Fatal(err)
	}
	policy := json.RawMessage(`{"capabilityPolicy":{"allowed":[{"kind":"phone","name":"default"},{"kind":"tool","name":"workspace"},{"kind":"tool","name":"delivery"}]}}`)
	if _, err = bridge.BindIssue(context.Background(), issue.ID, "board_autonomy", "1", policy); err != nil {
		t.Fatal(err)
	}
	registry := capability.NewRegistry()
	emptySource := capability.SourceFunc(func(context.Context, capability.ResolveContext, capability.Ref) (capability.Resolved, error) {
		return capability.Resolved{}, nil
	})
	if err = registry.Register(capability.KindPhone, "default", emptySource); err != nil {
		t.Fatal(err)
	}
	if err = registry.Register(capability.KindTool, "workspace", emptySource); err != nil {
		t.Fatal(err)
	}
	if err = registry.Register(capability.KindTool, "delivery", emptySource); err != nil {
		t.Fatal(err)
	}
	planner := &coordination.CapabilityPlanner{Catalog: registry}
	bridge.SetCapabilityPlanner(planner, false)
	host := hostRunnerFunc(func(_ context.Context, spec agenthost.ExecutionSpec, _ agentcore.EventSink) (agenthost.Result, error) {
		if len(spec.Capabilities) != 3 || spec.Capabilities[0].Kind != capability.KindPhone || spec.Capabilities[1].Name != "workspace" || spec.Capabilities[2].Name != "delivery" {
			t.Fatalf("planned capabilities=%+v", spec.Capabilities)
		}
		decision, ok := spec.Values["coordination.capabilityDecision"].(coordination.CapabilityDecision)
		if !ok || decision.Selection != coordination.CapabilityReplace {
			t.Fatalf("decision=%#v", spec.Values["coordination.capabilityDecision"])
		}
		message := agentcore.TextMessage(agentcore.RoleAssistant, "policy-complete")
		return agenthost.Result{
			Core: agentcore.Result{State: agentcore.State{Messages: []agentcore.Message{message}}, NewMessages: []agentcore.Message{message}, StopReason: agentcore.StopReasonStop},
			Capabilities: []capability.Snapshot{
				{Kind: capability.KindPhone, Name: "default", Version: "phone-v1", Metadata: map[string]string{"apps": "aegis.board"}},
			},
		}, nil
	})
	runner := NativeIssueRunner{Manager: manager, Host: host}
	if _, err = runner.Run(context.Background(), agenthost.ExecutionSpec{Values: map[string]any{controlIssueIDValue: issue.ID}}, nil); err != nil {
		t.Fatal(err)
	}
	var execution Execution
	if err = store.db.Where("issue_id = ?", issue.ID).Order("started_at desc").First(&execution).Error; err != nil {
		t.Fatal(err)
	}
	if len(execution.CapabilitiesSnapshot) != 1 || execution.CapabilitiesSnapshot[0].Metadata["apps"] != "aegis.board" {
		t.Fatalf("capability snapshot=%+v", execution.CapabilitiesSnapshot)
	}
}

func TestNativeIssueRunnerInjectsGoalPreservingPolicyIntoLeaderContext(t *testing.T) {
	store, manager := bridgeTestManager(t)
	bridge, err := newTestCoordinationBridge(manager, "leader-goal-policy", "board_autonomy", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	manager.SetCoordination(bridge)
	t.Cleanup(func() { bridge.Close(); manager.SetCoordination(nil) })
	issue, err := store.CreateIssue(CreateIssueInput{
		Title: "Plan several independent outcomes", Objective: "Deliver every requested outcome at full depth.",
		Priority: "high", WorkMode: "autonomous", AssigneeAgentID: "development-lead",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err = store.db.Model(&Issue{}).Where("id = ?", issue.ID).Updates(map[string]any{
		"container_profile_id": "", "container_id": "", "validation_disabled": true,
	}).Error; err != nil {
		t.Fatal(err)
	}
	host := hostRunnerFunc(func(_ context.Context, spec agenthost.ExecutionSpec, _ agentcore.EventSink) (agenthost.Result, error) {
		for _, required := range []string{
			"<execution_efficiency>",
			"<aegis_organization_context>",
			`"agentId":"red-team-lead"`,
			`"decompositionLeader":true`,
			"Capacity is finite",
			"<goal_preserving_decomposition>",
			"one distinct direct child Issue for every goal",
			"do not create a redundant child that merely restates that goal",
		} {
			if !strings.Contains(spec.SystemPrompt, required) {
				t.Fatalf("Leader runtime system prompt is missing %q: %s", required, spec.SystemPrompt)
			}
		}
		message := agentcore.TextMessage(agentcore.RoleAssistant, "leader policy observed")
		return agenthost.Result{Core: agentcore.Result{
			State: agentcore.State{Messages: []agentcore.Message{message}}, NewMessages: []agentcore.Message{message}, StopReason: agentcore.StopReasonStop,
		}}, nil
	})
	if _, err = (NativeIssueRunner{Manager: manager, Host: host}).Run(context.Background(), agenthost.ExecutionSpec{Values: map[string]any{controlIssueIDValue: issue.ID}}, nil); err != nil {
		t.Fatal(err)
	}
}

func TestCoordinationExecutionRunsNativeIssueRunner(t *testing.T) {
	store, manager := bridgeTestManager(t)
	issue, err := store.CreateIssue(CreateIssueInput{Title: "Native scheduled", Objective: "Done", Priority: "medium", WorkMode: "autonomous", AssigneeAgentID: "backend-engineer"})
	if err != nil {
		t.Fatal(err)
	}
	_ = store.db.Model(&Issue{}).Where("id = ?", issue.ID).Updates(map[string]any{"container_profile_id": "", "container_id": "", "validation_disabled": true}).Error
	host := hostRunnerFunc(func(_ context.Context, spec agenthost.ExecutionSpec, _ agentcore.EventSink) (agenthost.Result, error) {
		message := agentcore.TextMessage(agentcore.RoleAssistant, "scheduled native result")
		return agenthost.Result{Core: agentcore.Result{State: agentcore.State{Messages: []agentcore.Message{message}}, NewMessages: []agentcore.Message{message}, StopReason: agentcore.StopReasonStop}}, nil
	})
	native := NativeIssueRunner{Manager: manager, Host: host}
	bridge, err := NewCoordinationBridge(manager, CoordinationBridgeOptions{WorkerID: "native-worker", DefaultMode: "board_autonomy", Executor: native})
	if err != nil {
		t.Fatal(err)
	}
	manager.SetCoordination(bridge)
	ctx, cancel := context.WithCancel(context.Background())
	bridge.Start(ctx)
	t.Cleanup(func() { cancel(); bridge.Close(); manager.SetCoordination(nil) })
	if err := manager.DispatchIssue(issue.ID); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		execution, getErr := bridge.Execution(context.Background(), issue.ID)
		if getErr == nil && execution.Status == coordination.ExecutionSucceeded {
			completed, _ := store.GetIssue(issue.ID)
			if completed.Status != "done" || completed.Result != "scheduled native result" {
				t.Fatalf("issue = %+v", completed)
			}
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	execution, _ := bridge.Execution(context.Background(), issue.ID)
	t.Fatalf("Coordination execution did not complete: %+v", execution)
}

func TestResumePromptIncludesFailedAndBudgetExceededChildrenThatArrivedWhileQueued(t *testing.T) {
	store, manager := bridgeTestManager(t)
	parent, _ := store.CreateIssue(CreateIssueInput{Title: "Parent", Priority: "high", WorkMode: "autonomous", AssigneeAgentID: "backend-engineer"})
	failed, _ := store.CreateIssue(CreateIssueInput{ParentID: parent.ID, Title: "Failed child", Priority: "medium", WorkMode: "autonomous", AssigneeAgentID: "frontend-engineer"})
	exceeded, _ := store.CreateIssue(CreateIssueInput{ParentID: parent.ID, Title: "Budget child", Priority: "medium", WorkMode: "autonomous", AssigneeAgentID: "red-team-engineer"})
	if err := store.db.Model(&Issue{}).Where("id = ?", failed.ID).Updates(map[string]any{"status": "done", "labels": issueLabelsColumn([]string{"failed"}), "execution_phase": "completed", "error": "connection failed"}).Error; err != nil {
		t.Fatal(err)
	}
	if err := store.db.Model(&Issue{}).Where("id = ?", exceeded.ID).Updates(map[string]any{"status": "done", "labels": issueLabelsColumn([]string{"budget_exceeded"}), "execution_phase": "budget_exceeded", "result": "partial audit evidence"}).Error; err != nil {
		t.Fatal(err)
	}
	prompt := (NativeIssueRunner{Manager: manager}).terminalChildAttentionPrompt(parent.ID)
	for _, expected := range []string{"必须处理", failed.Identifier, "connection failed", exceeded.Identifier, "partial audit evidence", "phone_board_continue_issue"} {
		if !strings.Contains(prompt, expected) {
			t.Fatalf("attention prompt missing %q: %s", expected, prompt)
		}
	}
}
