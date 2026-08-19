package boardcoordination

import (
	"time"

	"aegis/capability"
	platformcoordination "aegis/coordination"
)

const (
	EventDelegationRequested platformcoordination.EventType = "delegation_requested"
	EventIssueCreated        platformcoordination.EventType = "issue_created"
	EventIssueAssigned       platformcoordination.EventType = "issue_assigned"
	EventIssueCompleted      platformcoordination.EventType = "issue_completed"
	EventExecutionCompleted  platformcoordination.EventType = "execution_completed"
	EventRelayReceived       platformcoordination.EventType = "relay_received"
	EventWaitRequested       platformcoordination.EventType = "wait_requested"
	EventAgentStopped        platformcoordination.EventType = "agent_stopped"
)

const (
	EffectCreateIssue    platformcoordination.EffectType = "create_issue"
	EffectAssignIssue    platformcoordination.EffectType = "assign_issue"
	EffectEnqueueIssue   platformcoordination.EffectType = "enqueue_issue"
	EffectStartSubagent  platformcoordination.EffectType = "start_subagent"
	EffectSuspendAgent   platformcoordination.EffectType = "suspend_agent"
	EffectResumeAgent    platformcoordination.EffectType = "resume_agent"
	EffectDeliverMessage platformcoordination.EffectType = "deliver_message"
	EffectSendRelay      platformcoordination.EffectType = "send_relay"
	EffectCancelWork     platformcoordination.EffectType = "cancel_work"
)

type ChildWork struct {
	ID                  string                                   `json:"id,omitempty"`
	AgentID             string                                   `json:"agentId"`
	TaskAgentID         string                                   `json:"taskAgentId,omitempty"`
	Title               string                                   `json:"title,omitempty"`
	Prompt              string                                   `json:"prompt"`
	Priority            string                                   `json:"priority,omitempty"`
	Workspace           string                                   `json:"workspace,omitempty"`
	Capabilities        []capability.Ref                         `json:"capabilities,omitempty"`
	CapabilitySelection platformcoordination.CapabilitySelection `json:"capabilitySelection,omitempty"`
}

type DelegationRequest struct {
	Children       []ChildWork `json:"children"`
	ParentBehavior string      `json:"parentBehavior,omitempty"`
	ResultDelivery string      `json:"resultDelivery,omitempty"`
}

type Invocation struct {
	EventID     string `json:"eventId"`
	IssueID     string `json:"issueId"`
	ExecutionID string `json:"executionId"`
	AgentID     string `json:"agentId"`
}

type AgentInvocation struct {
	IssueID           string    `json:"issueId"`
	ParentAgentID     string    `json:"parentAgentId,omitempty"`
	ParentExecutionID string    `json:"parentExecutionId,omitempty"`
	Child             ChildWork `json:"child"`
	ParentBehavior    string    `json:"parentBehavior,omitempty"`
	ResultDelivery    string    `json:"resultDelivery,omitempty"`
}

type InvocationReceipt struct {
	EventID        string `json:"eventId"`
	CoordinationID string `json:"coordinationId"`
	IssueID        string `json:"issueId"`
	AgentID        string `json:"agentId"`
	Accepted       bool   `json:"accepted"`
}

type Completion struct {
	Result         string `json:"result,omitempty"`
	Success        bool   `json:"success"`
	Status         string `json:"status,omitempty"`
	Error          string `json:"error,omitempty"`
	ChildID        string `json:"childId,omitempty"`
	Message        string `json:"message,omitempty"`
	ParentBehavior string `json:"parentBehavior,omitempty"`
	ResultDelivery string `json:"resultDelivery,omitempty"`
}

type WaitRequest struct {
	ChildIDs         []string `json:"childIds,omitempty"`
	WakeAfterSeconds int64    `json:"wakeAfterSeconds,omitempty"`
	Message          string   `json:"message,omitempty"`
}

type ContinueRequest struct {
	ChildIssueID string `json:"childIssueId"`
	Reason       string `json:"reason"`
}

type WakeupPayload struct {
	Kind             string    `json:"kind"`
	RequestedAt      time.Time `json:"requestedAt"`
	ChildIDs         []string  `json:"childIds,omitempty"`
	WakeAfterSeconds int64     `json:"wakeAfterSeconds"`
	Message          string    `json:"message,omitempty"`
}

type RelayReceived struct {
	Channel           string `json:"channel,omitempty"`
	MessageID         string `json:"messageId,omitempty"`
	SenderID          string `json:"senderId,omitempty"`
	SenderTaskAgentID string `json:"senderTaskAgentId,omitempty"`
	SenderName        string `json:"senderName,omitempty"`
	Body              string `json:"body,omitempty"`
}

type EnqueueIssueCommand struct {
	IssueID string `json:"issueId"`
	AgentID string `json:"agentId,omitempty"`
}

type CreateIssueCommand struct {
	ParentIssueID     string    `json:"parentIssueId,omitempty"`
	ParentAgentID     string    `json:"parentAgentId,omitempty"`
	ParentTaskAgentID string    `json:"parentTaskAgentId,omitempty"`
	Child             ChildWork `json:"child"`
}

type StartSubagentCommand struct {
	CoordinationID    string    `json:"coordinationId"`
	ParentIssueID     string    `json:"parentIssueId,omitempty"`
	ParentExecutionID string    `json:"parentExecutionId,omitempty"`
	ParentAgentID     string    `json:"parentAgentId,omitempty"`
	CorrelationID     string    `json:"correlationId"`
	ParentBehavior    string    `json:"parentBehavior,omitempty"`
	ResultDelivery    string    `json:"resultDelivery,omitempty"`
	Child             ChildWork `json:"child"`
}

type AgentCommand struct {
	CommandID   string `json:"commandId,omitempty"`
	AgentID     string `json:"agentId"`
	TaskAgentID string `json:"taskAgentId,omitempty"`
	ExecutionID string `json:"executionId,omitempty"`
	IssueID     string `json:"issueId,omitempty"`
	Message     string `json:"message,omitempty"`
	Delivery    string `json:"delivery,omitempty"`
}

type RelayCommand struct {
	SenderID             string `json:"senderId,omitempty"`
	SenderTaskAgentID    string `json:"senderTaskAgentId,omitempty"`
	RecipientID          string `json:"recipientId"`
	RecipientTaskAgentID string `json:"recipientTaskAgentId,omitempty"`
	IssueID              string `json:"issueId,omitempty"`
	Body                 string `json:"body"`
}
