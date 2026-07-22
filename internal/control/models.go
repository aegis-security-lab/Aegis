package control

import "time"

type IssueBudgetConfig struct {
	TokenLimit           *int64   `json:"tokenLimit"`
	CostLimit            *float64 `json:"costLimit"`
	TimeLimitMinutes     *int     `json:"timeLimitMinutes"`
	CheckIntervalSeconds int      `json:"checkIntervalSeconds"`
}

type IssueHeartbeatConfig struct {
	IntervalSeconds int `json:"intervalSeconds"`
}

type Config struct {
	Configured            bool                 `json:"configured"`
	NodePath              string               `json:"nodePath"`
	PiPath                string               `json:"piPath"`
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
	UpdatedAt             time.Time            `json:"updatedAt"`
}

type ConfigView struct {
	Configured            bool                 `json:"configured"`
	NodePath              string               `json:"nodePath"`
	PiPath                string               `json:"piPath"`
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
	UpdatedAt             time.Time            `json:"updatedAt"`
}

type RuntimeProbe struct {
	Ready       bool   `json:"ready"`
	NodePath    string `json:"nodePath"`
	NodeVersion string `json:"nodeVersion,omitempty"`
	PiPath      string `json:"piPath"`
	PiVersion   string `json:"piVersion,omitempty"`
	AuthFound   bool   `json:"authFound"`
	Error       string `json:"error,omitempty"`
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
	NodePath      string    `json:"nodePath"`
	PiPath        string    `json:"piPath"`
	WorkspacePath string    `json:"workspacePath"`
	HostWorkspace string    `json:"hostWorkspace"`
	NetworkMode   string    `json:"networkMode"`
	MemoryMB      int       `json:"memoryMb"`
	CPUs          float64   `json:"cpus"`
	Enabled       bool      `json:"enabled" gorm:"index"`
	RuntimeStatus string    `json:"runtimeStatus" gorm:"-"`
	ContainerName string    `json:"containerName" gorm:"-"`
	CreatedAt     time.Time `json:"createdAt"`
	UpdatedAt     time.Time `json:"updatedAt"`
}

type SaveContainerProfileInput struct {
	Name          string  `json:"name"`
	Description   string  `json:"description"`
	Image         string  `json:"image"`
	NodePath      string  `json:"nodePath"`
	PiPath        string  `json:"piPath"`
	WorkspacePath string  `json:"workspacePath"`
	HostWorkspace string  `json:"hostWorkspace"`
	NetworkMode   string  `json:"networkMode"`
	MemoryMB      int     `json:"memoryMb"`
	CPUs          float64 `json:"cpus"`
	Enabled       bool    `json:"enabled"`
}

type ContainerProfileDeleteImpact struct {
	ContainerProfileID   string `json:"containerProfileId"`
	IssueCount           int    `json:"issueCount"`
	TaskCount            int    `json:"taskCount"`
	ExecutionCount       int    `json:"executionCount"`
	ActiveExecutionCount int    `json:"activeExecutionCount"`
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
	ID                 string    `json:"id" gorm:"primaryKey"`
	ProjectID          string    `json:"projectId" gorm:"index"`
	Title              string    `json:"title"`
	Description        string    `json:"description"`
	Objective          string    `json:"objective" gorm:"type:text"`
	Priority           string    `json:"priority"`
	WorkMode           string    `json:"workMode"`
	AssigneeAgentID    string    `json:"assigneeAgentId,omitempty" gorm:"index"`
	Workspace          string    `json:"workspace"`
	ContainerProfileID string    `json:"containerProfileId,omitempty" gorm:"index"`
	Context            string    `json:"context,omitempty" gorm:"type:text"`
	Constraints        string    `json:"constraints,omitempty" gorm:"type:text"`
	TimeBudgetMinutes  *int      `json:"timeBudgetMinutes,omitempty"`
	CreatedAt          time.Time `json:"createdAt"`
	UpdatedAt          time.Time `json:"updatedAt"`
}

// Issue is the single source of truth for one run or a child execution unit.
type Issue struct {
	ID                    string     `json:"id" gorm:"primaryKey"`
	Number                int64      `json:"number" gorm:"uniqueIndex"`
	Identifier            string     `json:"identifier" gorm:"uniqueIndex"`
	ProjectID             string     `json:"projectId" gorm:"index"`
	ParentID              string     `json:"parentId,omitempty" gorm:"index"`
	TaskSourceID          string     `json:"taskSourceId,omitempty" gorm:"index"`
	Title                 string     `json:"title"`
	Description           string     `json:"description"`
	Objective             string     `json:"objective" gorm:"type:text"`
	Status                string     `json:"status" gorm:"index"`
	Priority              string     `json:"priority" gorm:"index"`
	WorkMode              string     `json:"workMode"`
	ExecutionPhase        string     `json:"executionPhase" gorm:"index"`
	RequestDepth          int        `json:"requestDepth"`
	AssigneeAgentID       string     `json:"assigneeAgentId,omitempty" gorm:"index"`
	CheckoutExecutionID   string     `json:"checkoutExecutionId,omitempty" gorm:"index"`
	CurrentExecutionID    string     `json:"currentExecutionId,omitempty" gorm:"index"`
	ValidationExecutionID string     `json:"validationExecutionId,omitempty" gorm:"index"`
	ValidationMode        string     `json:"validationMode" gorm:"default:fixed"`
	MaxValidationAttempts int        `json:"maxValidationAttempts" gorm:"default:3"`
	ValidationDisabled    bool       `json:"validationDisabled" gorm:"index"`
	RecoveryExecutionID   string     `json:"recoveryExecutionId,omitempty" gorm:"index"`
	RecoveryPhase         string     `json:"recoveryPhase,omitempty"`
	RecoveryRequestedAt   *time.Time `json:"recoveryRequestedAt,omitempty"`
	Workspace             string     `json:"workspace"`
	ContainerProfileID    string     `json:"containerProfileId,omitempty" gorm:"index"`
	Context               string     `json:"context,omitempty"`
	Constraints           string     `json:"constraints,omitempty"`
	TimeBudgetMinutes     *int       `json:"timeBudgetMinutes,omitempty"`
	Result                string     `json:"result,omitempty"`
	Error                 string     `json:"error,omitempty"`
	ObjectiveAbandoned    bool       `json:"objectiveAbandoned" gorm:"index"`
	Hidden                bool       `json:"-" gorm:"index"`
	AbandonmentReason     string     `json:"abandonmentReason,omitempty" gorm:"type:text"`
	AbandonRequestedAt    *time.Time `json:"abandonRequestedAt,omitempty"`
	AbandonedAt           *time.Time `json:"abandonedAt,omitempty"`
	CreatedBy             string     `json:"createdBy"`
	StartedAt             *time.Time `json:"startedAt,omitempty"`
	CompletedAt           *time.Time `json:"completedAt,omitempty"`
	CancelledAt           *time.Time `json:"cancelledAt,omitempty"`
	CreatedAt             time.Time  `json:"createdAt"`
	UpdatedAt             time.Time  `json:"updatedAt"`
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
	Constraints     string `json:"constraints"`
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
	ID                  string         `json:"id" gorm:"primaryKey"`
	IssueID             string         `json:"issueId" gorm:"index;index:idx_executions_issue_started,priority:1"`
	AgentID             string         `json:"agentId" gorm:"index"`
	Kind                string         `json:"kind"`
	Status              string         `json:"status" gorm:"index"`
	Provider            string         `json:"provider"`
	Model               string         `json:"model"`
	Pricing             ModelPricing   `json:"pricing" gorm:"serializer:json;type:text"`
	Thinking            string         `json:"thinking"`
	SessionID           string         `json:"sessionId" gorm:"index"`
	IssueAgentSessionID string         `json:"issueAgentSessionId,omitempty" gorm:"index"`
	PID                 int            `json:"pid,omitempty" gorm:"column:pid"`
	RuntimeType         string         `json:"runtimeType" gorm:"index"`
	RuntimeID           string         `json:"runtimeId,omitempty"`
	ContainerProfileID  string         `json:"containerProfileId,omitempty" gorm:"index"`
	ContainerImage      string         `json:"containerImage,omitempty"`
	CurrentTool         string         `json:"currentTool,omitempty"`
	Checkpoint          string         `json:"checkpoint,omitempty" gorm:"type:text"`
	CheckpointAt        *time.Time     `json:"checkpointAt,omitempty"`
	InitialPrompt       string         `json:"initialPrompt,omitempty" gorm:"type:text"`
	SystemPrompt        string         `json:"systemPrompt,omitempty" gorm:"type:text"`
	ToolsSnapshot       []ToolSnapshot `json:"toolsSnapshot" gorm:"serializer:json;type:text"`
	Result              string         `json:"result,omitempty"`
	Error               string         `json:"error,omitempty"`
	Cost                float64        `json:"cost"`
	Tokens              int64          `json:"tokens"`
	InputTokens         int64          `json:"inputTokens"`
	OutputTokens        int64          `json:"outputTokens"`
	CacheReadTokens     int64          `json:"cacheReadTokens"`
	CacheWriteTokens    int64          `json:"cacheWriteTokens"`
	MessageCount        int            `json:"messageCount"`
	StartedAt           time.Time      `json:"startedAt" gorm:"index:idx_executions_issue_started,priority:2"`
	UpdatedAt           time.Time      `json:"updatedAt"`
	FinishedAt          *time.Time     `json:"finishedAt,omitempty"`
}

// IssueAgentSession is the durable current Pi conversation for one Agent on
// one Issue. Executions are audit attempts; multiple executions may continue
// the same conversation through this binding.
type IssueAgentSession struct {
	ID                 string    `json:"id" gorm:"primaryKey"`
	IssueID            string    `json:"issueId" gorm:"index;index:idx_issue_agent_session_pair,priority:1"`
	AgentID            string    `json:"agentId" gorm:"index;index:idx_issue_agent_session_pair,priority:2"`
	SessionID          string    `json:"sessionId" gorm:"uniqueIndex"`
	Status             string    `json:"status" gorm:"index"`
	Generation         int       `json:"generation"`
	RuntimeType        string    `json:"runtimeType"`
	ContainerProfileID string    `json:"containerProfileId,omitempty"`
	Workspace          string    `json:"workspace"`
	CreatedAt          time.Time `json:"createdAt"`
	UpdatedAt          time.Time `json:"updatedAt"`
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

// TaskBroadcast is a durable peer-to-peer coordination message scoped to one
// top-level Task tree. Runtime delivery is best-effort; the history remains
// available to Agents that start after the broadcast was created.
type TaskBroadcast struct {
	ID                    string    `json:"id" gorm:"primaryKey"`
	TaskID                string    `json:"taskId" gorm:"index"`
	SourceIssueID         string    `json:"sourceIssueId" gorm:"index"`
	SourceIssueIdentifier string    `json:"sourceIssueIdentifier"`
	SourceIssueTitle      string    `json:"sourceIssueTitle"`
	SourceExecutionID     string    `json:"sourceExecutionId" gorm:"index"`
	SourceAgentID         string    `json:"sourceAgentId" gorm:"index"`
	SourceAgentName       string    `json:"sourceAgentName"`
	Subject               string    `json:"subject"`
	Message               string    `json:"message" gorm:"type:text"`
	Importance            string    `json:"importance" gorm:"index"`
	DeliveredCount        int       `json:"deliveredCount"`
	RecipientExecutionIDs []string  `json:"recipientExecutionIds" gorm:"serializer:json;type:text"`
	CreatedAt             time.Time `json:"createdAt"`
}

type Message struct {
	ID          string    `json:"id" gorm:"primaryKey"`
	ExecutionID string    `json:"executionId" gorm:"index;index:idx_messages_execution_created,priority:1;index:idx_messages_execution_updated,priority:1"`
	IssueID     string    `json:"issueId,omitempty" gorm:"index"`
	Role        string    `json:"role"`
	Content     string    `json:"content" gorm:"type:text"`
	Streaming   bool      `json:"streaming"`
	CreatedAt   time.Time `json:"createdAt" gorm:"index:idx_messages_execution_created,priority:2"`
	UpdatedAt   time.Time `json:"updatedAt" gorm:"index:idx_messages_execution_updated,priority:2"`
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

type PublishAttachmentInput struct {
	Path        string `json:"path"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

type ValidationAttachmentInfo struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	MimeType    string `json:"mimeType"`
	Size        int64  `json:"size"`
	DownloadURL string `json:"downloadUrl"`
	Readable    bool   `json:"readable"`
}

type ValidationAttachmentChunk struct {
	Attachment ValidationAttachmentInfo `json:"attachment"`
	Offset     int64                    `json:"offset"`
	NextOffset int64                    `json:"nextOffset"`
	Content    string                   `json:"content"`
	EOF        bool                     `json:"eof"`
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
	TemplateID       string             `json:"templateId"`
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
type AgentTemplateMetadata struct {
	EnglishName  string   `json:"englishName"`
	ChineseName  string   `json:"chineseName"`
	Introduction string   `json:"introduction"`
	Positions    []string `json:"positions" gorm:"serializer:json;type:text"`
}
type AgentTemplate struct {
	ID           string                `json:"id" gorm:"primaryKey"`
	Provider     string                `json:"provider" gorm:"index"`
	Model        string                `json:"model" gorm:"index"`
	SystemPrompt string                `json:"systemPrompt" gorm:"type:text"`
	Note         string                `json:"note" gorm:"type:text"`
	Metadata     AgentTemplateMetadata `json:"metadata" gorm:"embedded;embeddedPrefix:metadata_"`
	Hidden       bool                  `json:"hidden" gorm:"index"`
	Builtin      bool                  `json:"builtin"`
	CreatedAt    time.Time             `json:"createdAt"`
	UpdatedAt    time.Time             `json:"updatedAt"`
}
type SaveAgentTemplateInput struct {
	Provider     string                `json:"provider"`
	Model        string                `json:"model"`
	SystemPrompt string                `json:"systemPrompt"`
	Note         string                `json:"note"`
	Metadata     AgentTemplateMetadata `json:"metadata"`
}
type SetAgentTemplateHiddenInput struct {
	Hidden bool `json:"hidden"`
}
type UpdateAgentTemplateNoteInput struct {
	Note string `json:"note"`
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
	TemplateID       string             `json:"templateId"`
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

type BroadcastMessageInput struct {
	Subject    string `json:"subject"`
	Message    string `json:"message"`
	Importance string `json:"importance"`
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
	Issue          Issue                `json:"issue"`
	Children       []Issue              `json:"children"`
	BlockedBy      []Issue              `json:"blockedBy"`
	Blocks         []Issue              `json:"blocks"`
	Executions     []Execution          `json:"executions"`
	AgentSessions  []IssueAgentSession  `json:"agentSessions"`
	Comments       []IssueComment       `json:"comments"`
	Messages       []Message            `json:"messages"`
	Events         []ExecutionEvent     `json:"events"`
	Approvals      []Approval           `json:"approvals"`
	Wakeups        []AgentWakeup        `json:"wakeups"`
	Decompositions []IssueDecomposition `json:"decompositions"`
	Validations    []IssueValidation    `json:"validations"`
	Broadcasts     []TaskBroadcast      `json:"broadcasts"`
	CommentsPage   PageInfo             `json:"commentsPage"`
	EventsPage     PageInfo             `json:"eventsPage"`
	ExecutionsPage PageInfo             `json:"executionsPage"`
	Watermark      time.Time            `json:"watermark"`
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
	Configured        bool               `json:"configured"`
	Config            ConfigView         `json:"config"`
	Runtime           RuntimeProbe       `json:"runtime"`
	Projects          []Project          `json:"projects"`
	ContainerProfiles []ContainerProfile `json:"containerProfiles"`
	Tasks             []Task             `json:"tasks"`
	Issues            []Issue            `json:"issues"`
	Relations         []IssueRelation    `json:"relations"`
	Executions        []Execution        `json:"executions"`
	Approvals         []Approval         `json:"approvals"`
	Agents            []AgentDefinition  `json:"agents"`
	Skills            []SkillDefinition  `json:"skills"`
	KnowledgeBases    []KnowledgeBase    `json:"knowledgeBases"`
	Sessions          []SessionSummary   `json:"sessions"`
	UpdatedAt         time.Time          `json:"updatedAt"`
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
	ProjectID          string   `json:"projectId"`
	ParentID           string   `json:"parentId"`
	Title              string   `json:"title"`
	Description        string   `json:"description"`
	Objective          string   `json:"objective"`
	Priority           string   `json:"priority"`
	Status             string   `json:"status"`
	WorkMode           string   `json:"workMode"`
	AssigneeAgentID    string   `json:"assigneeAgentId"`
	Workspace          string   `json:"workspace"`
	ContainerProfileID string   `json:"containerProfileId"`
	Context            string   `json:"context"`
	Constraints        string   `json:"constraints"`
	TimeBudgetMinutes  *int     `json:"timeBudgetMinutes"`
	BlockedBy          []string `json:"blockedBy"`
	TaskSourceID       string   `json:"taskSourceId"`
}

type CommentIssueInput struct {
	IssueID string `json:"issueId"`
	Body    string `json:"body"`
}

type CommentIssueResult struct {
	IssueComment
	WakeupIDs []string `json:"wakeupIds"`
}

type ChildIssueInfo struct {
	Issue
	Comments []IssueComment `json:"comments,omitempty"`
}

type IssueChildWait struct {
	ID                string     `json:"id" gorm:"primaryKey"`
	ParentIssueID     string     `json:"parentIssueId" gorm:"index"`
	SourceExecutionID string     `json:"sourceExecutionId" gorm:"index"`
	WaitForAll        bool       `json:"waitForAll"`
	ChildIssueIDs     []string   `json:"childIssueIds" gorm:"serializer:json;type:text"`
	WakeupIDs         []string   `json:"wakeupIds" gorm:"serializer:json;type:text"`
	Status            string     `json:"status" gorm:"index"`
	CreatedAt         time.Time  `json:"createdAt"`
	CompletedAt       *time.Time `json:"completedAt,omitempty"`
}

type WaitForChildIssuesInput struct {
	WaitForAll    bool     `json:"waitForAll"`
	ChildIssueIDs []string `json:"childIssueIds"`
	WakeupIDs     []string `json:"wakeupIds"`
}

type CancelIssueInput struct {
	IssueID string `json:"issueId"`
	Reason  string `json:"reason"`
	Mode    string `json:"mode"`
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
	Title           *string `json:"title"`
	Description     *string `json:"description"`
	Objective       *string `json:"objective"`
	Priority        *string `json:"priority"`
	Status          *string `json:"status"`
	AssigneeAgentID *string `json:"assigneeAgentId"`
	ParentID        *string `json:"parentId"`
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
	Task                Issue `json:"task"`
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
	Task       Issue               `json:"task"`
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
	NodePath              string               `json:"nodePath"`
	PiPath                string               `json:"piPath"`
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
}

// Finding represents a security finding discovered during reconnaissance or vulnerability analysis.
type Finding struct {
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
