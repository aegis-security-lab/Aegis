package modes

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"aegis/coordination"
)

// BoardAutonomy is Aegis's single coordination policy. Every unit of work is
// a Board Issue, delegation creates child Issues, messages steer live Agents,
// and every non-terminal assignee receives a durable periodic heartbeat.
type BoardAutonomy struct{}

type boardAutonomyConfig struct {
	HeartbeatIntervalSeconds int64 `json:"heartbeatIntervalSeconds,omitempty"`
	NotifyAssignment         *bool `json:"notifyAssignment,omitempty"`
}

func (BoardAutonomy) Name() string    { return "board_autonomy" }
func (BoardAutonomy) Version() string { return "1" }

func (BoardAutonomy) Decide(_ context.Context, event coordination.Event, binding coordination.Binding, snapshot coordination.Snapshot) ([]coordination.PlannedEffect, error) {
	config := boardAutonomyConfig{HeartbeatIntervalSeconds: 60}
	if len(binding.Config) > 0 && string(binding.Config) != "null" {
		if err := json.Unmarshal(binding.Config, &config); err != nil {
			return nil, fmt.Errorf("board_autonomy config: %w", err)
		}
	}
	if config.HeartbeatIntervalSeconds <= 0 {
		config.HeartbeatIntervalSeconds = 60
	}
	notifyAssignment := config.NotifyAssignment == nil || *config.NotifyAssignment

	switch event.Type {
	case coordination.EventDelegationRequested:
		request, err := decode[coordination.DelegationRequest](event.Payload)
		if err != nil {
			return nil, err
		}
		result := make([]coordination.PlannedEffect, 0, len(request.Children))
		for index, child := range request.Children {
			if strings.TrimSpace(child.AgentID) == "" || strings.TrimSpace(child.Prompt) == "" {
				return nil, fmt.Errorf("board_autonomy: child %d requires agentId and prompt", index)
			}
			create, err := effect(coordination.EffectCreateIssue, fmt.Sprintf("board:create:%s:%d", event.ID, index), coordination.CreateIssueCommand{
				ParentIssueID: event.IssueID, ParentAgentID: event.AgentID, ParentTaskAgentID: event.TaskAgentID, Child: child,
			})
			if err != nil {
				return nil, err
			}
			result = append(result, create)
		}
		return result, nil

	case coordination.EventIssueCreated, coordination.EventIssueAssigned:
		if event.IssueID == "" || event.AgentID == "" {
			return nil, nil
		}
		enqueue, err := effect(coordination.EffectEnqueueIssue, "board:enqueue:"+event.ID, coordination.EnqueueIssueCommand{IssueID: event.IssueID, AgentID: event.AgentID})
		if err != nil {
			return nil, err
		}
		result := []coordination.PlannedEffect{enqueue}
		if notifyAssignment && event.ParentTaskAgentID != "" && event.ParentTaskAgentID != event.TaskAgentID {
			relay, err := effect(coordination.EffectSendRelay, "board:assignment:"+event.ID, coordination.RelayCommand{
				SenderID: event.ParentAgentID, SenderTaskAgentID: event.ParentTaskAgentID,
				RecipientID: event.AgentID, RecipientTaskAgentID: event.TaskAgentID, IssueID: event.IssueID,
				Body: "你收到了一条新的看板 Issue。请在手机中打开 Board，确认目标、上下文和验收要求后开始执行。",
			})
			if err != nil {
				return nil, err
			}
			result = append(result, relay)
		} else if notifyAssignment {
			// Root Issues have no parent TaskAgent to send a Relay message. Deliver
			// the assignment directly so the newly created root TaskAgent receives
			// the same task-local wake/steer signal as delegated TaskAgents.
			assignment, err := effect(coordination.EffectDeliverMessage, "board:assignment-delivery:"+event.ID, coordination.AgentCommand{
				AgentID: event.AgentID, TaskAgentID: event.TaskAgentID, ExecutionID: event.ExecutionID, IssueID: event.IssueID,
				Message: "你收到了一条新的看板 Issue。请在手机中打开 Board，确认目标、上下文和验收要求后开始执行。", Delivery: "steer",
			})
			if err != nil {
				return nil, err
			}
			result = append(result, assignment)
		}
		heartbeat, err := heartbeatEffect(event, config.HeartbeatIntervalSeconds, "board:heartbeat:init:"+event.ID)
		if err != nil {
			return nil, err
		}
		result = append(result, heartbeat)
		return result, nil

	case coordination.EventWaitRequested:
		request, err := decode[coordination.WaitRequest](event.Payload)
		if err != nil {
			return nil, err
		}
		seconds := request.WakeAfterSeconds
		if seconds <= 0 {
			seconds = config.HeartbeatIntervalSeconds
		}
		wake := coordination.WakeupPayload{
			Kind: "sleep", RequestedAt: event.OccurredAt, WakeAfterSeconds: seconds,
			ChildIDs: request.ChildIDs, Message: request.Message,
		}
		return one(wakeupEffect(event, wake, event.OccurredAt.Add(time.Duration(seconds)*time.Second), "board:sleep:"+event.ID))

	case coordination.EventTimerFired:
		if snapshot.Current == nil || snapshot.Current.Terminal() {
			return nil, nil
		}
		wake, err := decode[coordination.WakeupPayload](event.Payload)
		if err != nil {
			return nil, err
		}
		if wake.Kind == "sleep" && (snapshot.Current.ExecutionPhase != "sleeping" || snapshot.Current.SleepToken != event.CorrelationID) {
			// A comment, Relay message, or heartbeat may have woken this Agent
			// before its timer. A later sleep has a different token, so an old
			// timer can never wake the new sleep cycle.
			return nil, nil
		}
		if wake.Kind == "heartbeat" && !heartbeatShouldWake(snapshot.Current) {
			// A heartbeat is a durable liveness check, not periodic prompt input.
			// Steering a live model loop once per minute bloats its conversation and
			// consumes the per-Execution model-turn budget without adding new work.
			// Keep the timer chain alive and wake only an Agent that has released its
			// loop to wait or sleep.
			follow, err := heartbeatEffect(event, config.HeartbeatIntervalSeconds, "board:heartbeat:next:"+event.ID)
			if err != nil {
				return nil, err
			}
			return []coordination.PlannedEffect{follow}, nil
		}
		message := wake.Message
		if wake.Kind == "heartbeat" {
			message = heartbeatMessage(event, snapshot, wake)
		} else if strings.TrimSpace(message) == "" {
			message = "你主动设置的休息时间已经结束。请打开手机查看 Board 和 Relay 的最新变化，然后继续当前 Issue 中价值最高的工作。"
		}
		deliver, err := effect(coordination.EffectDeliverMessage, "board:wake:delivery:"+event.ID, coordination.AgentCommand{
			AgentID: event.AgentID, TaskAgentID: event.TaskAgentID, ExecutionID: event.ExecutionID, IssueID: event.IssueID,
			Message: message, Delivery: "steer",
		})
		if err != nil {
			return nil, err
		}
		result := []coordination.PlannedEffect{deliver}
		if wake.Kind == "heartbeat" {
			follow, err := heartbeatEffect(event, config.HeartbeatIntervalSeconds, "board:heartbeat:next:"+event.ID)
			if err != nil {
				return nil, err
			}
			result = append(result, follow)
		}
		return result, nil

	case coordination.EventIssueCompleted, coordination.EventExecutionCompleted:
		if event.ParentAgentID == "" {
			return nil, nil
		}
		completed, err := decode[coordination.Completion](event.Payload)
		if err != nil {
			return nil, err
		}
		message := completionMessage(event, completed)
		relay, err := effect(coordination.EffectSendRelay, "board:completed:relay:"+event.ID, coordination.RelayCommand{
			SenderID: event.AgentID, SenderTaskAgentID: event.TaskAgentID,
			RecipientID: event.ParentAgentID, RecipientTaskAgentID: event.ParentTaskAgentID, IssueID: event.IssueID, Body: message,
		})
		if err != nil {
			return nil, err
		}
		deliver, err := effect(coordination.EffectDeliverMessage, "board:completed:delivery:"+event.ID, coordination.AgentCommand{
			AgentID: event.ParentAgentID, TaskAgentID: event.ParentTaskAgentID, IssueID: event.ParentIssueID, Message: message, Delivery: "steer",
		})
		if err != nil {
			return nil, err
		}
		return []coordination.PlannedEffect{relay, deliver}, nil

	case coordination.EventRelayReceived:
		if event.AgentID == "" {
			return nil, nil
		}
		relay, err := decode[coordination.RelayReceived](event.Payload)
		if err != nil {
			return nil, err
		}
		channel := "Relay 消息"
		followup := "稍后从手机 Relay 中读取"
		if relay.Channel == "board_comment" {
			channel = "Board 评论"
			followup = "稍后从手机 Board 中查看"
		}
		message := fmt.Sprintf("手机收到来自 %s 的%s（messageId: %s）：\n\n%s\n\n这是任务内消息通知。请自行判断是立即处理、回复、调整当前工作，还是%s。", fallbackText(relay.SenderName, fallbackText(relay.SenderID, "未知发送者")), channel, relay.MessageID, relay.Body, followup)
		return one(effect(coordination.EffectDeliverMessage, "board:relay-delivery:"+event.ID, coordination.AgentCommand{
			AgentID: event.AgentID, TaskAgentID: event.TaskAgentID, IssueID: event.IssueID, Message: message, Delivery: "steer",
		}))
	default:
		return nil, nil
	}
}

func heartbeatShouldWake(item *coordination.WorkItem) bool {
	if item == nil {
		return false
	}
	return item.ExecutionPhase == "sleeping" || item.ExecutionPhase == "waiting_children"
}

func heartbeatEffect(event coordination.Event, seconds int64, key string) (coordination.PlannedEffect, error) {
	if seconds <= 0 {
		seconds = 60
	}
	wake := coordination.WakeupPayload{Kind: "heartbeat", RequestedAt: event.OccurredAt, WakeAfterSeconds: seconds}
	base := event.OccurredAt
	if base.IsZero() {
		base = time.Now().UTC()
		wake.RequestedAt = base
	}
	return wakeupEffect(event, wake, base.Add(time.Duration(seconds)*time.Second), key)
}

func wakeupEffect(event coordination.Event, wake coordination.WakeupPayload, availableAt time.Time, key string) (coordination.PlannedEffect, error) {
	raw, err := payload(wake)
	if err != nil {
		return coordination.PlannedEffect{}, err
	}
	command := coordination.ScheduleWakeupCommand{Event: coordination.Event{
		Type: coordination.EventTimerFired, CoordinationID: event.CoordinationID, TaskID: event.TaskID,
		IssueID: event.IssueID, ParentIssueID: event.ParentIssueID, ExecutionID: event.ExecutionID,
		AgentID: event.AgentID, TaskAgentID: event.TaskAgentID, ParentAgentID: event.ParentAgentID, ParentTaskAgentID: event.ParentTaskAgentID, CorrelationID: event.ID, Payload: raw,
	}}
	result, err := effect(coordination.EffectScheduleWakeup, key, command)
	result.AvailableAt = availableAt
	return result, err
}

func heartbeatMessage(event coordination.Event, snapshot coordination.Snapshot, wake coordination.WakeupPayload) string {
	elapsed := event.OccurredAt.Sub(wake.RequestedAt)
	if elapsed < 0 {
		elapsed = 0
	}
	statusCounts := map[coordination.WorkStatus]int{}
	for _, child := range snapshot.Children {
		statusCounts[child.Status]++
	}
	childDetails := make([]string, 0, len(snapshot.Children))
	for _, child := range snapshot.Children {
		owner := fallbackText(child.TaskAgentName, fallbackText(child.AssigneeID, "未分配"))
		progress := fallbackText(child.ProgressSummary, "尚无进度摘要")
		if child.CurrentActivity != "" {
			progress += "；当前：" + child.CurrentActivity
		}
		childDetails = append(childDetails, fmt.Sprintf("- %s · %s · %s（%s）：%s", child.ID, owner, fallbackText(child.Title, "未命名 Issue"), child.Status, progress))
	}
	if len(childDetails) == 0 {
		childDetails = append(childDetails, "- 暂无直属子 Issue")
	}
	return fmt.Sprintf(`## 任务心跳

距离上次计划心跳已经过去 %s。当前 Issue 状态：%s；直属子 Issue 共 %d 个（运行 %d、等待 %d、完成 %d、失败 %d）。

直属子 Issue 最新快照：
%s

这不是新任务。请打开手机查看 Board 与 Relay 的最新变化，然后自行选择价值最高的动作：
- 继续完成你自己的 Issue；
- 检查你拆出的子 Issue 和执行进度；
- 通过 Issue 评论或 Relay 询问、指导、纠偏；
- 必要时停止错误方向的子 Issue，再给出新的清晰要求；
- 如果暂时没有可做事项，可再次使用 coordinate_sleep 设置休息时间。

消息和评论只提供信息，不强制要求回复；是否回复由你根据任务价值自行判断。`,
		formatDuration(elapsed), snapshot.Current.Status, len(snapshot.Children), statusCounts[coordination.WorkRunning], statusCounts[coordination.WorkWaiting], statusCounts[coordination.WorkSucceeded], statusCounts[coordination.WorkFailed], strings.Join(childDetails, "\n"))
}

func formatDuration(value time.Duration) string {
	seconds := int64(value.Round(time.Second) / time.Second)
	if seconds < 60 {
		return fmt.Sprintf("%d 秒", seconds)
	}
	minutes := seconds / 60
	if minutes < 60 {
		return fmt.Sprintf("%d 分 %d 秒", minutes, seconds%60)
	}
	return fmt.Sprintf("%d 小时 %d 分", minutes/60, minutes%60)
}

func fallbackText(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return strings.TrimSpace(value)
}
