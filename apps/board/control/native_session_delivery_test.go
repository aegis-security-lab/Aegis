package control

import (
	"context"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"aegis/agenthost"
	boardcoordination "aegis/apps/board/coordination"
	"aegis/capability"
	"github.com/z3r2ne/agentcore"
)

type deliveryResolver struct{ model agentcore.Model }

func (r deliveryResolver) ResolveModel(context.Context, agenthost.ModelRef) (agentcore.Model, error) {
	return r.model, nil
}

type deliveryModel struct {
	mu       sync.Mutex
	calls    int
	requests []agentcore.ModelRequest
	started  chan struct{}
	release  chan struct{}
}

func (m *deliveryModel) Stream(_ context.Context, request agentcore.ModelRequest) (agentcore.ModelStream, error) {
	m.mu.Lock()
	m.calls++
	call := m.calls
	m.requests = append(m.requests, request)
	m.mu.Unlock()
	if call == 1 {
		close(m.started)
		return &deliveryStream{wait: m.release, chunk: agentcore.ModelChunk{TextDelta: "parent work", StopReason: agentcore.StopReasonStop}}, nil
	}
	return &deliveryStream{chunk: agentcore.ModelChunk{TextDelta: "integrated child result", StopReason: agentcore.StopReasonStop}}, nil
}

type deliveryStream struct {
	wait  <-chan struct{}
	chunk agentcore.ModelChunk
	done  bool
}

func (s *deliveryStream) Recv() (agentcore.ModelChunk, error) {
	if s.wait != nil {
		<-s.wait
		s.wait = nil
	}
	if s.done {
		return agentcore.ModelChunk{}, io.EOF
	}
	s.done = true
	return s.chunk, nil
}

func (*deliveryStream) Close() error { return nil }

func TestNativeSessionDeliveryInjectsFollowUpIntoRunningParent(t *testing.T) {
	model := &deliveryModel{started: make(chan struct{}), release: make(chan struct{})}
	host, err := agenthost.New(deliveryResolver{model: model}, capability.NewRegistry())
	if err != nil {
		t.Fatal(err)
	}
	delivery := NewNativeSessionDelivery(nil)
	spec := agenthost.ExecutionSpec{ExecutionID: "parent-execution", SessionID: "parent-session", AgentID: "parent-agent", Model: agenthost.ModelRef{Provider: "test", Model: "test"}, Prompt: "continue parent work"}
	completed := make(chan agenthost.Result, 1)
	failed := make(chan error, 1)
	go func() {
		result, runErr := delivery.Run(context.Background(), host, "parent-issue", spec, nil)
		completed <- result
		failed <- runErr
	}()
	select {
	case <-model.started:
	case <-time.After(time.Second):
		t.Fatal("parent did not start")
	}
	if err = delivery.DeliverCoordinationMessage(context.Background(), boardcoordination.AgentCommand{AgentID: "parent-agent", IssueID: "parent-issue", Message: "child evidence", Delivery: "follow_up"}); err != nil {
		t.Fatal(err)
	}
	close(model.release)
	select {
	case result := <-completed:
		if runErr := <-failed; runErr != nil {
			t.Fatal(runErr)
		}
		if lastAssistantText(result.Core.State.Messages) != "integrated child result" {
			t.Fatalf("result=%+v", result.Core.State.Messages)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("parent did not consume child follow-up")
	}
	model.mu.Lock()
	defer model.mu.Unlock()
	if model.calls != 2 {
		t.Fatalf("model calls=%d", model.calls)
	}
	found := false
	for _, message := range model.requests[1].Messages {
		if message.Text() == "child evidence" {
			found = true
		}
	}
	if !found {
		t.Fatalf("follow-up missing from second turn: %+v", model.requests[1].Messages)
	}
}

func TestNativeSessionDeliveryBudgetSteersIntoSummaryAfterFinalWorkTurn(t *testing.T) {
	model := &deliveryModel{started: make(chan struct{}), release: make(chan struct{})}
	close(model.release)
	host, err := agenthost.New(deliveryResolver{model: model}, capability.NewRegistry())
	if err != nil {
		t.Fatal(err)
	}
	host.Configurer = agenthost.AgentConfigurerFunc(func(ctx context.Context, spec agenthost.ExecutionSpec, config *agentcore.Config) error {
		if configured, ok := spec.Runtime.(agenthost.AgentConfigurer); ok {
			return configured.ConfigureAgent(ctx, spec, config)
		}
		return nil
	})
	controller := &executionBudgetController{
		executionID: "budget-execution", issueID: "budget-issue", startedAt: time.Now(),
		maxTurns: 1, maxActive: time.Hour, maxSummary: 2, summaryTime: time.Minute,
		now: time.Now, phase: "active",
	}
	spec := agenthost.ExecutionSpec{
		ExecutionID: "budget-execution", SessionID: "budget-session", AgentID: "worker",
		Model: agenthost.ModelRef{Provider: "test", Model: "test"}, Prompt: "do work", Runtime: controller,
	}
	result, err := NewNativeSessionDelivery(nil).Run(context.Background(), host, "budget-issue", spec, nil)
	if err != nil {
		t.Fatal(err)
	}
	if result.Core.Turns != 2 || lastAssistantText(result.Core.State.Messages) != "integrated child result" {
		t.Fatalf("budget summary result=%+v", result.Core)
	}
	outcome := controller.Outcome()
	if !outcome.Exceeded || outcome.SummaryUsed != 1 {
		t.Fatalf("outcome=%+v", outcome)
	}
	model.mu.Lock()
	defer model.mu.Unlock()
	if model.calls != 2 {
		t.Fatalf("model calls=%d, want one work and one summary turn", model.calls)
	}
	joined := ""
	for _, message := range model.requests[1].Messages {
		joined += message.Text() + "\n"
	}
	if !strings.Contains(joined, "单次 Execution 预算已耗尽") || !strings.Contains(model.requests[1].SystemPrompt, "停止继续扩展") {
		t.Fatalf("summary restrictions were not supplied: prompt=%q system=%q", joined, model.requests[1].SystemPrompt)
	}
}

func TestNativeSessionDeliveryRestoresTaskSessionInNewRun(t *testing.T) {
	store, manager := bridgeTestManager(t)
	issue, err := store.CreateIssue(CreateIssueInput{Title: "Persistent session", Objective: "continue after wake", Priority: "middle", WorkMode: "autonomous", AssigneeAgentID: "backend-engineer"})
	if err != nil {
		t.Fatal(err)
	}
	model := &deliveryModel{started: make(chan struct{}), release: make(chan struct{})}
	host, err := agenthost.New(deliveryResolver{model: model}, capability.NewRegistry())
	if err != nil {
		t.Fatal(err)
	}
	delivery := NewNativeSessionDelivery(manager)
	spec := agenthost.ExecutionSpec{
		ExecutionID: "first-execution", SessionID: "task-session-" + issue.AssigneeTaskAgentID, AgentID: issue.AssigneeAgentID,
		Model: agenthost.ModelRef{Provider: "test", Model: "test"}, Prompt: "initial turn",
		Values: map[string]any{"control.taskAgentId": issue.AssigneeTaskAgentID},
	}
	firstDone := make(chan error, 1)
	go func() {
		_, runErr := delivery.Run(context.Background(), host, issue.ID, spec, nil)
		firstDone <- runErr
	}()
	select {
	case <-model.started:
	case <-time.After(time.Second):
		t.Fatal("first turn did not start")
	}
	close(model.release)
	if err = <-firstDone; err != nil {
		t.Fatal(err)
	}
	spec.ExecutionID = "second-execution"
	spec.Prompt = "durable wake"
	if _, err = delivery.Run(context.Background(), host, issue.ID, spec, nil); err != nil {
		t.Fatal(err)
	}
	model.mu.Lock()
	defer model.mu.Unlock()
	if model.calls != 2 {
		t.Fatalf("model calls=%d", model.calls)
	}
	texts := make([]string, 0, len(model.requests[1].Messages))
	for _, message := range model.requests[1].Messages {
		texts = append(texts, message.Text())
	}
	joined := strings.Join(texts, "\n")
	if !strings.Contains(joined, "parent work") || !strings.Contains(joined, "durable wake") {
		t.Fatalf("restored request did not contain prior state and wake prompt: %q", joined)
	}
}

func TestNativeSessionDeliveryReleasesSleepingTaskAgent(t *testing.T) {
	store, manager := bridgeTestManager(t)
	issue, err := store.CreateIssue(CreateIssueInput{Title: "Sleep and wake", Objective: "resume the same task identity", Priority: "middle", WorkMode: "autonomous", AssigneeAgentID: "backend-engineer"})
	if err != nil {
		t.Fatal(err)
	}
	model := &deliveryModel{started: make(chan struct{}), release: make(chan struct{})}
	host, err := agenthost.New(deliveryResolver{model: model}, capability.NewRegistry())
	if err != nil {
		t.Fatal(err)
	}
	delivery := NewNativeSessionDelivery(manager)
	spec := agenthost.ExecutionSpec{
		ExecutionID: "sleep-execution", SessionID: "sleep-session", AgentID: issue.AssigneeAgentID,
		Model: agenthost.ModelRef{Provider: "test", Model: "test"}, Prompt: "work then sleep",
		Values: map[string]any{"control.taskAgentId": issue.AssigneeTaskAgentID},
	}
	completed := make(chan agenthost.Result, 1)
	failed := make(chan error, 1)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		result, runErr := delivery.Run(ctx, host, issue.ID, spec, nil)
		completed <- result
		failed <- runErr
	}()
	select {
	case <-model.started:
	case <-time.After(time.Second):
		t.Fatal("task Agent did not start")
	}
	if err = store.db.Model(&Issue{}).Where("id = ?", issue.ID).Update("execution_phase", "sleeping").Error; err != nil {
		t.Fatal(err)
	}
	close(model.release)
	select {
	case result := <-completed:
		if runErr := <-failed; runErr != nil {
			t.Fatal(runErr)
		}
		if lastAssistantText(result.Core.State.Messages) != "parent work" {
			t.Fatalf("result=%+v", result.Core.State.Messages)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("sleeping TaskAgent kept the execution worker occupied")
	}
	delivery.mu.RLock()
	_, live := delivery.entries[issue.ID]
	delivery.mu.RUnlock()
	if live {
		t.Fatal("sleeping TaskAgent session remained live")
	}
	model.mu.Lock()
	defer model.mu.Unlock()
	if model.calls != 1 {
		t.Fatalf("model calls=%d; sleeping must not poll the model", model.calls)
	}
}

func TestNativeSessionDeliveryIntegratesAlreadyTerminalChildren(t *testing.T) {
	store, manager := bridgeTestManager(t)
	parent, err := store.CreateIssue(CreateIssueInput{Title: "Parent", Objective: "integrate child evidence", Priority: "high", WorkMode: "autonomous", AssigneeAgentID: "backend-engineer"})
	if err != nil {
		t.Fatal(err)
	}
	child, err := store.CreateIssue(CreateIssueInput{ParentID: parent.ID, Title: "Child", Objective: "produce evidence", Priority: "middle", WorkMode: "autonomous", AssigneeAgentID: "frontend-engineer"})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	if err = store.db.Model(&Issue{}).Where("id = ?", parent.ID).Updates(map[string]any{"status": "in_progress", "execution_phase": "active"}).Error; err != nil {
		t.Fatal(err)
	}
	if err = store.db.Model(&Issue{}).Where("id = ?", child.ID).Updates(map[string]any{"status": "done", "execution_phase": "completed", "result": "verified child evidence", "completed_at": now}).Error; err != nil {
		t.Fatal(err)
	}
	model := &deliveryModel{started: make(chan struct{}), release: make(chan struct{})}
	host, err := agenthost.New(deliveryResolver{model: model}, capability.NewRegistry())
	if err != nil {
		t.Fatal(err)
	}
	delivery := NewNativeSessionDelivery(manager)
	spec := agenthost.ExecutionSpec{ExecutionID: "parent-execution", SessionID: "parent-session", AgentID: parent.AssigneeAgentID, Model: agenthost.ModelRef{Provider: "test", Model: "test"}, Prompt: "delegate and continue", Values: map[string]any{"control.taskAgentId": parent.AssigneeTaskAgentID}}
	completed := make(chan agenthost.Result, 1)
	failed := make(chan error, 1)
	go func() {
		result, runErr := delivery.Run(context.Background(), host, parent.ID, spec, nil)
		completed <- result
		failed <- runErr
	}()
	select {
	case <-model.started:
	case <-time.After(time.Second):
		t.Fatal("parent did not start")
	}
	close(model.release)
	select {
	case result := <-completed:
		if runErr := <-failed; runErr != nil {
			t.Fatal(runErr)
		}
		if lastAssistantText(result.Core.State.Messages) != "integrated child result" {
			t.Fatalf("result=%+v", result.Core.State.Messages)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("parent did not integrate terminal child")
	}
	model.mu.Lock()
	defer model.mu.Unlock()
	if model.calls != 2 {
		t.Fatalf("model calls=%d; expected initial and terminal integration turns", model.calls)
	}
	if !strings.Contains(model.requests[1].Messages[len(model.requests[1].Messages)-1].Text(), "verified child evidence") {
		t.Fatalf("terminal child evidence missing from integration prompt: %+v", model.requests[1].Messages)
	}
}
