package control

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	"aegis/agenthost"
	"aegis/capability"
	"aegis/coordination"
	"github.com/z3r2ne/agentcore"
	agentcoresqlite "github.com/z3r2ne/agentcore/sqlitestore"
)

type nativeSessionEntry struct {
	agentID     string
	taskAgentID string
	session     *agenthost.Session
}

type nativeSessionStore interface {
	agentcore.SessionStore
	LoadSession(context.Context, string) (agentcore.SessionSnapshot, error)
}

// NativeSessionDelivery indexes live AgentCore sessions by Issue. It injects
// coordination messages into an active task-local loop; after a loop has
// yielded, durable delivery creates a new execution and restores its SQLite
// session snapshot. It never falls back to a global Agent identity.
type NativeSessionDelivery struct {
	manager *Manager
	store   nativeSessionStore
	initErr error
	mu      sync.RWMutex
	entries map[string]nativeSessionEntry
}

func NewNativeSessionDelivery(manager *Manager) *NativeSessionDelivery {
	delivery := &NativeSessionDelivery{manager: manager, entries: make(map[string]nativeSessionEntry)}
	if manager != nil && manager.store != nil {
		delivery.store, delivery.initErr = agentcoresqlite.New(manager.store.db)
		manager.setNativeSessionDelivery(delivery)
	}
	return delivery
}

// AbortIssue stops a live native loop after the durable control plane has
// already made the Issue terminal (for example, Task wall-clock exhaustion).
func (d *NativeSessionDelivery) AbortIssue(issueID string) {
	if d == nil {
		return
	}
	d.mu.RLock()
	entry := d.entries[strings.TrimSpace(issueID)]
	d.mu.RUnlock()
	if entry.session != nil {
		_ = entry.session.Abort()
	}
}

// SteerLiveIssue injects a user message only when the exact in-process
// AgentCore session is currently running. It never creates a Coordination
// wakeup, which makes it suitable for the hidden concierge conversation.
func (d *NativeSessionDelivery) SteerLiveIssue(issueID, agentID, message string) (bool, error) {
	if d == nil || strings.TrimSpace(message) == "" {
		return false, nil
	}
	d.mu.RLock()
	entry, ok := d.entries[strings.TrimSpace(issueID)]
	d.mu.RUnlock()
	if !ok || entry.session == nil || (strings.TrimSpace(agentID) != "" && entry.agentID != strings.TrimSpace(agentID)) || !entry.session.Status().Running {
		return false, nil
	}
	return true, entry.session.Steer(agentcore.TextMessage(agentcore.RoleUser, message))
}

func (d *NativeSessionDelivery) Run(ctx context.Context, host *agenthost.Host, issueID string, spec agenthost.ExecutionSpec, sink agentcore.EventSink) (agenthost.Result, error) {
	if d == nil || host == nil {
		return agenthost.Result{}, errors.New("control coordination: native session delivery requires a Host")
	}
	if d.initErr != nil {
		return agenthost.Result{}, fmt.Errorf("control coordination: initialize AgentCore session store: %w", d.initErr)
	}
	var snapshot *agentcore.SessionSnapshot
	if d.store != nil {
		loaded, loadErr := d.store.LoadSession(ctx, spec.SessionID)
		if loadErr == nil {
			snapshot = &loaded
		} else if !errors.Is(loadErr, agentcoresqlite.ErrSessionNotFound) {
			return agenthost.Result{}, fmt.Errorf("control coordination: restore AgentCore session %q: %w", spec.SessionID, loadErr)
		}
	}
	session, err := host.NewSession(ctx, spec, snapshot, agentcore.SessionOptions{Store: d.store, SessionID: spec.SessionID, SteeringMode: agentcore.DeliveryAll, FollowUpMode: agentcore.DeliveryAll})
	if err != nil {
		return agenthost.Result{}, err
	}
	if binder, ok := spec.Runtime.(interface{ BindSession(*agenthost.Session) }); ok {
		binder.BindSession(session)
	}
	d.mu.Lock()
	taskAgentID, _ := spec.Values["control.taskAgentId"].(string)
	d.entries[issueID] = nativeSessionEntry{agentID: spec.AgentID, taskAgentID: taskAgentID, session: session}
	d.mu.Unlock()
	defer func() {
		d.mu.Lock()
		if current := d.entries[issueID]; current.session == session {
			delete(d.entries, issueID)
		}
		d.mu.Unlock()
		_ = session.Close()
	}()
	result, runErr := session.Prompt(ctx, []agentcore.Message{agentcore.TextMessage(agentcore.RoleUser, spec.Prompt)}, sink)
	wrapped := agenthost.Result{Core: result, Capabilities: append([]capability.Snapshot(nil), session.Capabilities...)}
	if runErr != nil || result.StopReason == agentcore.StopReasonTerminated || d.manager == nil || d.manager.store == nil {
		return wrapped, runErr
	}
	issue, issueErr := d.manager.store.GetIssue(issueID)
	if issueErr != nil || issueStatusTerminal(issue.Status) || issue.ExecutionPhase == "sleeping" {
		return wrapped, issueErr
	}
	children, unfinished, childErr := d.childState(issueID)
	if childErr != nil || len(children) == 0 || unfinished > 0 {
		return wrapped, childErr
	}
	// Terminal child results are immediately actionable, so integrate them in
	// this execution. Waiting for unfinished children is handled durably by a
	// later Coordination execution and never keeps this goroutine alive.
	result, runErr = session.Prompt(ctx, []agentcore.Message{agentcore.TextMessage(agentcore.RoleUser, terminalChildrenPrompt(children))}, sink)
	return agenthost.Result{Core: result, Capabilities: append([]capability.Snapshot(nil), session.Capabilities...)}, runErr
}

func (d *NativeSessionDelivery) childState(issueID string) ([]Issue, int, error) {
	var children []Issue
	if err := d.manager.store.db.Where("parent_id = ?", issueID).Order("number asc, created_at asc").Find(&children).Error; err != nil {
		return nil, 0, err
	}
	unfinished := 0
	for _, child := range children {
		if !issueStatusTerminal(child.Status) {
			unfinished++
		}
	}
	return children, unfinished, nil
}

func terminalChildrenPrompt(children []Issue) string {
	var summary strings.Builder
	for _, child := range children {
		fmt.Fprintf(&summary, "- %s · %s [%s]：%s\n", child.Identifier, child.Title, child.Status, fallback(child.Result, fallback(child.Error, "没有结果摘要")))
	}
	return fmt.Sprintf("所有直属子 Issue 已经结束。请在同一任务会话中检查 Phone Board/Relay 与以下结果，完成父 Issue 的整合、验证和最终交付；不要只复述子项。若子项状态为 failed 或 budget_exceeded，必须明确选择：使用 phone_board_continue_issue 重新派发同一 Issue、使用 phone_board_delegate 创建新 Issue 探索其他方向，或接受部分结果并继续。重新执行不会重置 Task 总时钟墙预算。\n\n%s", summary.String())
}

func (d *NativeSessionDelivery) DeliverCoordinationMessage(ctx context.Context, command coordination.AgentCommand) error {
	if d == nil || strings.TrimSpace(command.Message) == "" {
		return nil
	}
	d.mu.RLock()
	entry, ok := d.entries[command.IssueID]
	d.mu.RUnlock()
	identityMatches := command.TaskAgentID == "" || entry.taskAgentID == command.TaskAgentID
	entryMatches := ok && entry.session != nil && identityMatches && (command.AgentID == "" || entry.agentID == command.AgentID)
	endingTurn := false
	if d.manager != nil && d.manager.store != nil {
		if issue, err := d.manager.store.GetIssue(command.IssueID); err == nil {
			endingTurn = issue.ExecutionPhase == "sleeping" || issue.ExecutionPhase == "resuming" || issue.ExecutionPhase == "waiting_children"
		}
	}
	if command.Delivery == "assignment" && (!entryMatches || !entry.session.Status().Running || endingTurn) {
		// The enqueue effect already starts the Issue with its complete initial
		// prompt. A delayed assignment notification must never resurrect a turn
		// that has since gone to sleep or finished.
		return nil
	}
	if entryMatches && entry.session.Status().Running && !endingTurn {
		message := agentcore.TextMessage(agentcore.RoleUser, command.Message)
		if command.Delivery == "steer" || command.Delivery == "assignment" {
			return entry.session.Steer(message)
		}
		return entry.session.FollowUp(message)
	}
	if entryMatches {
		// The model turn has already committed to sleeping (or is closing after
		// it). Retrying the durable effect avoids losing a wake message in the
		// just-terminated session's in-memory queue and prevents overlapping runs.
		return errors.New("control coordination: task-local Agent turn is closing; delivery will retry as a new turn")
	}
	if d.manager == nil || d.manager.Coordination() == nil {
		return errors.New("control coordination: task-local Agent loop is not live")
	}
	return d.manager.Coordination().EnqueueIssueResumeExecution(ctx, command)
}

var _ CoordinationDelivery = (*NativeSessionDelivery)(nil)
