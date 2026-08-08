package control

import (
	"time"

	"aegis/capability"
)

type IssueBudgetConfig struct {
	MaxTurns           int `json:"maxTurns"`
	ActiveTimeMinutes  int `json:"activeTimeMinutes"`
	SummaryTurns       int `json:"summaryTurns"`
	SummaryTimeMinutes int `json:"summaryTimeMinutes"`
}

type IssueHeartbeatConfig struct {
	IntervalSeconds int `json:"intervalSeconds"`
}

type WebSearchConfig struct {
	Engine  string `json:"engine"`
	BaseURL string `json:"baseUrl"`
	APIKey  string `json:"apiKey,omitempty"`
	Enabled bool   `json:"enabled"`
}

type WebSearchConfigView struct {
	Engine    string `json:"engine"`
	BaseURL   string `json:"baseUrl"`
	HasAPIKey bool   `json:"hasApiKey"`
	Enabled   bool   `json:"enabled"`
}

type Config struct {
	Configured            bool                 `json:"configured"`
	Language              string               `json:"language"`
	Provider              string               `json:"provider"`
	Model                 string               `json:"model"`
	Pricing               ModelPricing         `json:"pricing"`
	BaseURL               string               `json:"baseUrl"`
	Thinking              string               `json:"thinking"`
	AuthMode              string               `json:"authMode"`
	APIKey                string               `json:"apiKey,omitempty"`
	Workspace             string               `json:"workspace"`
	Concurrency           int                  `json:"concurrency"`
	ApprovalMode          string               `json:"approvalMode"`
	ReworkApprovalMode    string               `json:"reworkApprovalMode"`
	ValidationMode        string               `json:"validationMode"`
	MaxValidationAttempts int                  `json:"maxValidationAttempts"`
	MaxIssueDepth         int                  `json:"maxIssueDepth"`
	MaxChildrenPerRequest int                  `json:"maxChildrenPerRequest"`
	MaxDirectChildren     int                  `json:"maxDirectChildren"`
	IssueBudget           IssueBudgetConfig    `json:"issueBudget"`
	IssueHeartbeat        IssueHeartbeatConfig `json:"issueHeartbeat"`
	WebSearch             WebSearchConfig      `json:"webSearch"`
	UpdatedAt             time.Time            `json:"updatedAt"`
}

type ConfigView struct {
	Configured            bool                 `json:"configured"`
	Language              string               `json:"language"`
	Provider              string               `json:"provider"`
	Model                 string               `json:"model"`
	Pricing               ModelPricing         `json:"pricing"`
	BaseURL               string               `json:"baseUrl"`
	Thinking              string               `json:"thinking"`
	AuthMode              string               `json:"authMode"`
	HasAPIKey             bool                 `json:"hasApiKey"`
	Workspace             string               `json:"workspace"`
	Concurrency           int                  `json:"concurrency"`
	ApprovalMode          string               `json:"approvalMode"`
	ReworkApprovalMode    string               `json:"reworkApprovalMode"`
	ValidationMode        string               `json:"validationMode"`
	MaxValidationAttempts int                  `json:"maxValidationAttempts"`
	MaxIssueDepth         int                  `json:"maxIssueDepth"`
	MaxChildrenPerRequest int                  `json:"maxChildrenPerRequest"`
	MaxDirectChildren     int                  `json:"maxDirectChildren"`
	IssueBudget           IssueBudgetConfig    `json:"issueBudget"`
	IssueHeartbeat        IssueHeartbeatConfig `json:"issueHeartbeat"`
	WebSearch             WebSearchConfigView  `json:"webSearch"`
	UpdatedAt             time.Time            `json:"updatedAt"`
}

type Project struct {
	ID          string    `json:"id" gorm:"primaryKey"`
	Key         string    `json:"key" gorm:"uniqueIndex"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Workspace   string    `json:"workspace"`
	Status      string    `json:"status" gorm:"index"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

type ContainerProfile struct {
	ID            string    `json:"id" gorm:"primaryKey"`
	Name          string    `json:"name"`
	Description   string    `json:"description" gorm:"type:text"`
	Image         string    `json:"image"`
	WorkspacePath string    `json:"workspacePath"`
	NetworkMode   string    `json:"networkMode"`
	MemoryMB      int       `json:"memoryMb"`
	CPUs          float64   `json:"cpus"`
	Enabled       bool      `json:"enabled" gorm:"index"`
	CreatedAt     time.Time `json:"createdAt"`
	UpdatedAt     time.Time `json:"updatedAt"`
}

// ContainerInstance is a durable task runtime created from a ContainerProfile.
// The runtime configuration is snapshotted so later profile edits only affect
// containers created for future tasks.
type ContainerInstance struct {
	ID                 string    `json:"id" gorm:"primaryKey"`
	ContainerProfileID string    `json:"containerProfileId" gorm:"index"`
	TaskID             string    `json:"taskId" gorm:"uniqueIndex"`
	Name               string    `json:"name" gorm:"uniqueIndex"`
	Image              string    `json:"image"`
	WorkspacePath      string    `json:"workspacePath"`
	NetworkMode        string    `json:"networkMode"`
	MemoryMB           int       `json:"memoryMb"`
	CPUs               float64   `json:"cpus"`
	RuntimeStatus      string    `json:"runtimeStatus" gorm:"-"`
	CreatedAt          time.Time `json:"createdAt"`
	UpdatedAt          time.Time `json:"updatedAt"`
}

type SaveContainerProfileInput struct {
	Name          string  `json:"name"`
	Description   string  `json:"description"`
	Image         string  `json:"image"`
	WorkspacePath string  `json:"workspacePath"`
	NetworkMode   string  `json:"networkMode"`
	MemoryMB      int     `json:"memoryMb"`
	CPUs          float64 `json:"cpus"`
	Enabled       bool    `json:"enabled"`
}

type ContainerProfileDeleteImpact struct {
	ContainerProfileID   string `json:"containerProfileId"`
	ContainerCount       int    `json:"containerCount"`
	IssueCount           int    `json:"issueCount"`
	TaskCount            int    `json:"taskCount"`
	ExecutionCount       int    `json:"executionCount"`
	ActiveExecutionCount int    `json:"activeExecutionCount"`
}

type ContainerDeleteImpact struct {
	ContainerID          string `json:"containerId"`
	TaskID               string `json:"taskId"`
	IssueCount           int    `json:"issueCount"`
	ExecutionCount       int    `json:"executionCount"`
	ActiveExecutionCount int    `json:"activeExecutionCount"`
}

type ContainerDeleteResult struct {
	ContainerID       string `json:"containerId"`
	DeletedIssues     int64  `json:"deletedIssues"`
	DeletedTasks      int64  `json:"deletedTasks"`
	DeletedExecutions int64  `json:"deletedExecutions"`
}

type ContainerBatchFailure struct {
	ContainerID string `json:"containerId"`
	Error       string `json:"error"`
}

type ContainerBatchStopResult struct {
	Requested int                     `json:"requested"`
	Stopped   []ContainerInstance     `json:"stopped"`
	Failed    []ContainerBatchFailure `json:"failed"`
}

type ContainerBatchDeleteImpact struct {
	ContainerIDs         []string                `json:"containerIds"`
	ContainerCount       int                     `json:"containerCount"`
	TaskCount            int                     `json:"taskCount"`
	IssueCount           int                     `json:"issueCount"`
	ExecutionCount       int                     `json:"executionCount"`
	ActiveExecutionCount int                     `json:"activeExecutionCount"`
	Items                []ContainerDeleteImpact `json:"items"`
}

type ContainerBatchDeleteResult struct {
	Requested         int                     `json:"requested"`
	Deleted           []ContainerDeleteResult `json:"deleted"`
	Failed            []ContainerBatchFailure `json:"failed"`
	DeletedIssues     int64                   `json:"deletedIssues"`
	DeletedTasks      int64                   `json:"deletedTasks"`
	DeletedExecutions int64                   `json:"deletedExecutions"`
}

type ContainerProfileDeleteResult struct {
	ContainerProfileID string `json:"containerProfileId"`
	DeletedIssues      int64  `json:"deletedIssues"`
	DeletedTasks       int64  `json:"deletedTasks"`
	DeletedExecutions  int64  `json:"deletedExecutions"`
}

// Task is a reusable work definition. Each run creates a new root Issue that
// points back to the Task through TaskSourceID.
type Task struct {
	ID                      string    `json:"id" gorm:"primaryKey"`
	ProjectID               string    `json:"projectId" gorm:"index"`
	Title                   string    `json:"title"`
	Description             string    `json:"description"`
	Objective               string    `json:"objective" gorm:"type:text"`
	Priority                string    `json:"priority"`
	WorkMode                string    `json:"workMode"`
	AssigneeAgentID         string    `json:"assigneeAgentId,omitempty" gorm:"index"`
	Workspace               string    `json:"workspace"`
	ContainerProfileID      string    `json:"containerProfileId,omitempty" gorm:"index"`
	ContainerID             string    `json:"containerId,omitempty" gorm:"index"`
	TimeBudgetMinutes       *int      `json:"timeBudgetMinutes,omitempty"`
	HumanValidationFallback bool      `json:"humanValidationFallback"`
	CreatedAt               time.Time `json:"createdAt"`
	UpdatedAt               time.Time `json:"updatedAt"`
}

// TaskDetail contains Task-owned data only. Issue executions and their
// histories remain available through the Task timeline and Issue pages.
type TaskDetail struct {
	Task             Task              `json:"task"`
	InputAttachments []InputAttachment `json:"inputAttachments"`
	RootReports      []TaskRootReport  `json:"rootReports"`
}

// TaskRootReport is the Task Home projection for one root Issue and its latest
// accepted (or validation-skipped) user-facing delivery bundle.
type TaskRootReport struct {
	IssueID     string       `json:"issueId"`
	Identifier  string       `json:"identifier"`
	Title       string       `json:"title"`
	Status      string       `json:"status"`
	CompletedAt *time.Time   `json:"completedAt,omitempty"`
	Reports     []TaskReport `json:"reports"`
}

// TaskReport is one immutable, lifecycle-generated ZIP delivery. Repeated
// completions of the same root Issue append history instead of replacing it.
type TaskReport struct {
	ID                string    `json:"id" gorm:"primaryKey"`
	TaskID            string    `json:"taskId" gorm:"index"`
	RootIssueID       string    `json:"rootIssueId" gorm:"index"`
	SourceExecutionID string    `json:"sourceExecutionId" gorm:"index"`
	Version           int       `json:"version"`
	Title             string    `json:"title"`
	Name              string    `json:"name"`
	StoragePath       string    `json:"-"`
	MimeType          string    `json:"mimeType"`
	Size              int64     `json:"size"`
	AttachmentCount   int       `json:"attachmentCount"`
	CreatedAt         time.Time `json:"createdAt"`
	UpdatedAt         time.Time `json:"updatedAt"`
}

// TaskAudit is one immutable evaluation attempt over a point-in-time Task
// evidence snapshot. Events carry the evaluator's live trace, ReportMarkdown
// stores only the validated final deliverable, and EvidencePath keeps the exact
// input used for later reproduction.
type TaskAudit struct {
	ID               string           `json:"id" gorm:"primaryKey"`
	TaskID           string           `json:"taskId,omitempty" gorm:"index"`
	RequestedID      string           `json:"requestedId" gorm:"index"`
	RootIssueIDs     []string         `json:"rootIssueIds" gorm:"serializer:json;type:text"`
	Status           string           `json:"status" gorm:"index"`
	FrameworkVersion string           `json:"frameworkVersion"`
	EvidenceExportID string           `json:"evidenceExportId,omitempty"`
	EvidenceComplete bool             `json:"evidenceComplete"`
	EvidenceManifest string           `json:"evidenceManifest,omitempty" gorm:"type:text"`
	EvidencePath     string           `json:"-"`
	ReportMarkdown   string           `json:"reportMarkdown,omitempty" gorm:"type:text"`
	Error            string           `json:"error,omitempty" gorm:"type:text"`
	InputTokens      int              `json:"inputTokens"`
	OutputTokens     int              `json:"outputTokens"`
	StartedAt        *time.Time       `json:"startedAt,omitempty"`
	CompletedAt      *time.Time       `json:"completedAt,omitempty"`
	CreatedAt        time.Time        `json:"createdAt" gorm:"index"`
	UpdatedAt        time.Time        `json:"updatedAt"`
	Events           []TaskAuditEvent `json:"events,omitempty" gorm:"-"`
}

// TaskAuditEvent is one durable, UI-facing step in an audit Agent trace. Text
// and tool argument deltas update the same row so observability stays useful
// without producing one database record per token.
type TaskAuditEvent struct {
	ID         string    `json:"id" gorm:"primaryKey"`
	AuditID    string    `json:"auditId" gorm:"index:idx_task_audit_events_order,priority:1"`
	Sequence   int64     `json:"sequence" gorm:"index:idx_task_audit_events_order,priority:2"`
	Kind       string    `json:"kind"`
	Status     string    `json:"status"`
	Turn       int       `json:"turn,omitempty"`
	ToolCallID string    `json:"toolCallId,omitempty" gorm:"index"`
	ToolName   string    `json:"toolName,omitempty"`
	Content    string    `json:"content,omitempty" gorm:"type:text"`
	Arguments  string    `json:"arguments,omitempty" gorm:"type:text"`
	Result     string    `json:"result,omitempty" gorm:"type:text"`
	IsError    bool      `json:"isError,omitempty"`
	CreatedAt  time.Time `json:"createdAt" gorm:"index"`
	UpdatedAt  time.Time `json:"updatedAt"`
}

// Issue is the single source of truth for one run or a child execution unit.
type Issue struct {
	ID                      string           `json:"id" gorm:"primaryKey"`
	Number                  int64            `json:"number" gorm:"uniqueIndex"`
	Identifier              string           `json:"identifier" gorm:"uniqueIndex"`
	ProjectID               string           `json:"projectId" gorm:"index"`
	ParentID                string           `json:"parentId,omitempty" gorm:"index"`
	TaskSourceID            string           `json:"taskSourceId,omitempty" gorm:"index"`
	Title                   string           `json:"title"`
	Description             string           `json:"description"`
	Objective               string           `json:"objective" gorm:"type:text"`
	Status                  string           `json:"status" gorm:"index"`
	Labels                  []string         `json:"labels,omitempty" gorm:"serializer:json;type:text"`
	Priority                string           `json:"priority" gorm:"index"`
	WorkMode                string           `json:"workMode"`
	ExecutionPhase          string           `json:"executionPhase" gorm:"index"`
	SleepToken              string           `json:"-" gorm:"index"`
	RequestDepth            int              `json:"requestDepth"`
	AssigneeAgentID         string           `json:"assigneeAgentId,omitempty" gorm:"index"`
	AssigneeTaskAgentID     string           `json:"assigneeTaskAgentId,omitempty" gorm:"index"`
	Capabilities            []capability.Ref `json:"capabilities,omitempty" gorm:"serializer:json;type:text"`
	CapabilitySelection     string           `json:"capabilitySelection,omitempty"`
	CheckoutExecutionID     string           `json:"checkoutExecutionId,omitempty" gorm:"index"`
	CurrentExecutionID      string           `json:"currentExecutionId,omitempty" gorm:"index"`
	ValidationExecutionID   string           `json:"validationExecutionId,omitempty" gorm:"index"`
	ValidationMode          string           `json:"validationMode" gorm:"default:fixed"`
	MaxValidationAttempts   int              `json:"maxValidationAttempts" gorm:"default:3"`
	ValidationDisabled      bool             `json:"validationDisabled" gorm:"index"`
	RecoveryExecutionID     string           `json:"recoveryExecutionId,omitempty" gorm:"index"`
	RecoveryPhase           string           `json:"recoveryPhase,omitempty"`
	RecoveryRequestedAt     *time.Time       `json:"recoveryRequestedAt,omitempty"`
	Workspace               string           `json:"workspace"`
	ContainerProfileID      string           `json:"containerProfileId,omitempty" gorm:"index"`
	ContainerID             string           `json:"containerId,omitempty" gorm:"index"`
	TimeBudgetMinutes       *int             `json:"timeBudgetMinutes,omitempty"`
	HumanValidationFallback bool             `json:"humanValidationFallback"`
	Result                  string           `json:"result,omitempty"`
	Error                   string           `json:"error,omitempty"`
	ObjectiveAbandoned      bool             `json:"objectiveAbandoned" gorm:"index"`
	Hidden                  bool             `json:"-" gorm:"index"`
	AbandonmentReason       string           `json:"abandonmentReason,omitempty" gorm:"type:text"`
	AbandonRequestedAt      *time.Time       `json:"abandonRequestedAt,omitempty"`
	AbandonedAt             *time.Time       `json:"abandonedAt,omitempty"`
	CreatedBy               string           `json:"createdBy"`
	StartedAt               *time.Time       `json:"startedAt,omitempty"`
	CompletedAt             *time.Time       `json:"completedAt,omitempty"`
	CancelledAt             *time.Time       `json:"cancelledAt,omitempty"`
	CreatedAt               time.Time        `json:"createdAt"`
	UpdatedAt               time.Time        `json:"updatedAt"`
}

// IssueRuntimeView is a read-only projection of the durable records that own
// an Issue's current activity. It is deliberately not persisted: API clients
// must render this projection instead of independently interpreting the
// partially overlapping Issue, Execution, wait, validation, and recovery
// fields.
type IssueRuntimeView struct {
	IssueID                string    `json:"issueId"`
	State                  string    `json:"state"`
	Kind                   string    `json:"kind"`
	Health                 string    `json:"health"`
	CurrentExecutionID     string    `json:"currentExecutionId,omitempty"`
	CurrentExecutionStatus string    `json:"currentExecutionStatus,omitempty"`
	Detail                 string    `json:"detail,omitempty"`
	UpdatedAt              time.Time `json:"updatedAt"`
}

// ConciergeConversation is a user-facing, persistent chat with the built-in
// concierge Agent. IssueID points at an internal hidden Issue used only to
// reuse the durable Pi execution/session machinery without polluting Tasks.
type ConciergeConversation struct {
	ID            string    `json:"id" gorm:"primaryKey"`
	Title         string    `json:"title"`
	IssueID       string    `json:"-" gorm:"uniqueIndex"`
	ExecutionID   string    `json:"executionId" gorm:"uniqueIndex"`
	Status        string    `json:"status" gorm:"index"`
	LastMessage   string    `json:"lastMessage,omitempty" gorm:"type:text"`
	MessageCount  int       `json:"messageCount"`
	CreatedTaskID string    `json:"createdTaskId,omitempty" gorm:"index"`
	CreatedAt     time.Time `json:"createdAt"`
	UpdatedAt     time.Time `json:"updatedAt" gorm:"index"`
}

type ConciergeConversationDetail struct {
	Conversation ConciergeConversation `json:"conversation"`
	Execution    Execution             `json:"execution"`
	Messages     []Message             `json:"messages"`
	MessagesPage PageInfo              `json:"messagesPage"`
	Watermark    time.Time             `json:"watermark"`
}

type CreateConciergeTaskInput struct {
	Title           string `json:"title"`
	Description     string `json:"taskDescription"`
	Objective       string `json:"objective"`
	Priority        string `json:"priority"`
	WorkMode        string `json:"workMode"`
	AssigneeAgentID string `json:"agentId"`
	Workspace       string `json:"workspace"`
}

// IssueRelation keeps dependency edges independent from the parent/child hierarchy.
type IssueRelation struct {
	ID             string    `json:"id" gorm:"primaryKey"`
	IssueID        string    `json:"issueId" gorm:"uniqueIndex:idx_issue_relation"`
	RelatedIssueID string    `json:"relatedIssueId" gorm:"uniqueIndex:idx_issue_relation"`
	Type           string    `json:"type" gorm:"uniqueIndex:idx_issue_relation"`
	CreatedAt      time.Time `json:"createdAt"`
	UpdatedAt      time.Time `json:"updatedAt"`
}

type Execution struct {
	ID                   string                `json:"id" gorm:"primaryKey"`
	IssueID              string                `json:"issueId" gorm:"index;index:idx_executions_issue_started,priority:1"`
	AgentID              string                `json:"agentId" gorm:"index"`
	TaskAgentID          string                `json:"taskAgentId,omitempty" gorm:"index"`
	Kind                 string                `json:"kind"`
	Status               string                `json:"status" gorm:"index"`
	Provider             string                `json:"provider"`
	Model                string                `json:"model"`
	Pricing              ModelPricing          `json:"pricing" gorm:"serializer:json;type:text"`
	Thinking             string                `json:"thinking"`
	SessionID            string                `json:"sessionId" gorm:"index"`
	PID                  int                   `json:"pid,omitempty" gorm:"column:pid"`
	RuntimeType          string                `json:"runtimeType" gorm:"index"`
	RuntimeID            string                `json:"runtimeId,omitempty"`
	ContainerProfileID   string                `json:"containerProfileId,omitempty" gorm:"index"`
	ContainerImage       string                `json:"containerImage,omitempty"`
	CurrentTool          string                `json:"currentTool,omitempty"`
	Checkpoint           string                `json:"checkpoint,omitempty" gorm:"type:text"`
	CheckpointAt         *time.Time            `json:"checkpointAt,omitempty"`
	InitialPrompt        string                `json:"initialPrompt,omitempty" gorm:"type:text"`
	SystemPrompt         string                `json:"systemPrompt,omitempty" gorm:"type:text"`
	ToolsSnapshot        []ToolSnapshot        `json:"toolsSnapshot" gorm:"serializer:json;type:text"`
	CapabilitiesSnapshot []capability.Snapshot `json:"capabilitiesSnapshot,omitempty" gorm:"serializer:json;type:text"`
	Result               string                `json:"result,omitempty"`
	FinalResult          string                `json:"finalResult,omitempty" gorm:"type:text"`
	FinalResultSubmitted bool                  `json:"finalResultSubmitted"`
	Error                string                `json:"error,omitempty"`
	Cost                 float64               `json:"cost"`
	Tokens               int64                 `json:"tokens"`
	InputTokens          int64                 `json:"inputTokens"`
	OutputTokens         int64                 `json:"outputTokens"`
	CacheReadTokens      int64                 `json:"cacheReadTokens"`
	CacheWriteTokens     int64                 `json:"cacheWriteTokens"`
	MessageCount         int                   `json:"messageCount"`
	BudgetMaxTurns       int                   `json:"budgetMaxTurns"`
	BudgetActiveMinutes  int                   `json:"budgetActiveMinutes"`
	BudgetSummaryTurns   int                   `json:"budgetSummaryTurns"`
	BudgetSummaryMinutes int                   `json:"budgetSummaryMinutes"`
	BudgetPhase          string                `json:"budgetPhase,omitempty" gorm:"index"`
	BudgetTurnsUsed      int                   `json:"budgetTurnsUsed"`
	BudgetSummaryUsed    int                   `json:"budgetSummaryUsed"`
	BudgetExceededReason string                `json:"budgetExceededReason,omitempty" gorm:"type:text"`
	BudgetExceededAt     *time.Time            `json:"budgetExceededAt,omitempty"`
	StartedAt            time.Time             `json:"startedAt" gorm:"index:idx_executions_issue_started,priority:2"`
	UpdatedAt            time.Time             `json:"updatedAt"`
	FinishedAt           *time.Time            `json:"finishedAt,omitempty"`
}

// TaskAgent is one task-scoped runtime identity leased from an Agent
// definition. Name mirrors the Agent type name; ID provides Session, Phone and
// Relay isolation when the same Agent type runs more than one Issue.
type TaskAgent struct {
	ID        string    `json:"id" gorm:"primaryKey"`
	TaskID    string    `json:"taskId" gorm:"index"`
	AgentID   string    `json:"agentId" gorm:"index"`
	Name      string    `json:"name"`
	Status    string    `json:"status" gorm:"index"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// IssueValidation is the durable hand-off between a worker Execution and the
// read-only acceptance Agent. It snapshots both sides of the decision so a
// later model or prompt change cannot rewrite the audit trail.
type IssueValidation struct {
	ID                    string     `json:"id" gorm:"primaryKey"`
	IssueID               string     `json:"issueId" gorm:"index"`
	SourceExecutionID     string     `json:"sourceExecutionId" gorm:"index"`
	ValidationExecutionID string     `json:"validationExecutionId" gorm:"index"`
	Attempt               int        `json:"attempt"`
	ObjectiveID           string     `json:"objectiveId,omitempty" gorm:"index"`
	Objective             string     `json:"objective" gorm:"type:text"`
	CandidateResult       string     `json:"candidateResult" gorm:"type:text"`
	Status                string     `json:"status" gorm:"index"`
	Passed                bool       `json:"passed"`
	Summary               string     `json:"summary" gorm:"type:text"`
	Feedback              string     `json:"feedback" gorm:"type:text"`
	AbandonmentProof      string     `json:"abandonmentProof,omitempty" gorm:"type:text"`
	DecisionJSON          string     `json:"decisionJson,omitempty" gorm:"type:text"`
	ManualOverrideReason  string     `json:"manualOverrideReason,omitempty" gorm:"type:text"`
	Error                 string     `json:"error,omitempty" gorm:"type:text"`
	CreatedAt             time.Time  `json:"createdAt"`
	CompletedAt           *time.Time `json:"completedAt,omitempty"`
}

// IssueObjective is one immutable revision of an Issue's acceptance target.
// Issue.Objective remains the current projection used by runtime prompts.
type IssueObjective struct {
	ID              string    `json:"id" gorm:"primaryKey"`
	IssueID         string    `json:"issueId" gorm:"uniqueIndex:idx_issue_objective_version,priority:1;index"`
	Version         int       `json:"version" gorm:"uniqueIndex:idx_issue_objective_version,priority:2"`
	Content         string    `json:"content" gorm:"type:text"`
	SourceCommentID string    `json:"sourceCommentId,omitempty" gorm:"index"`
	CreatedBy       string    `json:"createdBy"`
	CreatedAt       time.Time `json:"createdAt"`
}

type IssueObjectiveView struct {
	IssueObjective
	Current          bool       `json:"current"`
	ValidationRounds int        `json:"validationRounds"`
	ValidationStatus string     `json:"validationStatus"`
	ValidationPassed bool       `json:"validationPassed"`
	LastValidationAt *time.Time `json:"lastValidationAt,omitempty"`
}

type ToolSnapshot struct {
	Name        string                  `json:"name"`
	Label       string                  `json:"label"`
	Description string                  `json:"description"`
	Source      string                  `json:"source"`
	Parameters  []ToolParameterSnapshot `json:"parameters"`
}

type ToolParameterSnapshot struct {
	Name        string                  `json:"name"`
	Type        string                  `json:"type"`
	Description string                  `json:"description"`
	Required    bool                    `json:"required"`
	Enum        []string                `json:"enum,omitempty"`
	Children    []ToolParameterSnapshot `json:"children,omitempty"`
}

type ExecutionEvent struct {
	ID          string    `json:"id" gorm:"primaryKey"`
	ExecutionID string    `json:"executionId" gorm:"index;index:idx_events_execution_created,priority:1;index:idx_events_execution_updated,priority:1"`
	IssueID     string    `json:"issueId,omitempty" gorm:"index;index:idx_events_issue_created,priority:1"`
	Type        string    `json:"type"`
	Title       string    `json:"title"`
	Detail      string    `json:"detail"`
	ToolCallID  string    `json:"toolCallId,omitempty" gorm:"index"`
	ToolName    string    `json:"toolName,omitempty" gorm:"index"`
	Status      string    `json:"status,omitempty"`
	InputJSON   string    `json:"inputJson,omitempty" gorm:"type:text"`
	OutputJSON  string    `json:"outputJson,omitempty" gorm:"type:text"`
	IsError     bool      `json:"isError,omitempty"`
	CreatedAt   time.Time `json:"createdAt" gorm:"index:idx_events_execution_created,priority:2;index:idx_events_issue_created,priority:2"`
	UpdatedAt   time.Time `json:"updatedAt" gorm:"index:idx_events_execution_updated,priority:2"`
}

// ExecutionProgress is an immutable, Agent-authored milestone within one Pi
// Session. It is stored separately from raw tool events so the Session UI can
// present a concise work log without interpreting provider-specific payloads.
type ExecutionProgress struct {
	ID              string    `json:"id" gorm:"primaryKey"`
	ExecutionID     string    `json:"executionId" gorm:"index;index:idx_progress_execution_created,priority:1"`
	IssueID         string    `json:"issueId" gorm:"index"`
	Stage           string    `json:"stage"`
	Summary         string    `json:"summary" gorm:"type:text"`
	CurrentActivity string    `json:"currentActivity" gorm:"type:text"`
	CreatedAt       time.Time `json:"createdAt" gorm:"index:idx_progress_execution_created,priority:2"`
}

type Message struct {
	ID          string            `json:"id" gorm:"primaryKey"`
	ExecutionID string            `json:"executionId" gorm:"index;index:idx_messages_execution_created,priority:1;index:idx_messages_execution_updated,priority:1"`
	IssueID     string            `json:"issueId,omitempty" gorm:"index"`
	Role        string            `json:"role"`
	Content     string            `json:"content" gorm:"type:text"`
	Streaming   bool              `json:"streaming"`
	CreatedAt   time.Time         `json:"createdAt" gorm:"index:idx_messages_execution_created,priority:2"`
	UpdatedAt   time.Time         `json:"updatedAt" gorm:"index:idx_messages_execution_updated,priority:2"`
	Attachments []InputAttachment `json:"attachments,omitempty" gorm:"-"`
}

// RelayThread and RelayMessage implement task-scoped asynchronous messaging
// between TaskAgents and applications such as Board.
type RelayThread struct {
	ID             string    `json:"id" gorm:"primaryKey"`
	TaskID         string    `json:"taskId,omitempty" gorm:"index"`
	Kind           string    `json:"kind" gorm:"index"`
	Title          string    `json:"title"`
	DirectKey      string    `json:"-" gorm:"uniqueIndex"`
	ParticipantIDs []string  `json:"participantIds" gorm:"serializer:json;type:text"`
	LastMessageAt  time.Time `json:"lastMessageAt" gorm:"index"`
	CreatedAt      time.Time `json:"createdAt"`
	UpdatedAt      time.Time `json:"updatedAt"`
}

type RelayMessage struct {
	ID           string    `json:"id" gorm:"primaryKey"`
	ThreadID     string    `json:"threadId" gorm:"index;index:idx_relay_messages_thread_created,priority:1"`
	SenderType   string    `json:"senderType"`
	SenderID     string    `json:"senderId" gorm:"index"`
	RecipientIDs []string  `json:"recipientIds" gorm:"serializer:json;type:text"`
	Body         string    `json:"body" gorm:"type:text"`
	IssueID      string    `json:"issueId,omitempty" gorm:"index"`
	DeliveryKey  *string   `json:"-" gorm:"uniqueIndex"`
	CreatedAt    time.Time `json:"createdAt" gorm:"index:idx_relay_messages_thread_created,priority:2"`
}

type RelayReceipt struct {
	ID        string     `json:"id" gorm:"primaryKey"`
	MessageID string     `json:"messageId" gorm:"uniqueIndex:idx_relay_receipt"`
	AgentID   string     `json:"agentId" gorm:"uniqueIndex:idx_relay_receipt;index"`
	ReadAt    *time.Time `json:"readAt,omitempty"`
	CreatedAt time.Time  `json:"createdAt"`
}

type RelayThreadSummary struct {
	Thread      RelayThread   `json:"thread"`
	LastMessage *RelayMessage `json:"lastMessage,omitempty"`
	UnreadCount int64         `json:"unreadCount"`
}

type RelayConversation struct {
	Thread      RelayThread    `json:"thread"`
	Messages    []RelayMessage `json:"messages"`
	UnreadCount int64          `json:"unreadCount"`
}

type SendRelayMessageInput struct {
	RecipientID          string `json:"recipientId"`
	RecipientTaskAgentID string `json:"recipientTaskAgentId,omitempty"`
	Body                 string `json:"body"`
	TaskID               string `json:"taskId,omitempty"`
	IssueID              string `json:"issueId"`
	IdempotencyKey       string `json:"-"`
}

type BoardCommandInput struct {
	Action          string   `json:"action"`
	IssueID         string   `json:"issueId"`
	Title           string   `json:"title"`
	Description     string   `json:"description"`
	Objective       string   `json:"objective"`
	Status          string   `json:"status"`
	Priority        string   `json:"priority"`
	AssigneeAgentID string   `json:"assigneeAgentId"`
	ParentID        string   `json:"parentId"`
	Body            string   `json:"body"`
	CommentType     string   `json:"commentType"`
	RelatedIssueID  string   `json:"relatedIssueId"`
	RelationID      string   `json:"relationId"`
	RelationType    string   `json:"relationType"`
	Reason          string   `json:"reason"`
	Query           string   `json:"query"`
	Statuses        []string `json:"statuses"`
}

type BoardCommandResult struct {
	Action            string         `json:"action"`
	Issue             *Issue         `json:"issue,omitempty"`
	Issues            []Issue        `json:"issues,omitempty"`
	Detail            *IssueDetail   `json:"detail,omitempty"`
	Comment           *IssueComment  `json:"comment,omitempty"`
	Relation          *IssueRelation `json:"relation,omitempty"`
	DeletedRelationID string         `json:"deletedRelationId,omitempty"`
	DeletedIssueCount int64          `json:"deletedIssueCount,omitempty"`
}

type DeleteIssueResult struct {
	IssueID           string `json:"issueId"`
	DeletedIssues     int64  `json:"deletedIssues"`
	DeletedExecutions int64  `json:"deletedExecutions"`
	DeletedTasks      int64  `json:"deletedTasks"`
}

type RelayCommandInput struct {
	Action      string `json:"action"`
	RecipientID string `json:"recipientId"`
	ThreadID    string `json:"threadId"`
	Body        string `json:"body"`
	IssueID     string `json:"issueId"`
}

type RelayCommandResult struct {
	Action       string                    `json:"action"`
	Directory    []TaskAgentDirectoryEntry `json:"directory,omitempty"`
	Inbox        []RelayThreadSummary      `json:"inbox,omitempty"`
	Conversation *RelayConversation        `json:"conversation,omitempty"`
	Message      *RelayMessage             `json:"message,omitempty"`
}

type TaskAgentDirectoryEntry struct {
	ID          string `json:"taskAgentId"`
	Name        string `json:"name"`
	AgentID     string `json:"agentId"`
	Role        string `json:"role"`
	Description string `json:"description"`
}
type Approval struct {
	ID          string     `json:"id" gorm:"primaryKey"`
	ExecutionID string     `json:"executionId" gorm:"index"`
	IssueID     string     `json:"issueId,omitempty" gorm:"index"`
	RequestID   string     `json:"-"`
	Type        string     `json:"type" gorm:"index"`
	Title       string     `json:"title"`
	Detail      string     `json:"detail"`
	Status      string     `json:"status" gorm:"index"`
	CreatedAt   time.Time  `json:"createdAt"`
	ResolvedAt  *time.Time `json:"resolvedAt,omitempty"`
}

type ApprovalDecisionInput struct {
	Approved      bool   `json:"approved"`
	ReviewContent string `json:"reviewContent"`
}
type IssueComment struct {
	ID          string            `json:"id" gorm:"primaryKey"`
	IssueID     string            `json:"issueId" gorm:"index;index:idx_comments_issue_created,priority:1"`
	Type        string            `json:"type" gorm:"index"`
	AuthorType  string            `json:"authorType"`
	AuthorID    string            `json:"authorId"`
	ExecutionID string            `json:"executionId,omitempty" gorm:"index"`
	Body        string            `json:"body" gorm:"type:text"`
	Mentions    []string          `json:"mentions" gorm:"serializer:json;type:text"`
	Attachments []IssueAttachment `json:"attachments" gorm:"-"`
	CreatedAt   time.Time         `json:"createdAt" gorm:"index:idx_comments_issue_created,priority:2"`
}

type IssueAttachment struct {
	ID          string    `json:"id" gorm:"primaryKey"`
	IssueID     string    `json:"issueId" gorm:"index"`
	CommentID   string    `json:"commentId,omitempty" gorm:"index"`
	ExecutionID string    `json:"executionId" gorm:"index;uniqueIndex:idx_execution_source"`
	Name        string    `json:"name"`
	Description string    `json:"description,omitempty"`
	SourcePath  string    `json:"sourcePath" gorm:"uniqueIndex:idx_execution_source"`
	StoragePath string    `json:"-"`
	MimeType    string    `json:"mimeType"`
	Size        int64     `json:"size"`
	CreatedAt   time.Time `json:"createdAt"`
}

// InputAttachment is an operator-supplied file staged before an Issue starts.
// StoragePath is server-private; Agents only receive the materialized path
// inside their task workspace/container.
type InputAttachment struct {
	ID          string     `json:"id" gorm:"primaryKey"`
	Scope       string     `json:"scope" gorm:"index"`
	OwnerID     string     `json:"ownerId,omitempty" gorm:"index"`
	TaskID      string     `json:"taskId,omitempty" gorm:"index"`
	IssueID     string     `json:"issueId,omitempty" gorm:"index"`
	ExecutionID string     `json:"executionId,omitempty" gorm:"index"`
	MessageID   string     `json:"messageId,omitempty" gorm:"index"`
	Name        string     `json:"name"`
	StoragePath string     `json:"-"`
	MimeType    string     `json:"mimeType"`
	Size        int64      `json:"size"`
	CreatedAt   time.Time  `json:"createdAt" gorm:"index"`
	BoundAt     *time.Time `json:"boundAt,omitempty"`
}

type PublishAttachmentInput struct {
	Path        string `json:"path"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

type SubmitFinalResultInput struct {
	Body        string `json:"body"`
	Path        string `json:"path"`
	Name        string `json:"name"`
	Description string `json:"attachmentDescription"`
}

type ValidationAttachmentInfo struct {
	ID             string                   `json:"id"`
	Name           string                   `json:"name"`
	Description    string                   `json:"description,omitempty"`
	MimeType       string                   `json:"mimeType"`
	Size           int64                    `json:"size"`
	Path           string                   `json:"path,omitempty"`
	DownloadURL    string                   `json:"downloadUrl"`
	Readable       bool                     `json:"readable"`
	ArchiveEntries []ValidationArchiveEntry `json:"archiveEntries,omitempty"`
}

type ValidationArchiveEntry struct {
	Path     string `json:"path"`
	Size     int64  `json:"size"`
	Readable bool   `json:"readable"`
}

type ValidationAttachmentChunk struct {
	Attachment  ValidationAttachmentInfo `json:"attachment"`
	Offset      int64                    `json:"offset"`
	NextOffset  int64                    `json:"nextOffset"`
	Content     string                   `json:"content"`
	EOF         bool                     `json:"eof"`
	ArchivePath string                   `json:"archivePath,omitempty"`
}

type AgentWakeup struct {
	ID                      string     `json:"id" gorm:"primaryKey"`
	IssueID                 string     `json:"issueId" gorm:"index"`
	CommentID               string     `json:"commentId"`
	AgentID                 string     `json:"agentId" gorm:"index"`
	ExecutionID             string     `json:"executionId,omitempty"`
	Reason                  string     `json:"reason"`
	Status                  string     `json:"status" gorm:"index"`
	Error                   string     `json:"error,omitempty"`
	PriorIssueStatus        string     `json:"priorIssueStatus,omitempty"`
	PriorExecutionPhase     string     `json:"priorExecutionPhase,omitempty"`
	PriorExecutionID        string     `json:"priorExecutionId,omitempty"`
	HeartbeatElapsedSeconds int64      `json:"heartbeatElapsedSeconds,omitempty"`
	CreatedAt               time.Time  `json:"createdAt"`
	DeliveredAt             *time.Time `json:"deliveredAt,omitempty"`
	CompletedAt             *time.Time `json:"completedAt,omitempty"`
}

type IssueDecomposition struct {
	ID                string    `json:"id" gorm:"primaryKey"`
	ParentIssueID     string    `json:"parentIssueId" gorm:"uniqueIndex:idx_decomposition_request"`
	SourceExecutionID string    `json:"sourceExecutionId" gorm:"index"`
	RequestKey        string    `json:"requestKey" gorm:"uniqueIndex:idx_decomposition_request"`
	Summary           string    `json:"summary"`
	ChildIDs          []string  `json:"childIds" gorm:"serializer:json;type:text"`
	CreatedAt         time.Time `json:"createdAt"`
}

type SubIssueSpec struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	Objective   string `json:"objective"`
	Priority    string `json:"priority"`
	AgentID     string `json:"agentId"`
	DependsOn   []int  `json:"dependsOn"`
}

type DecomposeIssueInput struct {
	RequestKey string         `json:"requestKey"`
	Summary    string         `json:"summary"`
	Children   []SubIssueSpec `json:"children"`
}

type DecompositionResult struct {
	Decomposition IssueDecomposition `json:"decomposition"`
	Children      []Issue            `json:"children"`
	Reused        bool               `json:"reused"`
}

type AgentModelConfig struct {
	Provider string        `json:"provider"`
	Model    string        `json:"model"`
	BaseURL  string        `json:"baseUrl"`
	Thinking string        `json:"thinking"`
	Pricing  *ModelPricing `json:"pricing"`
}

// ModelPricing stores USD prices per one million tokens.
type ModelPricing struct {
	Input      float64 `json:"input"`
	Output     float64 `json:"output"`
	CacheRead  float64 `json:"cacheRead"`
	CacheWrite float64 `json:"cacheWrite"`
}
type PermissionBoundary struct {
	WorkspaceScope     string `json:"workspaceScope"`
	AllowNetwork       bool   `json:"allowNetwork"`
	AllowShell         bool   `json:"allowShell"`
	AllowWrite         bool   `json:"allowWrite"`
	ApprovalMode       string `json:"approvalMode"`
	ReworkApprovalMode string `json:"reworkApprovalMode"`
}
type AgentDefinition struct {
	ID               string             `json:"id"`
	Name             string             `json:"name"`
	Description      string             `json:"description"`
	Avatar           string             `json:"avatar"`
	Category         string             `json:"category"`
	Enabled          bool               `json:"enabled"`
	Builtin          bool               `json:"builtin"`
	Internal         bool               `json:"internal"`
	Model            AgentModelConfig   `json:"model"`
	SystemPrompt     string             `json:"systemPrompt"`
	Memo             string             `json:"memo" gorm:"type:text"`
	Tools            []string           `json:"tools"`
	SkillIDs         []string           `json:"skillIds"`
	KnowledgeBaseIDs []string           `json:"knowledgeBaseIds"`
	Permissions      PermissionBoundary `json:"permissions"`
	CreatedAt        time.Time          `json:"createdAt"`
	UpdatedAt        time.Time          `json:"updatedAt"`
}
type SkillDefinition struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	DisplayName string    `json:"displayName"`
	Description string    `json:"description"`
	Content     string    `json:"content"`
	Source      string    `json:"source"`
	Version     string    `json:"version"`
	Builtin     bool      `json:"builtin"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
}
type SaveAgentInput struct {
	ID               string             `json:"id"`
	Name             string             `json:"name"`
	Description      string             `json:"description"`
	Avatar           string             `json:"avatar"`
	Category         string             `json:"category"`
	Enabled          bool               `json:"enabled"`
	Internal         bool               `json:"internal"`
	Model            AgentModelConfig   `json:"model"`
	SystemPrompt     string             `json:"systemPrompt"`
	Memo             string             `json:"memo"`
	Tools            []string           `json:"tools"`
	SkillIDs         []string           `json:"skillIds"`
	KnowledgeBaseIDs []string           `json:"knowledgeBaseIds"`
	Permissions      PermissionBoundary `json:"permissions"`
}

type UpdateAgentMemoInput struct {
	Content string `json:"content"`
}

type RequestIssueReworkInput struct {
	Reason           string `json:"reason"`
	RequestedOutcome string `json:"requestedOutcome"`
}

type IssueReworkRequestResult struct {
	Status      string `json:"status"`
	ApprovalID  string `json:"approvalId,omitempty"`
	ExecutionID string `json:"executionId,omitempty"`
}

type AgentMemoResult struct {
	AgentID string `json:"agentId"`
	Content string `json:"content"`
}

type ReportExecutionProgressInput struct {
	Stage           string `json:"stage"`
	Summary         string `json:"summary"`
	CurrentActivity string `json:"currentActivity"`
}

type GetIssueProgressInput struct {
	SessionID     string `json:"sessionId"`
	Mode          string `json:"mode"`
	ProgressLimit int    `json:"progressLimit"`
	MessageLimit  int    `json:"messageLimit"`
}

type IssueProgressResult struct {
	SessionID         string              `json:"sessionId"`
	Mode              string              `json:"mode"`
	Execution         Execution           `json:"execution"`
	Issue             Issue               `json:"issue"`
	AgentName         string              `json:"agentName"`
	ProgressUpdates   []ExecutionProgress `json:"progressUpdates"`
	Messages          []Message           `json:"messages"`
	ProgressTotal     int64               `json:"progressTotal"`
	MessageTotal      int64               `json:"messageTotal"`
	ProgressTruncated bool                `json:"progressTruncated"`
	MessagesTruncated bool                `json:"messagesTruncated"`
	OverflowFilePath  string              `json:"overflowFilePath,omitempty"`
	OverflowReason    string              `json:"overflowReason,omitempty"`
}

type SaveSkillInput struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	DisplayName string `json:"displayName"`
	Description string `json:"description"`
	Content     string `json:"content"`
	Source      string `json:"source"`
	Version     string `json:"version"`
}

type IssueDetail struct {
	Issue            Issue                `json:"issue"`
	Runtime          IssueRuntimeView     `json:"runtime"`
	Children         []Issue              `json:"children"`
	BlockedBy        []Issue              `json:"blockedBy"`
	Blocks           []Issue              `json:"blocks"`
	Executions       []Execution          `json:"executions"`
	Comments         []IssueComment       `json:"comments"`
	Messages         []Message            `json:"messages"`
	InputAttachments []InputAttachment    `json:"inputAttachments"`
	Events           []ExecutionEvent     `json:"events"`
	Approvals        []Approval           `json:"approvals"`
	Wakeups          []AgentWakeup        `json:"wakeups"`
	Decompositions   []IssueDecomposition `json:"decompositions"`
	Validations      []IssueValidation    `json:"validations"`
	Objectives       []IssueObjectiveView `json:"objectives"`
	CommentsPage     PageInfo             `json:"commentsPage"`
	EventsPage       PageInfo             `json:"eventsPage"`
	ExecutionsPage   PageInfo             `json:"executionsPage"`
	Watermark        time.Time            `json:"watermark"`
}

type SessionSummary struct {
	Execution       Execution `json:"execution"`
	IssueIdentifier string    `json:"issueIdentifier"`
	IssueTitle      string    `json:"issueTitle"`
	AgentName       string    `json:"agentName"`
}
type SessionDetail struct {
	Session         SessionSummary      `json:"session"`
	Messages        []Message           `json:"messages"`
	Events          []ExecutionEvent    `json:"events"`
	ProgressUpdates []ExecutionProgress `json:"progressUpdates"`
	Approvals       []Approval          `json:"approvals"`
	MessagesPage    PageInfo            `json:"messagesPage"`
	EventsPage      PageInfo            `json:"eventsPage"`
	ProgressPage    PageInfo            `json:"progressPage"`
	Watermark       time.Time           `json:"watermark"`
}

type PageInfo struct {
	NextCursor string `json:"nextCursor,omitempty"`
	HasMore    bool   `json:"hasMore"`
	Total      int64  `json:"total"`
}

type IssueCommentPage struct {
	Items []IssueComment `json:"items"`
	Page  PageInfo       `json:"page"`
}

type ExecutionEventPage struct {
	Items []ExecutionEvent `json:"items"`
	Page  PageInfo         `json:"page"`
}

type ExecutionPage struct {
	Items []Execution `json:"items"`
	Page  PageInfo    `json:"page"`
}

type MessagePage struct {
	Items []Message `json:"items"`
	Page  PageInfo  `json:"page"`
}

type ExecutionProgressPage struct {
	Items []ExecutionProgress `json:"items"`
	Page  PageInfo            `json:"page"`
}

type SessionDelta struct {
	Execution       Execution           `json:"execution"`
	Messages        []Message           `json:"messages"`
	Events          []ExecutionEvent    `json:"events"`
	ProgressUpdates []ExecutionProgress `json:"progressUpdates"`
	Watermark       time.Time           `json:"watermark"`
}
type StateView struct {
	Configured        bool                `json:"configured"`
	Config            ConfigView          `json:"config"`
	Projects          []Project           `json:"projects"`
	ContainerProfiles []ContainerProfile  `json:"containerProfiles"`
	Containers        []ContainerInstance `json:"containers"`
	Tasks             []Task              `json:"tasks"`
	Issues            []Issue             `json:"issues"`
	IssueRuntimes     []IssueRuntimeView  `json:"issueRuntimes"`
	Relations         []IssueRelation     `json:"relations"`
	Executions        []Execution         `json:"executions"`
	Approvals         []Approval          `json:"approvals"`
	Agents            []AgentDefinition   `json:"agents"`
	TaskAgents        []TaskAgent         `json:"taskAgents"`
	Skills            []SkillDefinition   `json:"skills"`
	KnowledgeBases    []KnowledgeBase     `json:"knowledgeBases"`
	Sessions          []SessionSummary    `json:"sessions"`
	UpdatedAt         time.Time           `json:"updatedAt"`
}

const KnowledgeProviderKeywordAI = "keyword_ai"

type KnowledgeBase struct {
	ID                string    `json:"id" gorm:"primaryKey"`
	Name              string    `json:"name" gorm:"uniqueIndex"`
	Description       string    `json:"description" gorm:"type:text"`
	RetrievalProvider string    `json:"retrievalProvider" gorm:"index"`
	DocumentCount     int64     `json:"documentCount" gorm:"-"`
	CreatedAt         time.Time `json:"createdAt"`
	UpdatedAt         time.Time `json:"updatedAt"`
}

type KnowledgeDocument struct {
	ID              string    `json:"id" gorm:"primaryKey"`
	KnowledgeBaseID string    `json:"knowledgeBaseId" gorm:"uniqueIndex:idx_knowledge_document_name;index"`
	Name            string    `json:"name" gorm:"uniqueIndex:idx_knowledge_document_name"`
	Content         string    `json:"content" gorm:"type:text"`
	CreatedAt       time.Time `json:"createdAt"`
	UpdatedAt       time.Time `json:"updatedAt"`
}

type KnowledgeBaseDetail struct {
	KnowledgeBase KnowledgeBase       `json:"knowledgeBase"`
	Documents     []KnowledgeDocument `json:"documents"`
}

type SaveKnowledgeBaseInput struct {
	Name              string `json:"name"`
	Description       string `json:"description"`
	RetrievalProvider string `json:"retrievalProvider"`
}

type SaveKnowledgeDocumentInput struct {
	Name    string `json:"name"`
	Content string `json:"content"`
}

type KnowledgeSearchInput struct {
	Query           string `json:"query"`
	KnowledgeBaseID string `json:"knowledgeBaseId"`
	Limit           int    `json:"limit"`
}

type KnowledgeSearchHit struct {
	KnowledgeBaseID   string  `json:"knowledgeBaseId"`
	KnowledgeBaseName string  `json:"knowledgeBaseName"`
	DocumentID        string  `json:"documentId"`
	DocumentName      string  `json:"documentName"`
	Excerpt           string  `json:"excerpt"`
	Score             float64 `json:"score"`
	Reason            string  `json:"reason"`
}

type KnowledgeSearchResult struct {
	Provider string               `json:"provider"`
	Query    string               `json:"query"`
	Summary  string               `json:"summary"`
	Hits     []KnowledgeSearchHit `json:"hits"`
}

type CreateIssueInput struct {
	ProjectID               string           `json:"projectId"`
	ParentID                string           `json:"parentId"`
	Title                   string           `json:"title"`
	Description             string           `json:"description"`
	Objective               string           `json:"objective"`
	Priority                string           `json:"priority"`
	Status                  string           `json:"status"`
	WorkMode                string           `json:"workMode"`
	AssigneeAgentID         string           `json:"assigneeAgentId"`
	AssigneeTaskAgentID     string           `json:"assigneeTaskAgentId,omitempty"`
	Capabilities            []capability.Ref `json:"capabilities,omitempty"`
	CapabilitySelection     string           `json:"capabilitySelection,omitempty"`
	Workspace               string           `json:"workspace"`
	ContainerProfileID      string           `json:"containerProfileId"`
	ContainerID             string           `json:"containerId"`
	TimeBudgetMinutes       *int             `json:"timeBudgetMinutes"`
	HumanValidationFallback bool             `json:"humanValidationFallback"`
	BlockedBy               []string         `json:"blockedBy"`
	TaskSourceID            string           `json:"taskSourceId"`
	AttachmentIDs           []string         `json:"attachmentIds" gorm:"-"`
	// AttachmentSourceExecutionID is internal-only. It lets an Agent hand files
	// from a task-creation turn to the first Board Issue it
	// create without exposing another binding primitive through the API.
	AttachmentSourceExecutionID string `json:"-" gorm:"-"`
	CreatedBy                   string `json:"-" gorm:"-"`
	RequestedID                 string `json:"-" gorm:"-"`
}

type ChildIssueInfo struct {
	Issue
	Comments []IssueComment `json:"comments,omitempty"`
}

type IssueChildWait struct {
	ID                   string     `json:"id" gorm:"primaryKey"`
	ParentIssueID        string     `json:"parentIssueId" gorm:"index"`
	SourceExecutionID    string     `json:"sourceExecutionId" gorm:"index"`
	WaitForAll           bool       `json:"waitForAll"`
	ChildIssueIDs        []string   `json:"childIssueIds" gorm:"serializer:json;type:text"`
	WakeupIDs            []string   `json:"wakeupIds" gorm:"serializer:json;type:text"`
	EstimatedWaitMinutes int        `json:"estimatedWaitMinutes"`
	NextCheckAt          *time.Time `json:"nextCheckAt,omitempty" gorm:"index"`
	Status               string     `json:"status" gorm:"index"`
	CreatedAt            time.Time  `json:"createdAt"`
	CompletedAt          *time.Time `json:"completedAt,omitempty"`
}

type WaitForChildIssuesInput struct {
	WaitForAll           bool     `json:"waitForAll"`
	ChildIssueIDs        []string `json:"childIssueIds"`
	WakeupIDs            []string `json:"wakeupIds"`
	EstimatedWaitMinutes int      `json:"estimatedWaitMinutes"`
}

type CancelIssueInput struct {
	IssueID string `json:"issueId"`
	Reason  string `json:"reason"`
	Mode    string `json:"mode"`
}

type ResumeIssueTreeInput struct {
	Reason string `json:"reason"`
}

type WorkspaceEntry struct {
	Path       string    `json:"path"`
	Kind       string    `json:"kind"`
	Size       int64     `json:"size,omitempty"`
	ModifiedAt time.Time `json:"modifiedAt"`
}

type TaskWorkspace struct {
	Root    string           `json:"root"`
	Entries []WorkspaceEntry `json:"entries"`
}
type UpdateIssueInput struct {
	Title             *string   `json:"title"`
	Description       *string   `json:"description"`
	Objective         *string   `json:"objective"`
	Priority          *string   `json:"priority"`
	Status            *string   `json:"status"`
	Labels            *[]string `json:"labels"`
	AssigneeAgentID   *string   `json:"assigneeAgentId"`
	ParentID          *string   `json:"parentId"`
	TimeBudgetMinutes *int      `json:"timeBudgetMinutes"`
}

type CancelTaskInput struct {
	Reason string `json:"reason"`
}

type AbandonIssueInput struct {
	Reason string `json:"reason"`
}

type IssueValidationControlInput struct {
	Disabled bool `json:"disabled"`
}

type ManualValidationOverrideInput struct {
	Reason string `json:"reason"`
}

type SubmitValidationDecisionInput struct {
	Outcome            string `json:"outcome"`
	Summary            string `json:"summary"`
	Feedback           string `json:"feedback"`
	ImpossibilityProof string `json:"impossibilityProof"`
}

type CloseValidatedIssueInput struct {
	Summary           string `json:"summary"`
	EvidenceCommentID string `json:"evidenceCommentId"`
}

type TaskCancellationResult struct {
	Task                Task  `json:"task"`
	TotalIssues         int   `json:"totalIssues"`
	CancelledIssues     int64 `json:"cancelledIssues"`
	CancelledExecutions int64 `json:"cancelledExecutions"`
	RemovedApprovals    int64 `json:"removedApprovals"`
	CancelledWakeups    int64 `json:"cancelledWakeups"`
}

type ToolInterruptResult struct {
	ExecutionID string `json:"executionId"`
	Tool        string `json:"tool"`
	Status      string `json:"status"`
}

// TaskTimeline is a read model for observing the important lifecycle events of
// an entire top-level task and all Issues below it.
type TaskTimeline struct {
	Task       Task                `json:"task"`
	IssueCount int                 `json:"issueCount"`
	Events     []TaskTimelineEvent `json:"events"`
}

type TaskTimelineEvent struct {
	ID              string    `json:"id"`
	Kind            string    `json:"kind"`
	IssueID         string    `json:"issueId"`
	IssueIdentifier string    `json:"issueIdentifier"`
	IssueTitle      string    `json:"issueTitle"`
	ExecutionID     string    `json:"executionId,omitempty"`
	ActorType       string    `json:"actorType"`
	ActorID         string    `json:"actorId,omitempty"`
	ActorName       string    `json:"actorName"`
	Title           string    `json:"title"`
	Summary         string    `json:"summary,omitempty"`
	Detail          string    `json:"detail,omitempty"`
	Status          string    `json:"status,omitempty"`
	CreatedAt       time.Time `json:"createdAt"`
}

type CheckoutIssueInput struct {
	AgentID          string   `json:"agentId"`
	ExecutionID      string   `json:"executionId"`
	ExpectedStatuses []string `json:"expectedStatuses"`
}
type CreateRelationInput struct {
	RelatedIssueID string `json:"relatedIssueId"`
	Type           string `json:"type"`
}

type SaveConfigInput struct {
	Language              string               `json:"language"`
	Provider              string               `json:"provider"`
	Model                 string               `json:"model"`
	Pricing               ModelPricing         `json:"pricing"`
	BaseURL               string               `json:"baseUrl"`
	Thinking              string               `json:"thinking"`
	AuthMode              string               `json:"authMode"`
	APIKey                string               `json:"apiKey"`
	Workspace             string               `json:"workspace"`
	Concurrency           int                  `json:"concurrency"`
	ApprovalMode          string               `json:"approvalMode"`
	ReworkApprovalMode    string               `json:"reworkApprovalMode"`
	ValidationMode        string               `json:"validationMode"`
	MaxValidationAttempts int                  `json:"maxValidationAttempts"`
	MaxIssueDepth         int                  `json:"maxIssueDepth"`
	MaxChildrenPerRequest int                  `json:"maxChildrenPerRequest"`
	MaxDirectChildren     int                  `json:"maxDirectChildren"`
	IssueBudget           IssueBudgetConfig    `json:"issueBudget"`
	IssueHeartbeat        IssueHeartbeatConfig `json:"issueHeartbeat"`
	WebSearch             WebSearchConfig      `json:"webSearch"`
}

type WebSearchInput struct {
	Query         string `json:"query"`
	Topic         string `json:"topic"`
	SearchDepth   string `json:"searchDepth"`
	IncludeAnswer bool   `json:"includeAnswer"`
	MaxResults    int    `json:"maxResults"`
}

type WebSearchTestInput struct {
	Config WebSearchConfig `json:"config"`
	Search WebSearchInput  `json:"search"`
}

type WebSearchItem struct {
	Title   string  `json:"title"`
	URL     string  `json:"url"`
	Content string  `json:"content"`
	Score   float64 `json:"score,omitempty"`
}

type WebSearchResult struct {
	Engine       string          `json:"engine"`
	Query        string          `json:"query"`
	Answer       string          `json:"answer,omitempty"`
	Results      []WebSearchItem `json:"results"`
	ResponseTime float64         `json:"responseTime,omitempty"`
}

// Finding represents a security finding discovered during reconnaissance or vulnerability analysis.
type Finding struct {
	AuditLevel       string    `json:"auditLevel,omitempty"`
	ID               string    `json:"id" gorm:"primaryKey"`
	Domain           string    `json:"domain" gorm:"index"`
	Category         string    `json:"category" gorm:"index"`
	Severity         string    `json:"severity" gorm:"index"`
	Title            string    `json:"title"`
	Description      string    `json:"description" gorm:"type:text"`
	Evidence         any       `json:"evidence" gorm:"serializer:json;type:text"`
	StepsToReproduce string    `json:"stepsToReproduce" gorm:"type:text"`
	Remediation      string    `json:"remediation" gorm:"type:text"`
	Status           string    `json:"status" gorm:"index"`
	CreatedAt        time.Time `json:"createdAt"`
	UpdatedAt        time.Time `json:"updatedAt"`
}

type CreateFindingInput struct {
	AuditLevel       string `json:"auditLevel"`
	Domain           string `json:"domain"`
	Category         string `json:"category"`
	Severity         string `json:"severity"`
	Title            string `json:"title"`
	Description      string `json:"description"`
	Evidence         any    `json:"evidence"`
	StepsToReproduce string `json:"stepsToReproduce"`
	Remediation      string `json:"remediation"`
}

type UpdateFindingInput struct {
	AuditLevel       *string `json:"auditLevel"`
	Domain           *string `json:"domain"`
	Category         *string `json:"category"`
	Severity         *string `json:"severity"`
	Title            *string `json:"title"`
	Description      *string `json:"description"`
	Evidence         any     `json:"evidence"`
	StepsToReproduce *string `json:"stepsToReproduce"`
	Remediation      *string `json:"remediation"`
	Status           *string `json:"status"`
}

type FindingList struct {
	Findings []Finding `json:"findings"`
	Total    int64     `json:"total"`
	Page     int       `json:"page"`
	PageSize int       `json:"pageSize"`
}

type ConnectionTestResult struct {
	OK       bool   `json:"ok"`
	Reply    string `json:"reply,omitempty"`
	Duration int64  `json:"durationMs"`
	Error    string `json:"error,omitempty"`
}
