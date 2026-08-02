package modes

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"aegis/coordination"
)

func TestBoardAutonomyAssignmentEnqueuesNotifiesAndSchedulesHeartbeat(t *testing.T) {
	base := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	effects, err := (BoardAutonomy{}).Decide(context.Background(), coordination.Event{
		ID: "assigned", Type: coordination.EventIssueAssigned, CoordinationID: "task", TaskID: "task",
		IssueID: "child", AgentID: "worker", TaskAgentID: "task-agent-child",
		ParentAgentID: "worker", ParentTaskAgentID: "task-agent-parent", OccurredAt: base,
	}, coordination.Binding{}, coordination.Snapshot{})
	if err != nil {
		t.Fatal(err)
	}
	if len(effects) != 3 || effects[0].Type != coordination.EffectEnqueueIssue || effects[1].Type != coordination.EffectSendRelay || effects[2].Type != coordination.EffectScheduleWakeup {
		t.Fatalf("effects=%+v", effects)
	}
	if !effects[2].AvailableAt.Equal(base.Add(time.Minute)) {
		t.Fatalf("heartbeat due=%s", effects[2].AvailableAt)
	}
	for _, effect := range effects {
		if effect.Type == coordination.EffectSuspendAgent || effect.Type == coordination.EffectStartSubagent {
			t.Fatalf("board autonomy bypassed Board or suspended parent: %+v", effects)
		}
	}
	second, err := (BoardAutonomy{}).Decide(context.Background(), coordination.Event{
		ID: "reassigned", Type: coordination.EventIssueAssigned, CoordinationID: "task", TaskID: "task",
		IssueID: "child", AgentID: "worker", TaskAgentID: "task-agent-child",
		ParentAgentID: "worker", ParentTaskAgentID: "task-agent-parent", OccurredAt: base.Add(time.Minute),
	}, coordination.Binding{}, coordination.Snapshot{})
	if err != nil {
		t.Fatal(err)
	}
	for index := range effects {
		if effects[index].IdempotencyKey == second[index].IdempotencyKey {
			t.Fatalf("separate assignment events reused effect key %q", effects[index].IdempotencyKey)
		}
	}
}

func TestBoardAutonomyDelegationAlwaysCreatesChildIssues(t *testing.T) {
	payload, _ := json.Marshal(coordination.DelegationRequest{Children: []coordination.ChildWork{
		{AgentID: "child-agent", Prompt: "work independently"},
		{AgentID: "child-agent", Prompt: "work on a second slice"},
	}})
	effects, err := (BoardAutonomy{}).Decide(context.Background(), coordination.Event{ID: "delegate", Type: coordination.EventDelegationRequested, CoordinationID: "task", IssueID: "parent", AgentID: "parent-agent", Payload: payload}, coordination.Binding{}, coordination.Snapshot{})
	if err != nil {
		t.Fatal(err)
	}
	if len(effects) != 2 || effects[0].Type != coordination.EffectCreateIssue || effects[1].Type != coordination.EffectCreateIssue {
		t.Fatalf("effects=%+v", effects)
	}
}

func TestBoardAutonomyHeartbeatDoesNotSteerActiveModelLoop(t *testing.T) {
	base := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	payload, _ := json.Marshal(coordination.WakeupPayload{Kind: "heartbeat", RequestedAt: base.Add(-time.Minute), WakeAfterSeconds: 60})
	snapshot := coordination.Snapshot{Current: &coordination.WorkItem{ID: "parent", Status: coordination.WorkRunning, ExecutionPhase: "active"}, Children: []coordination.WorkItem{
		{ID: "one", Status: coordination.WorkRunning}, {ID: "two", Status: coordination.WorkSucceeded},
	}}
	effects, err := (BoardAutonomy{}).Decide(context.Background(), coordination.Event{
		ID: "heartbeat", Type: coordination.EventTimerFired, CoordinationID: "task", TaskID: "task",
		IssueID: "parent", AgentID: "leader", OccurredAt: base, Payload: payload,
	}, coordination.Binding{}, snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if len(effects) != 1 || effects[0].Type != coordination.EffectScheduleWakeup {
		t.Fatalf("effects=%+v", effects)
	}
	if !effects[0].AvailableAt.Equal(base.Add(time.Minute)) {
		t.Fatalf("next heartbeat due=%s", effects[0].AvailableAt)
	}
}

func TestBoardAutonomyHeartbeatWakesWaitingAgentAndRepeats(t *testing.T) {
	base := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	payload, _ := json.Marshal(coordination.WakeupPayload{Kind: "heartbeat", RequestedAt: base.Add(-time.Minute), WakeAfterSeconds: 60})
	snapshot := coordination.Snapshot{Current: &coordination.WorkItem{ID: "parent", Status: coordination.WorkWaiting, ExecutionPhase: "waiting_children"}, Children: []coordination.WorkItem{
		{ID: "one", Status: coordination.WorkRunning}, {ID: "two", Status: coordination.WorkSucceeded},
	}}
	effects, err := (BoardAutonomy{}).Decide(context.Background(), coordination.Event{
		ID: "heartbeat", Type: coordination.EventTimerFired, CoordinationID: "task", TaskID: "task",
		IssueID: "parent", AgentID: "leader", OccurredAt: base, Payload: payload,
	}, coordination.Binding{}, snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if len(effects) != 2 || effects[0].Type != coordination.EffectDeliverMessage || effects[1].Type != coordination.EffectScheduleWakeup {
		t.Fatalf("effects=%+v", effects)
	}
	var command coordination.AgentCommand
	if err = json.Unmarshal(effects[0].Payload, &command); err != nil {
		t.Fatal(err)
	}
	if command.Delivery != "steer" || !strings.Contains(command.Message, "1 分 0 秒") || !strings.Contains(command.Message, "运行 1") {
		t.Fatalf("command=%+v", command)
	}
}

func TestBoardAutonomySleepIsOneShotAndHeartbeatMayPreemptIt(t *testing.T) {
	base := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	waitPayload, _ := json.Marshal(coordination.WaitRequest{WakeAfterSeconds: 300, Message: "resume planned review"})
	effects, err := (BoardAutonomy{}).Decide(context.Background(), coordination.Event{ID: "sleep", Type: coordination.EventWaitRequested, CoordinationID: "task", IssueID: "issue", AgentID: "agent", OccurredAt: base, Payload: waitPayload}, coordination.Binding{}, coordination.Snapshot{})
	if err != nil || len(effects) != 1 || effects[0].Type != coordination.EffectScheduleWakeup || !effects[0].AvailableAt.Equal(base.Add(5*time.Minute)) {
		t.Fatalf("sleep effects=%+v err=%v", effects, err)
	}
	var scheduled coordination.ScheduleWakeupCommand
	if err = json.Unmarshal(effects[0].Payload, &scheduled); err != nil {
		t.Fatal(err)
	}
	wakeSnapshot := coordination.Snapshot{Current: &coordination.WorkItem{ID: "issue", Status: coordination.WorkWaiting, ExecutionPhase: "sleeping", SleepToken: "sleep"}}
	effects, err = (BoardAutonomy{}).Decide(context.Background(), coordination.Event{ID: "sleep-fired", Type: coordination.EventTimerFired, CoordinationID: "task", IssueID: "issue", AgentID: "agent", CorrelationID: scheduled.Event.CorrelationID, OccurredAt: base.Add(5 * time.Minute), Payload: scheduled.Event.Payload}, coordination.Binding{}, wakeSnapshot)
	if err != nil || len(effects) != 1 || effects[0].Type != coordination.EffectDeliverMessage {
		t.Fatalf("wake effects=%+v err=%v", effects, err)
	}

	staleSnapshot := coordination.Snapshot{Current: &coordination.WorkItem{ID: "issue", Status: coordination.WorkWaiting, ExecutionPhase: "sleeping", SleepToken: "newer-sleep"}}
	effects, err = (BoardAutonomy{}).Decide(context.Background(), coordination.Event{ID: "old-sleep-fired", Type: coordination.EventTimerFired, CoordinationID: "task", IssueID: "issue", AgentID: "agent", CorrelationID: scheduled.Event.CorrelationID, OccurredAt: base.Add(5 * time.Minute), Payload: scheduled.Event.Payload}, coordination.Binding{}, staleSnapshot)
	if err != nil || len(effects) != 0 {
		t.Fatalf("stale sleep timer was not ignored: effects=%+v err=%v", effects, err)
	}
}

func TestRegisterBuiltinsExposesOnlyBoardAutonomy(t *testing.T) {
	registry := coordination.NewRegistry()
	if err := RegisterBuiltins(registry); err != nil {
		t.Fatal(err)
	}
	modes := registry.Modes()
	if len(modes) != 1 || modes[0].Name != "board_autonomy" || modes[0].Version != "1" {
		t.Fatalf("modes=%+v", modes)
	}
}
