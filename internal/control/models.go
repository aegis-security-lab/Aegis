package control

import "time"

type Config struct {
	Configured   bool         `json:"configured"`
	NodePath     string       `json:"nodePath"`
	PiPath       string       `json:"piPath"`
	Provider     string       `json:"provider"`
	Model        string       `json:"model"`
	Pricing      ModelPricing `json:"pricing"`
	BaseURL      string       `json:"baseUrl"`
	Thinking     string       `json:"thinking"`
	AuthMode     string       `json:"authMode"`
	APIKey       string       `json:"apiKey,omitempty"`
	Workspace    string       `json:"workspace"`
	Concurrency  int          `json:"concurrency"`
	ApprovalMode string       `json:"approvalMode"`
	UpdatedAt    time.Time    `json:"updatedAt"`
}

type ConfigView struct {
	Configured   bool         `json:"configured"`
	NodePath     string       `json:"nodePath"`
	PiPath       string       `json:"piPath"`
	Provider     string       `json:"provider"`
	Model        string       `json:"model"`
	Pricing      ModelPricing `json:"pricing"`
	BaseURL      string       `json:"baseUrl"`
	Thinking     string       `json:"thinking"`
	AuthMode     string       `json:"authMode"`
	HasAPIKey    bool         `json:"hasApiKey"`
	Workspace    string       `json:"workspace"`
	Concurrency  int          `json:"concurrency"`
	ApprovalMode string       `json:"approvalMode"`
	UpdatedAt    time.Time    `json:"updatedAt"`
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

// Issue is the single source of truth for work. Top-level issues are Tasks; children are execution units.
type Issue struct {
	ID                  string     `json:"id" gorm:"primaryKey"`
	Number              int64      `json:"number" gorm:"uniqueIndex"`
	Identifier          string     `json:"identifier" gorm:"uniqueIndex"`
	ProjectID           string     `json:"projectId" gorm:"index"`
	ParentID            string     `json:"parentId,omitempty" gorm:"index"`
	Title               string     `json:"title"`
	Description         string     `json:"description"`
	AcceptanceCriteria  string     `json:"acceptanceCriteria"`
	Status              string     `json:"status" gorm:"index"`
	Priority            string     `json:"priority" gorm:"index"`
	WorkMode            string     `json:"workMode"`
	ExecutionPhase      string     `json:"executionPhase" gorm:"index"`
	RequestDepth        int        `json:"requestDepth"`
	AssigneeAgentID     string     `json:"assigneeAgentId,omitempty" gorm:"index"`
	CheckoutExecutionID string     `json:"checkoutExecutionId,omitempty" gorm:"index"`
	CurrentExecutionID  string     `json:"currentExecutionId,omitempty" gorm:"index"`
	Workspace           string     `json:"workspace"`
	Context             string     `json:"context,omitempty"`
	Constraints         string     `json:"constraints,omitempty"`
	Result              string     `json:"result,omitempty"`
	Error               string     `json:"error,omitempty"`
	CreatedBy           string     `json:"createdBy"`
	StartedAt           *time.Time `json:"startedAt,omitempty"`
	CompletedAt         *time.Time `json:"completedAt,omitempty"`
	CancelledAt         *time.Time `json:"cancelledAt,omitempty"`
	CreatedAt           time.Time  `json:"createdAt"`
	UpdatedAt           time.Time  `json:"updatedAt"`
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
	ID               string         `json:"id" gorm:"primaryKey"`
	IssueID          string         `json:"issueId" gorm:"index"`
	AgentID          string         `json:"agentId" gorm:"index"`
	Kind             string         `json:"kind"`
	Status           string         `json:"status" gorm:"index"`
	Provider         string         `json:"provider"`
	Model            string         `json:"model"`
	Pricing          ModelPricing   `json:"pricing" gorm:"serializer:json;type:text"`
	Thinking         string         `json:"thinking"`
	SessionID        string         `json:"sessionId" gorm:"uniqueIndex"`
	PID              int            `json:"pid,omitempty" gorm:"column:pid"`
	CurrentTool      string         `json:"currentTool,omitempty"`
	InitialPrompt    string         `json:"initialPrompt,omitempty" gorm:"type:text"`
	SystemPrompt     string         `json:"systemPrompt,omitempty" gorm:"type:text"`
	ToolsSnapshot    []ToolSnapshot `json:"toolsSnapshot" gorm:"serializer:json;type:text"`
	Result           string         `json:"result,omitempty"`
	Error            string         `json:"error,omitempty"`
	Cost             float64        `json:"cost"`
	Tokens           int64          `json:"tokens"`
	InputTokens      int64          `json:"inputTokens"`
	OutputTokens     int64          `json:"outputTokens"`
	CacheReadTokens  int64          `json:"cacheReadTokens"`
	CacheWriteTokens int64          `json:"cacheWriteTokens"`
	MessageCount     int            `json:"messageCount"`
	StartedAt        time.Time      `json:"startedAt"`
	UpdatedAt        time.Time      `json:"updatedAt"`
	FinishedAt       *time.Time     `json:"finishedAt,omitempty"`
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
	ExecutionID string    `json:"executionId" gorm:"index"`
	IssueID     string    `json:"issueId,omitempty" gorm:"index"`
	Type        string    `json:"type"`
	Title       string    `json:"title"`
	Detail      string    `json:"detail"`
	ToolCallID  string    `json:"toolCallId,omitempty" gorm:"index"`
	ToolName    string    `json:"toolName,omitempty" gorm:"index"`
	Status      string    `json:"status,omitempty"`
	InputJSON   string    `json:"inputJson,omitempty" gorm:"type:text"`
	OutputJSON  string    `json:"outputJson,omitempty" gorm:"type:text"`
	IsError     bool      `json:"isError,omitempty"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
}
type Message struct {
	ID          string    `json:"id" gorm:"primaryKey"`
	ExecutionID string    `json:"executionId" gorm:"index"`
	IssueID     string    `json:"issueId,omitempty" gorm:"index"`
	Role        string    `json:"role"`
	Content     string    `json:"content" gorm:"type:text"`
	Streaming   bool      `json:"streaming"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
}
type Approval struct {
	ID          string     `json:"id" gorm:"primaryKey"`
	ExecutionID string     `json:"executionId" gorm:"index"`
	IssueID     string     `json:"issueId,omitempty" gorm:"index"`
	RequestID   string     `json:"-"`
	Title       string     `json:"title"`
	Detail      string     `json:"detail"`
	Status      string     `json:"status" gorm:"index"`
	CreatedAt   time.Time  `json:"createdAt"`
	ResolvedAt  *time.Time `json:"resolvedAt,omitempty"`
}
type IssueComment struct {
	ID          string            `json:"id" gorm:"primaryKey"`
	IssueID     string            `json:"issueId" gorm:"index"`
	AuthorType  string            `json:"authorType"`
	AuthorID    string            `json:"authorId"`
	Body        string            `json:"body" gorm:"type:text"`
	Mentions    []string          `json:"mentions" gorm:"serializer:json;type:text"`
	Attachments []IssueAttachment `json:"attachments" gorm:"-"`
	CreatedAt   time.Time         `json:"createdAt"`
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
type AgentWakeup struct {
	ID          string     `json:"id" gorm:"primaryKey"`
	IssueID     string     `json:"issueId" gorm:"index"`
	CommentID   string     `json:"commentId"`
	AgentID     string     `json:"agentId" gorm:"index"`
	ExecutionID string     `json:"executionId,omitempty"`
	Reason      string     `json:"reason"`
	Status      string     `json:"status" gorm:"index"`
	Error       string     `json:"error,omitempty"`
	CreatedAt   time.Time  `json:"createdAt"`
	DeliveredAt *time.Time `json:"deliveredAt,omitempty"`
	CompletedAt *time.Time `json:"completedAt,omitempty"`
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
	Title              string `json:"title"`
	Description        string `json:"description"`
	AcceptanceCriteria string `json:"acceptanceCriteria"`
	Priority           string `json:"priority"`
	AgentID            string `json:"agentId"`
	DependsOn          []int  `json:"dependsOn"`
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
	WorkspaceScope string `json:"workspaceScope"`
	AllowNetwork   bool   `json:"allowNetwork"`
	AllowShell     bool   `json:"allowShell"`
	AllowWrite     bool   `json:"allowWrite"`
	ApprovalMode   string `json:"approvalMode"`
}
type AgentDefinition struct {
	ID           string             `json:"id"`
	Name         string             `json:"name"`
	Description  string             `json:"description"`
	Avatar       string             `json:"avatar"`
	Category     string             `json:"category"`
	Enabled      bool               `json:"enabled"`
	Builtin      bool               `json:"builtin"`
	Model        AgentModelConfig   `json:"model"`
	SystemPrompt string             `json:"systemPrompt"`
	Tools        []string           `json:"tools"`
	SkillIDs     []string           `json:"skillIds"`
	Permissions  PermissionBoundary `json:"permissions"`
	CreatedAt    time.Time          `json:"createdAt"`
	UpdatedAt    time.Time          `json:"updatedAt"`
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
	ID           string             `json:"id"`
	Name         string             `json:"name"`
	Description  string             `json:"description"`
	Avatar       string             `json:"avatar"`
	Category     string             `json:"category"`
	Enabled      bool               `json:"enabled"`
	Model        AgentModelConfig   `json:"model"`
	SystemPrompt string             `json:"systemPrompt"`
	Tools        []string           `json:"tools"`
	SkillIDs     []string           `json:"skillIds"`
	Permissions  PermissionBoundary `json:"permissions"`
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
	Comments       []IssueComment       `json:"comments"`
	Messages       []Message            `json:"messages"`
	Events         []ExecutionEvent     `json:"events"`
	Approvals      []Approval           `json:"approvals"`
	Wakeups        []AgentWakeup        `json:"wakeups"`
	Decompositions []IssueDecomposition `json:"decompositions"`
}
type SessionSummary struct {
	Execution       Execution `json:"execution"`
	IssueIdentifier string    `json:"issueIdentifier"`
	IssueTitle      string    `json:"issueTitle"`
	AgentName       string    `json:"agentName"`
}
type SessionDetail struct {
	Session   SessionSummary   `json:"session"`
	Messages  []Message        `json:"messages"`
	Events    []ExecutionEvent `json:"events"`
	Approvals []Approval       `json:"approvals"`
}
type StateView struct {
	Configured bool              `json:"configured"`
	Config     ConfigView        `json:"config"`
	Runtime    RuntimeProbe      `json:"runtime"`
	Projects   []Project         `json:"projects"`
	Issues     []Issue           `json:"issues"`
	Relations  []IssueRelation   `json:"relations"`
	Executions []Execution       `json:"executions"`
	Approvals  []Approval        `json:"approvals"`
	Agents     []AgentDefinition `json:"agents"`
	Skills     []SkillDefinition `json:"skills"`
	Sessions   []SessionSummary  `json:"sessions"`
	UpdatedAt  time.Time         `json:"updatedAt"`
}

type CreateIssueInput struct {
	ProjectID          string   `json:"projectId"`
	ParentID           string   `json:"parentId"`
	Title              string   `json:"title"`
	Description        string   `json:"description"`
	AcceptanceCriteria string   `json:"acceptanceCriteria"`
	Priority           string   `json:"priority"`
	Status             string   `json:"status"`
	WorkMode           string   `json:"workMode"`
	AssigneeAgentID    string   `json:"assigneeAgentId"`
	Workspace          string   `json:"workspace"`
	Context            string   `json:"context"`
	Constraints        string   `json:"constraints"`
	BlockedBy          []string `json:"blockedBy"`
}
type UpdateIssueInput struct {
	Title              *string `json:"title"`
	Description        *string `json:"description"`
	AcceptanceCriteria *string `json:"acceptanceCriteria"`
	Priority           *string `json:"priority"`
	Status             *string `json:"status"`
	AssigneeAgentID    *string `json:"assigneeAgentId"`
	ParentID           *string `json:"parentId"`
}

type CancelTaskInput struct {
	Reason string `json:"reason"`
}

type TaskCancellationResult struct {
	Task                Issue `json:"task"`
	TotalIssues         int   `json:"totalIssues"`
	CancelledIssues     int64 `json:"cancelledIssues"`
	CancelledExecutions int64 `json:"cancelledExecutions"`
	ExpiredApprovals    int64 `json:"expiredApprovals"`
	CancelledWakeups    int64 `json:"cancelledWakeups"`
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
	NodePath     string       `json:"nodePath"`
	PiPath       string       `json:"piPath"`
	Provider     string       `json:"provider"`
	Model        string       `json:"model"`
	Pricing      ModelPricing `json:"pricing"`
	BaseURL      string       `json:"baseUrl"`
	Thinking     string       `json:"thinking"`
	AuthMode     string       `json:"authMode"`
	APIKey       string       `json:"apiKey"`
	Workspace    string       `json:"workspace"`
	Concurrency  int          `json:"concurrency"`
	ApprovalMode string       `json:"approvalMode"`
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
