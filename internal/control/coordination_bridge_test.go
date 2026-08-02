package control

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"aegis/agenthost"
	"aegis/capability"
	"aegis/coordination"
	"github.com/z3r2ne/agentcore"
)

func TestFailedChildImmediatelyWakesWaitingParentThroughCoordination(t *testing.T) {
	store, manager := bridgeTestManager(t)
	bridge, err := newTestCoordinationBridge(manager, "failed-child-wakeup", "board_autonomy", &coordinationDeliveryRecorder{notify: make(chan struct{}, 2)}, nil)
	if err != nil {
		t.Fatal(err)
	}
	manager.SetCoordination(bridge)
	t.Cleanup(func() { bridge.Close(); manager.SetCoordination(nil) })

	parent, _ := store.CreateIssue(CreateIssueInput{Title: "Parent", Priority: "high", WorkMode: "autonomous", AssigneeAgentID: "backend-engineer"})
	failedChild, _ := store.CreateIssue(CreateIssueInput{ParentID: parent.ID, Title: "Fragile child", Priority: "high", WorkMode: "autonomous", AssigneeAgentID: "frontend-engineer"})
	runningChild, _ := store.CreateIssue(CreateIssueInput{ParentID: parent.ID, Title: "Still running", Priority: "medium", WorkMode: "autonomous", AssigneeAgentID: "red-team-engineer"})
	parentExecution, _ := store.createExecution(parent, parent.AssigneeAgentID, "work")
	failedExecution, _ := store.createExecution(failedChild, failedChild.AssigneeAgentID, "work")
	anchor := time.Now().Add(-time.Second)
	_ = store.updateExecution(parentExecution.ID, map[string]any{"status": "completed", "finished_at": anchor})
	_ = store.db.Model(&Issue{}).Where("id = ?", parent.ID).Updates(map[string]any{
		"status": "in_progress", "execution_phase": "waiting_children", "checkout_execution_id": "",
		"current_execution_id": parentExecution.ID, "updated_at": anchor,
	}).Error
	_ = store.db.Model(&Issue{}).Where("id = ?", runningChild.ID).Updates(map[string]any{"status": "in_progress", "execution_phase": "active"}).Error
	failedChild, _ = store.CheckoutIssue(failedChild.ID, CheckoutIssueInput{AgentID: failedChild.AssigneeAgentID, ExecutionID: failedExecution.ID, ExpectedStatuses: []string{"todo"}})
	manager.failExecution(failedChild, failedExecution, errors.New("dependency download failed"))

	deadline := time.Now().Add(2 * time.Second)
	var resumed Issue
	for time.Now().Before(deadline) {
		resumed, _ = store.GetIssue(parent.ID)
		if resumed.ExecutionPhase == "resuming" && resumed.SleepToken != "" {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if resumed.ExecutionPhase != "resuming" || resumed.SleepToken == "" {
		t.Fatalf("parent remained waiting after child failure: %+v", resumed)
	}
	queued, err := bridge.Execution(context.Background(), "resume-"+resumed.SleepToken)
	if err != nil || queued.Status != coordination.ExecutionQueued {
		t.Fatalf("resume execution=%+v err=%v", queued, err)
	}
	if !strings.Contains(queued.Spec.Prompt, "dependency download failed") || !strings.Contains(queued.Spec.Prompt, "coordinate_continue") {
		t.Fatalf("failure recovery context missing from wake prompt: %q", queued.Spec.Prompt)
	}
	currentSibling, _ := store.GetIssue(runningChild.ID)
	if currentSibling.Status != "in_progress" {
		t.Fatalf("unrelated running child was disturbed: %+v", currentSibling)
	}
}

type coordinationDeliveryRecorder struct {
	mu       sync.Mutex
	commands []coordination.AgentCommand
	notify   chan struct{}
}

func (r *coordinationDeliveryRecorder) DeliverCoordinationMessage(_ context.Context, command coordination.AgentCommand) error {
	r.mu.Lock()
	r.commands = append(r.commands, command)
	r.mu.Unlock()
	select {
	case r.notify <- struct{}{}:
	default:
	}
	return nil
}

type coordinationSubagentRecorder struct {
	commands chan coordination.StartSubagentCommand
}

type coordinationFixedHost struct{}

func (coordinationFixedHost) Run(_ context.Context, spec agenthost.ExecutionSpec, sink agentcore.EventSink) (agenthost.Result, error) {
	if sink != nil {
		_ = sink(context.Background(), agentcore.Event{Type: agentcore.EventAgentStart})
	}
	message := agentcore.TextMessage(agentcore.RoleAssistant, "direct child result for "+spec.Prompt)
	return agenthost.Result{Core: agentcore.Result{State: agentcore.State{Messages: []agentcore.Message{message}}, NewMessages: []agentcore.Message{message}, StopReason: agentcore.StopReasonStop}}, nil
}

func newTestCoordinationBridge(manager *Manager, workerID, mode string, delivery CoordinationDelivery, subagents CoordinationSubagentStarter) (*CoordinationBridge, error) {
	return NewCoordinationBridge(manager, CoordinationBridgeOptions{
		WorkerID: workerID, DefaultMode: mode, Delivery: delivery, Subagents: subagents,
		Executor: coordinationFixedHost{},
	})
}

func (r coordinationSubagentRecorder) StartCoordinationSubagent(_ context.Context, command coordination.StartSubagentCommand) error {
	r.commands <- command
	return nil
}

func TestBoardAutonomyDispatchesAssignedIssueWithoutSuspendingParent(t *testing.T) {
	store, manager := bridgeTestManager(t)
	delivery := &coordinationDeliveryRecorder{notify: make(chan struct{}, 10)}
	bridge, err := newTestCoordinationBridge(manager, "coord-test", "board_autonomy", delivery, nil)
	if err != nil {
		t.Fatal(err)
	}
	manager.SetCoordination(bridge)
	ctx, cancel := context.WithCancel(context.Background())
	bridge.Start(ctx)
	t.Cleanup(func() { cancel(); bridge.Close(); manager.SetCoordination(nil) })

	issue, err := manager.CreateIssue(CreateIssueInput{Title: "Async Board work", Objective: "Run independently", Priority: "medium", WorkMode: "autonomous", AssigneeAgentID: "backend-engineer"})
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if execution, getErr := bridge.Execution(context.Background(), issue.ID); getErr == nil && execution.ID == issue.ID {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if _, getErr := bridge.Execution(context.Background(), issue.ID); getErr != nil {
		t.Fatal("assigned Board Issue did not create a Coordination execution")
	}
	current, _ := store.GetIssue(issue.ID)
	if current.ExecutionPhase == "waiting_children" {
		t.Fatalf("board autonomy suspended work: %+v", current)
	}
	select {
	case <-delivery.notify:
	case <-time.After(2 * time.Second):
		t.Fatal("assignment Relay was not delivered through coordination mode")
	}
}

func TestPublicDispatchCreatesCoordinationOwnedExecution(t *testing.T) {
	store, manager := bridgeTestManager(t)
	issue, err := manager.CreateIssue(CreateIssueInput{Title: "Gateway work", Objective: "route centrally", Priority: "medium", Status: "backlog", WorkMode: "autonomous"})
	if err != nil {
		t.Fatal(err)
	}
	agentID := "backend-engineer"
	if _, err = store.UpdateIssue(issue.ID, UpdateIssueInput{AssigneeAgentID: &agentID}); err != nil {
		t.Fatal(err)
	}
	bridge, err := newTestCoordinationBridge(manager, "coord-gateway", "board_autonomy", &coordinationDeliveryRecorder{notify: make(chan struct{}, 2)}, nil)
	if err != nil {
		t.Fatal(err)
	}
	manager.SetCoordination(bridge)
	ctx, cancel := context.WithCancel(context.Background())
	bridge.Start(ctx)
	t.Cleanup(func() { cancel(); bridge.Close(); manager.SetCoordination(nil) })

	if err = manager.DispatchIssue(issue.ID); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		execution, getErr := bridge.Execution(context.Background(), issue.ID)
		if getErr == nil && execution.ID == issue.ID {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("Coordination did not create the durable execution")
}

func TestPublicDispatchReservesIssueAndDeduplicatesBeforeDecisionWorker(t *testing.T) {
	store, manager := bridgeTestManager(t)
	bridge, err := newTestCoordinationBridge(manager, "coord-dispatch-dedup", "board_autonomy", &coordinationDeliveryRecorder{notify: make(chan struct{}, 1)}, nil)
	if err != nil {
		t.Fatal(err)
	}
	manager.SetCoordination(bridge)
	t.Cleanup(func() { bridge.Close(); manager.SetCoordination(nil) })
	issue, err := store.CreateIssue(CreateIssueInput{Title: "Deduplicate dispatch", Objective: "one event", Priority: "medium", Status: "todo", WorkMode: "autonomous", AssigneeAgentID: "backend-engineer"})
	if err != nil {
		t.Fatal(err)
	}

	if err = manager.DispatchIssue(issue.ID); err != nil {
		t.Fatal(err)
	}
	if err = manager.DispatchIssue(issue.ID); err != nil {
		t.Fatalf("idempotent dispatch failed: %v", err)
	}
	current, err := store.GetIssue(issue.ID)
	if err != nil {
		t.Fatal(err)
	}
	if current.ExecutionPhase != "scheduled" {
		t.Fatalf("dispatch ownership was not reserved: %+v", current)
	}
	var count int64
	if err = store.db.Table("coordination_events").Where("coordination_id = ? AND payload LIKE ? AND payload LIKE ?", issue.ID, "%\"type\":\"issue_assigned\"%", "%\"issueId\":\""+issue.ID+"\"%").Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("repeated dispatch published %d assignment events", count)
	}
}

func TestResumeEnqueueRecoversInterruptedDurableEffect(t *testing.T) {
	store, manager := bridgeTestManager(t)
	bridge, err := newTestCoordinationBridge(manager, "resume-recovery", "board_autonomy", &coordinationDeliveryRecorder{notify: make(chan struct{}, 1)}, nil)
	if err != nil {
		t.Fatal(err)
	}
	manager.SetCoordination(bridge)
	t.Cleanup(func() { bridge.Close(); manager.SetCoordination(nil) })
	issue, err := store.CreateIssue(CreateIssueInput{Title: "Resume", Objective: "recover wake enqueue", Priority: "medium", WorkMode: "autonomous", AssigneeAgentID: "backend-engineer"})
	if err != nil {
		t.Fatal(err)
	}
	command := coordination.AgentCommand{CommandID: "wake-command", IssueID: issue.ID, AgentID: issue.AssigneeAgentID, TaskAgentID: issue.AssigneeTaskAgentID, Message: "continue"}
	if err = store.db.Model(&Issue{}).Where("id = ?", issue.ID).Updates(map[string]any{
		"status": "in_progress", "execution_phase": "resuming", "sleep_token": command.CommandID, "checkout_execution_id": "",
	}).Error; err != nil {
		t.Fatal(err)
	}
	if err = bridge.EnqueueIssueResumeExecution(context.Background(), command); err != nil {
		t.Fatal(err)
	}
	queued, err := bridge.Execution(context.Background(), "resume-"+command.CommandID)
	if err != nil || queued.Status != coordination.ExecutionQueued {
		t.Fatalf("queued=%+v err=%v", queued, err)
	}
	if err = bridge.EnqueueIssueResumeExecution(context.Background(), command); err != nil {
		t.Fatalf("idempotent retry failed: %v", err)
	}
}

func TestQueuedParentResumeMergesLaterUrgentChildOutcome(t *testing.T) {
	store, manager := bridgeTestManager(t)
	bridge, err := newTestCoordinationBridge(manager, "resume-merge", "board_autonomy", &coordinationDeliveryRecorder{notify: make(chan struct{}, 1)}, nil)
	if err != nil {
		t.Fatal(err)
	}
	manager.SetCoordination(bridge)
	t.Cleanup(func() { bridge.Close(); manager.SetCoordination(nil) })
	issue, err := store.CreateIssue(CreateIssueInput{Title: "Parent", Objective: "integrate children", Priority: "high", WorkMode: "autonomous", AssigneeAgentID: "backend-engineer"})
	if err != nil {
		t.Fatal(err)
	}
	if err = store.db.Model(&Issue{}).Where("id = ?", issue.ID).Updates(map[string]any{"status": "in_progress", "execution_phase": "sleeping", "checkout_execution_id": ""}).Error; err != nil {
		t.Fatal(err)
	}
	first := coordination.AgentCommand{CommandID: "heartbeat-wake", IssueID: issue.ID, AgentID: issue.AssigneeAgentID, TaskAgentID: issue.AssigneeTaskAgentID, Message: "periodic check"}
	if err = bridge.EnqueueIssueResumeExecution(context.Background(), first); err != nil {
		t.Fatal(err)
	}
	second := coordination.AgentCommand{CommandID: "budget-child-wake", IssueID: issue.ID, AgentID: issue.AssigneeAgentID, TaskAgentID: issue.AssigneeTaskAgentID, Message: "child budget exceeded; choose a recovery"}
	if err = bridge.EnqueueIssueResumeExecution(context.Background(), second); err != nil {
		t.Fatal(err)
	}
	queued, err := bridge.Execution(context.Background(), "resume-"+first.CommandID)
	if err != nil {
		t.Fatal(err)
	}
	if queued.Priority != coordination.ExecutionPriorityWakeup || !strings.Contains(queued.Spec.Prompt, first.Message) || !strings.Contains(queued.Spec.Prompt, second.Message) {
		t.Fatalf("merged resume=%+v", queued)
	}
	if queued.Spec.Values["control.resumePrompt"] != queued.Spec.Prompt {
		t.Fatalf("resume value did not include merged message: %+v", queued.Spec.Values)
	}
	if _, err = bridge.Execution(context.Background(), "resume-"+second.CommandID); !errors.Is(err, coordination.ErrExecutionNotFound) {
		t.Fatalf("second wake created a duplicate execution: %v", err)
	}
	if len(bridge.runtime.ExecutionWorkers) != store.Config().Concurrency+1 {
		t.Fatalf("workers=%d, want configured+reserved", len(bridge.runtime.ExecutionWorkers))
	}
	reserved := bridge.runtime.ExecutionWorkers[len(bridge.runtime.ExecutionWorkers)-1]
	if reserved.MinimumPriority != coordination.ExecutionPriorityWakeup {
		t.Fatalf("reserved worker minimum priority=%d", reserved.MinimumPriority)
	}
}

func TestBoardCommentSteersExactTaskAgentThroughCoordination(t *testing.T) {
	store, manager := bridgeTestManager(t)
	delivery := &coordinationDeliveryRecorder{notify: make(chan struct{}, 4)}
	bridge, err := newTestCoordinationBridge(manager, "coord-comment", "board_autonomy", delivery, nil)
	if err != nil {
		t.Fatal(err)
	}
	manager.SetCoordination(bridge)
	ctx, cancel := context.WithCancel(context.Background())
	bridge.Start(ctx)
	t.Cleanup(func() { cancel(); bridge.Close(); manager.SetCoordination(nil) })

	issue, err := store.CreateIssue(CreateIssueInput{Title: "Comment routing", Objective: "steer exact identity", Priority: "medium", WorkMode: "autonomous", AssigneeAgentID: "backend-engineer"})
	if err != nil {
		t.Fatal(err)
	}
	comment, err := manager.AddIssueComment(issue.ID, "请先补充可复现测试。")
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-delivery.notify:
	case <-time.After(2 * time.Second):
		t.Fatal("Board comment was not delivered")
	}
	delivery.mu.Lock()
	defer delivery.mu.Unlock()
	if len(delivery.commands) != 1 {
		t.Fatalf("commands=%+v", delivery.commands)
	}
	command := delivery.commands[0]
	if command.IssueID != issue.ID || command.TaskAgentID != issue.AssigneeTaskAgentID || !strings.Contains(command.Message, comment.Body) || !strings.Contains(command.Message, "Board 评论") {
		t.Fatalf("command=%+v", command)
	}
}

func TestBindingRejectsInvalidCapabilityPolicy(t *testing.T) {
	_, manager := bridgeTestManager(t)
	bridge, err := newTestCoordinationBridge(manager, "coord-invalid-policy", "board_autonomy", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	manager.SetCoordination(bridge)
	t.Cleanup(func() { bridge.Close(); manager.SetCoordination(nil) })
	issue, err := manager.CreateIssue(CreateIssueInput{Title: "Invalid policy", Objective: "reject", Priority: "low", Status: "backlog", WorkMode: "autonomous"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = bridge.BindIssue(context.Background(), issue.ID, "board_autonomy", "1", json.RawMessage(`{"capabilityPolicy":{"defaultSelection":"everything","maxCapabilities":1000}}`))
	if err == nil {
		t.Fatal("invalid capability policy should not be persisted")
	}
}

func TestBoardDelegationPersistsCapabilityPlanOnChildIssue(t *testing.T) {
	store, manager := bridgeTestManager(t)
	bridge, err := newTestCoordinationBridge(manager, "coord-capabilities", "board_autonomy", &coordinationDeliveryRecorder{notify: make(chan struct{}, 10)}, nil)
	if err != nil {
		t.Fatal(err)
	}
	manager.SetCoordination(bridge)
	ctx, cancel := context.WithCancel(context.Background())
	bridge.Start(ctx)
	t.Cleanup(func() { cancel(); bridge.Close(); manager.SetCoordination(nil) })
	parent, err := manager.CreateIssue(CreateIssueInput{Title: "Capability parent", Objective: "delegate", Priority: "medium", Status: "backlog", WorkMode: "autonomous"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = bridge.BindIssue(context.Background(), parent.ID, "board_autonomy", "1", json.RawMessage(`{"capabilityPolicy":{"allowed":[{"kind":"phone","name":"default"},{"kind":"tool","name":"workspace"},{"kind":"tool","name":"delivery"},{"kind":"tool","name":"coordination"}]}}`)); err != nil {
		t.Fatal(err)
	}
	request := coordination.DelegationRequest{Children: []coordination.ChildWork{{
		AgentID: "backend-engineer", Prompt: "inspect with Board only", CapabilitySelection: coordination.CapabilityReplace,
		Capabilities: []capability.Ref{{Kind: capability.KindPhone, Name: "default", Config: map[string]any{"installedApps": []any{"aegis.board"}}}},
	}}}
	if err = bridge.SubmitDelegation(context.Background(), "board-capability-delegation", parent.ID, "parent-execution", "frontend-engineer", request); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		var children []Issue
		if queryErr := store.db.Where("parent_id = ?", parent.ID).Find(&children).Error; queryErr != nil {
			t.Fatal(queryErr)
		}
		if len(children) == 1 {
			child := children[0]
			if child.CapabilitySelection != "replace" || len(child.Capabilities) != 1 || child.Capabilities[0].Kind != capability.KindPhone || child.Capabilities[0].Name != "default" {
				t.Fatalf("child capability plan=%+v selection=%q", child.Capabilities, child.CapabilitySelection)
			}
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("delegated Board child was not created")
}

func TestDelegationRejectsUnapprovedPluginBeforeOutbox(t *testing.T) {
	_, manager := bridgeTestManager(t)
	bridge, err := newTestCoordinationBridge(manager, "coord-reject-plugin", "board_autonomy", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	manager.SetCoordination(bridge)
	t.Cleanup(func() { bridge.Close(); manager.SetCoordination(nil) })
	parent, err := manager.CreateIssue(CreateIssueInput{Title: "Policy parent", Objective: "protect", Priority: "medium", Status: "backlog", WorkMode: "autonomous"})
	if err != nil {
		t.Fatal(err)
	}
	err = bridge.SubmitDelegation(context.Background(), "unapproved-plugin", parent.ID, "parent-execution", "frontend-engineer", coordination.DelegationRequest{Children: []coordination.ChildWork{{
		AgentID: "backend-engineer", Prompt: "use an unapproved connector", CapabilitySelection: coordination.CapabilityMerge,
		Capabilities: []capability.Ref{{Kind: capability.KindMCP, Name: "github"}},
	}}})
	if err == nil || !strings.Contains(err.Error(), "explicit allowed policy") {
		t.Fatalf("error=%v", err)
	}
	var eventCount int64
	if err = manager.store.db.Table("coordination_events").Where("id = ?", "unapproved-plugin").Count(&eventCount).Error; err != nil {
		t.Fatal(err)
	}
	if eventCount != 0 {
		t.Fatalf("rejected delegation persisted %d event(s)", eventCount)
	}
}

func TestBoardAutonomyCompletionFlowsBackToParent(t *testing.T) {
	store, manager := bridgeTestManager(t)
	delivery := &coordinationDeliveryRecorder{notify: make(chan struct{}, 10)}
	bridge, err := newTestCoordinationBridge(manager, "coord-test", "board_autonomy", delivery, nil)
	if err != nil {
		t.Fatal(err)
	}
	manager.SetCoordination(bridge)
	ctx, cancel := context.WithCancel(context.Background())
	bridge.Start(ctx)
	t.Cleanup(func() { cancel(); bridge.Close(); manager.SetCoordination(nil) })
	parent, err := store.CreateIssue(CreateIssueInput{Title: "Parent", Objective: "integrate", Priority: "medium", Status: "in_progress", WorkMode: "autonomous", AssigneeAgentID: "frontend-engineer"})
	if err != nil {
		t.Fatal(err)
	}
	child, err := store.CreateIssue(CreateIssueInput{ParentID: parent.ID, Title: "Child", Objective: "produce evidence", Priority: "medium", Status: "in_progress", WorkMode: "autonomous", AssigneeAgentID: "backend-engineer", CreatedBy: "frontend-engineer"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = bridge.BindIssue(context.Background(), parent.ID, "board_autonomy", "1", nil); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	if err = store.db.Model(&Issue{}).Where("id = ?", child.ID).Updates(map[string]any{"status": "done", "execution_phase": "completed", "result": "evidence ready", "completed_at": now, "updated_at": now}).Error; err != nil {
		t.Fatal(err)
	}
	manager.reconcileIssueID(child.ID)
	select {
	case <-delivery.notify:
	case <-time.After(2 * time.Second):
		t.Fatal("child completion did not flow back to parent")
	}
	delivery.mu.Lock()
	defer delivery.mu.Unlock()
	command := delivery.commands[len(delivery.commands)-1]
	if command.AgentID != "frontend-engineer" || command.IssueID != parent.ID || !strings.Contains(command.Message, "evidence ready") {
		t.Fatalf("unexpected completion delivery: %+v", command)
	}
}

func TestAssignExistingIssueIsRoutedThroughSelectedMode(t *testing.T) {
	_, manager := bridgeTestManager(t)
	delivery := &coordinationDeliveryRecorder{notify: make(chan struct{}, 10)}
	bridge, err := newTestCoordinationBridge(manager, "coord-test", "board_autonomy", delivery, nil)
	if err != nil {
		t.Fatal(err)
	}
	manager.SetCoordination(bridge)
	ctx, cancel := context.WithCancel(context.Background())
	bridge.Start(ctx)
	t.Cleanup(func() { cancel(); bridge.Close(); manager.SetCoordination(nil) })
	issue, err := manager.CreateIssue(CreateIssueInput{Title: "Unassigned", Objective: "Assign later", Priority: "low", Status: "backlog", WorkMode: "autonomous"})
	if err != nil {
		t.Fatal(err)
	}
	assigned := "frontend-engineer"
	if _, err := manager.UpdateBoardIssue(issue.ID, UpdateIssueInput{AssigneeAgentID: &assigned}); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if execution, getErr := bridge.Execution(context.Background(), issue.ID); getErr == nil && execution.ID == issue.ID {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("assigning an existing Issue did not create a Coordination execution")
}

func TestCoordinationCanProactivelyInvokeAgent(t *testing.T) {
	store, manager := bridgeTestManager(t)
	bridge, err := newTestCoordinationBridge(manager, "coord-proactive", "board_autonomy", &coordinationDeliveryRecorder{notify: make(chan struct{}, 10)}, nil)
	if err != nil {
		t.Fatal(err)
	}
	manager.SetCoordination(bridge)
	ctx, cancel := context.WithCancel(context.Background())
	bridge.Start(ctx)
	t.Cleanup(func() { cancel(); bridge.Close(); manager.SetCoordination(nil) })
	issue, err := manager.CreateIssue(CreateIssueInput{Title: "Operator controlled", Objective: "coordinate proactively", Priority: "medium", Status: "backlog", WorkMode: "autonomous"})
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := bridge.InvokeAgent(context.Background(), coordination.AgentInvocation{
		IssueID: issue.ID, ParentAgentID: "operator", Child: coordination.ChildWork{AgentID: "backend-engineer", Prompt: "Start without waiting for a parent Agent", CapabilitySelection: coordination.CapabilityReplace},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !receipt.Accepted || receipt.EventID == "" || receipt.CoordinationID != issue.ID {
		t.Fatalf("receipt=%+v", receipt)
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		var children []Issue
		if err = store.db.Where("parent_id = ?", issue.ID).Find(&children).Error; err != nil {
			t.Fatal(err)
		}
		if len(children) == 1 {
			if children[0].AssigneeAgentID != "backend-engineer" || children[0].CreatedBy != "operator" || children[0].CapabilitySelection != "replace" {
				t.Fatalf("child=%+v", children[0])
			}
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("proactive invocation did not create a child Board Issue")
}

func TestBoardAutonomySleepWakesAgentAfterDurableDelay(t *testing.T) {
	store, manager := bridgeTestManager(t)
	delivery := &coordinationDeliveryRecorder{notify: make(chan struct{}, 10)}
	bridge, err := newTestCoordinationBridge(manager, "coord-test", "board_autonomy", delivery, nil)
	if err != nil {
		t.Fatal(err)
	}
	manager.SetCoordination(bridge)
	ctx, cancel := context.WithCancel(context.Background())
	bridge.runtime.Poll = 5 * time.Millisecond
	bridge.Start(ctx)
	t.Cleanup(func() { cancel(); bridge.Close(); manager.SetCoordination(nil) })
	issue, err := manager.CreateIssue(CreateIssueInput{Title: "Timed", Objective: "Wake later", Priority: "low", Status: "backlog", WorkMode: "autonomous"})
	if err != nil {
		t.Fatal(err)
	}
	if err := bridge.SubmitWait(context.Background(), "wait-1", issue.ID, "execution-1", "backend-engineer", coordination.WaitRequest{WakeAfterSeconds: 1, Message: "scheduled check"}); err != nil {
		t.Fatal(err)
	}
	if err := store.db.Model(&Issue{}).Where("id = ?", issue.ID).Updates(map[string]any{"execution_phase": "sleeping", "sleep_token": "wait-1"}).Error; err != nil {
		t.Fatal(err)
	}
	select {
	case <-delivery.notify:
		delivery.mu.Lock()
		commands := append([]coordination.AgentCommand(nil), delivery.commands...)
		delivery.mu.Unlock()
		if len(commands) == 0 || commands[len(commands)-1].Message != "scheduled check" {
			t.Fatalf("commands=%+v", commands)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("sleep timer did not wake agent")
	}
}
