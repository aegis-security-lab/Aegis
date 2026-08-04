export interface ConfigView {
  configured: boolean
  language: "zh" | "en"
  provider: string
  model: string
  pricing: ModelPricing
  baseUrl: string
  thinking: string
  authMode: "api_key" | "environment" | ""
  hasApiKey: boolean
  workspace: string
  concurrency: number
  approvalMode: "all" | "risky" | "none" | ""
  reworkApprovalMode: "all" | "none" | ""
  validationMode: "fixed" | "automatic"
  maxValidationAttempts: number
  maxIssueDepth: number
  maxChildrenPerRequest: number
  maxDirectChildren: number
  issueBudget: IssueBudgetConfig
  issueHeartbeat: IssueHeartbeatConfig
  webSearch: WebSearchConfigView
  updatedAt: string
}
export interface IssueBudgetConfig {
  maxTurns: number
  activeTimeMinutes: number
  summaryTurns: number
  summaryTimeMinutes: number
}
export interface IssueHeartbeatConfig {
  intervalSeconds: number
}
export interface WebSearchConfigView {
  engine: "tavily"
  baseUrl: string
  hasApiKey: boolean
  enabled: boolean
}
export interface WebSearchConfig {
  engine: "tavily"
  baseUrl: string
  apiKey: string
  enabled: boolean
}
export interface WebSearchInput {
  query: string
  topic: "general" | "news" | "finance"
  searchDepth: "basic" | "advanced"
  includeAnswer: boolean
  maxResults: number
}
export interface WebSearchItem {
  title: string
  url: string
  content: string
  score?: number
}
export interface WebSearchResult {
  engine: string
  query: string
  answer?: string
  results: WebSearchItem[]
  responseTime?: number
}
export interface Project {
  id: string
  key: string
  name: string
  description: string
  workspace: string
  status: string
  createdAt: string
  updatedAt: string
}
export interface Task {
  id: string
  projectId: string
  title: string
  description: string
  objective: string
  priority: "high" | "middle" | "low"
  workMode: "guided" | "autonomous"
  assigneeAgentId?: string
  workspace: string
  containerProfileId?: string
  containerId?: string
  context?: string
  constraints?: string
  timeBudgetMinutes?: number
  humanValidationFallback?: boolean
  createdAt: string
  updatedAt: string
}
export interface TaskAudit {
  id: string
  taskId?: string
  requestedId: string
  rootIssueIds: string[]
  status: "preparing" | "running" | "completed" | "failed"
  frameworkVersion: string
  evidenceExportId?: string
  evidenceComplete: boolean
  evidenceManifest?: string
  reportMarkdown?: string
  error?: string
  inputTokens: number
  outputTokens: number
  startedAt?: string
  completedAt?: string
  createdAt: string
  updatedAt: string
}
export interface ContainerProfile {
  id: string
  name: string
  description: string
  image: string
  workspacePath: string
  networkMode: "bridge" | "none"
  memoryMb: number
  cpus: number
  enabled: boolean
  createdAt: string
  updatedAt: string
}

export interface ContainerInstance {
  id: string
  containerProfileId: string
  taskId: string
  name: string
  image: string
  workspacePath: string
  networkMode: "bridge" | "none"
  memoryMb: number
  cpus: number
  runtimeStatus:
    | "created"
    | "running"
    | "paused"
    | "restarting"
    | "removing"
    | "exited"
    | "dead"
    | "missing"
    | "unavailable"
  createdAt: string
  updatedAt: string
}

export interface ContainerProfileDeleteImpact {
  containerProfileId: string
  containerCount: number
  issueCount: number
  taskCount: number
  executionCount: number
  activeExecutionCount: number
}

export interface ContainerDeleteImpact {
  containerId: string
  taskId: string
  issueCount: number
  executionCount: number
  activeExecutionCount: number
}

export interface ContainerDeleteResult {
  containerId: string
  deletedIssues: number
  deletedTasks: number
  deletedExecutions: number
}

export interface ContainerBatchFailure {
  containerId: string
  error: string
}

export interface ContainerBatchStopResult {
  requested: number
  stopped: ContainerInstance[]
  failed: ContainerBatchFailure[]
}

export interface ContainerBatchDeleteImpact {
  containerIds: string[]
  containerCount: number
  taskCount: number
  issueCount: number
  executionCount: number
  activeExecutionCount: number
  items: ContainerDeleteImpact[]
}

export interface ContainerBatchDeleteResult {
  requested: number
  deleted: ContainerDeleteResult[]
  failed: ContainerBatchFailure[]
  deletedIssues: number
  deletedTasks: number
  deletedExecutions: number
}

export interface ContainerProfileDeleteResult {
  containerProfileId: string
  deletedIssues: number
  deletedTasks: number
  deletedExecutions: number
}
export type IssueStatus =
  | "backlog"
  | "todo"
  | "in_progress"
  | "in_review"
  | "done"
  | "blocked"
  | "failed"
  | "budget_exceeded"
  | "cancelled"
export type IssueExecutionPhase =
  | "active"
  | "scheduled"
  | "waiting_children"
  | "sleeping"
  | "resuming"
  | "validating"
  | "summarizing"
  | "budget_summarizing"
  | "budget_exceeded"
  | "recovering"
  | "completed"
  | "blocked"
export interface Issue {
  id: string
  number: number
  identifier: string
  projectId: string
  parentId?: string
  taskSourceId?: string
  title: string
  description: string
  objective: string
  status: IssueStatus
  priority: "high" | "middle" | "low"
  workMode: "guided" | "autonomous"
  executionPhase: IssueExecutionPhase
  requestDepth: number
  assigneeAgentId?: string
  assigneeTaskAgentId?: string
  checkoutExecutionId?: string
  currentExecutionId?: string
  validationExecutionId?: string
  validationMode: "fixed" | "automatic"
  maxValidationAttempts: number
  validationDisabled: boolean
  recoveryExecutionId?: string
  recoveryPhase?: string
  recoveryRequestedAt?: string
  workspace: string
  containerProfileId?: string
  containerId?: string
  context?: string
  constraints?: string
  timeBudgetMinutes?: number
  humanValidationFallback?: boolean
  result?: string
  error?: string
  objectiveAbandoned: boolean
  abandonmentReason?: string
  abandonRequestedAt?: string
  abandonedAt?: string
  createdBy: string
  startedAt?: string
  completedAt?: string
  cancelledAt?: string
  createdAt: string
  updatedAt: string
}
export type IssueRuntimeKind =
  "running" | "waiting" | "failed" | "completed" | "cancelled" | "pending"
export interface IssueRuntimeView {
  issueId: string
  state: string
  kind: IssueRuntimeKind
  health: "healthy" | "waiting" | "stalled" | "error"
  currentExecutionId?: string
  currentExecutionStatus?: string
  detail?: string
  updatedAt: string
}
export interface IssueRelation {
  id: string
  issueId: string
  relatedIssueId: string
  type: "blocks"
  createdAt: string
  updatedAt: string
}
export type ExecutionStatus =
  | "queued"
  | "starting"
  | "running"
  | "waiting_approval"
  | "completed"
  | "failed"
  | "budget_exceeded"
  | "stopped"
  | "cancelled"
  | "disconnected"
export interface Execution {
  id: string
  issueId: string
  agentId: string
  kind:
    | "planning"
    | "work"
    | "continuation"
    | "validation"
    | "rework"
    | "wakeup"
    | "heartbeat"
    | "recovery"
    | "concierge"
    | "chat"
  status: ExecutionStatus
  provider: string
  model: string
  pricing: ModelPricing
  thinking: string
  sessionId: string
  pid?: number
  runtimeType: "host" | "container"
  runtimeId?: string
  containerProfileId?: string
  containerImage?: string
  currentTool?: string
  checkpoint?: string
  checkpointAt?: string
  initialPrompt?: string
  systemPrompt?: string
  toolsSnapshot: ToolSnapshot[]
  result?: string
  finalResult?: string
  finalResultSubmitted: boolean
  error?: string
  cost: number
  tokens: number
  inputTokens: number
  outputTokens: number
  cacheReadTokens: number
  cacheWriteTokens: number
  messageCount: number
  budgetMaxTurns: number
  budgetActiveMinutes: number
  budgetSummaryTurns: number
  budgetSummaryMinutes: number
  budgetPhase?: "active" | "summarizing" | "exceeded"
  budgetTurnsUsed: number
  budgetSummaryUsed: number
  budgetExceededReason?: string
  budgetExceededAt?: string
  startedAt: string
  updatedAt: string
  finishedAt?: string
}
export interface ToolSnapshot {
  name: string
  label: string
  description: string
  source: "agentcore_builtin" | "aegis_extension" | "unknown"
  parameters: ToolParameterSnapshot[]
}
export interface ToolParameterSnapshot {
  name: string
  type: string
  description: string
  required: boolean
  enum?: string[]
  children?: ToolParameterSnapshot[]
}
export interface ExecutionEvent {
  id: string
  executionId: string
  issueId?: string
  type: string
  title: string
  detail: string
  toolCallId?: string
  toolName?: string
  status?: "running" | "completed" | "failed" | "interrupted"
  inputJson?: string
  outputJson?: string
  isError?: boolean
  createdAt: string
  updatedAt?: string
}
export interface ExecutionProgress {
  id: string
  executionId: string
  issueId: string
  stage: string
  summary: string
  currentActivity: string
  createdAt: string
}
export interface Message {
  id: string
  executionId: string
  issueId?: string
  role: "user" | "assistant" | "system"
  content: string
  streaming: boolean
  createdAt: string
  updatedAt: string
  attachments?: InputAttachment[]
}
export interface OperatorAttachment {
  name: string
  path: string
  size: number
}
export interface InputAttachment {
  id: string
  scope: "task" | "employee" | "concierge"
  ownerId?: string
  taskId?: string
  issueId?: string
  executionId?: string
  messageId?: string
  name: string
  mimeType: string
  size: number
  createdAt: string
  boundAt?: string
}
export interface ConciergeConversation {
  id: string
  title: string
  executionId: string
  status: "idle" | "running" | "disconnected" | "error"
  lastMessage?: string
  messageCount: number
  createdTaskId?: string
  createdAt: string
  updatedAt: string
}
export interface ConciergeConversationDetail {
  conversation: ConciergeConversation
  execution: Execution
  messages: Message[]
  messagesPage: PageInfo
  watermark: string
}
export interface Approval {
  id: string
  executionId: string
  issueId?: string
  type: "tool_call" | "issue_rework" | "validation_review"
  title: string
  detail: string
  status: "pending" | "approved" | "rejected"
  createdAt: string
  resolvedAt?: string
}
export interface IssueComment {
  id: string
  issueId: string
  authorType: "operator" | "agent" | "system"
  authorId: string
  body: string
  mentions: string[]
  attachments: IssueAttachment[]
  createdAt: string
}
export interface IssueAttachment {
  id: string
  issueId: string
  commentId?: string
  executionId: string
  name: string
  description?: string
  sourcePath: string
  mimeType: string
  size: number
  createdAt: string
}
export interface AgentWakeup {
  id: string
  issueId: string
  commentId: string
  agentId: string
  executionId?: string
  reason: string
  status: "queued" | "delivered" | "completed" | "failed" | "cancelled"
  error?: string
  createdAt: string
  deliveredAt?: string
  completedAt?: string
}
export interface IssueDecomposition {
  id: string
  parentIssueId: string
  sourceExecutionId: string
  requestKey: string
  summary: string
  childIds: string[]
  createdAt: string
}
export interface IssueValidation {
  id: string
  issueId: string
  sourceExecutionId: string
  validationExecutionId: string
  attempt: number
  objective: string
  candidateResult: string
  status:
    | "running"
    | "passed"
    | "failed"
    | "abandoned"
    | "skipped"
    | "interrupted"
    | "error"
  passed: boolean
  summary: string
  feedback: string
  abandonmentProof?: string
  error?: string
  manualOverrideReason?: string
  createdAt: string
  completedAt?: string
}
export interface IssueDetail {
  issue: Issue
  runtime: IssueRuntimeView
  children: Issue[]
  blockedBy: Issue[]
  blocks: Issue[]
  executions: Execution[]
  comments: IssueComment[]
  messages: Message[]
  inputAttachments: InputAttachment[]
  events: ExecutionEvent[]
  approvals: Approval[]
  wakeups: AgentWakeup[]
  decompositions: IssueDecomposition[]
  validations: IssueValidation[]
  commentsPage: PageInfo
  eventsPage: PageInfo
  executionsPage: PageInfo
  watermark: string
}
export interface PageInfo {
  nextCursor?: string
  hasMore: boolean
  total: number
}
export interface CursorPage<T> {
  items: T[]
  page: PageInfo
}
export interface TaskCancellationResult {
  task: Issue
  totalIssues: number
  cancelledIssues: number
  cancelledExecutions: number
  removedApprovals: number
  cancelledWakeups: number
}
export interface WorkspaceEntry {
  path: string
  kind: "file" | "directory" | "symlink"
  size?: number
  modifiedAt: string
}
export interface TaskWorkspace {
  root: string
  entries: WorkspaceEntry[]
}

export interface ToolInterruptResult {
  executionId: string
  tool: string
  status: "interrupting"
}
export type TaskTimelineEventKind =
  | "issue"
  | "execution"
  | "result"
  | "comment"
  | "validation"
  | "approval"
  | "system"
export interface TaskTimelineEvent {
  id: string
  kind: TaskTimelineEventKind
  issueId: string
  issueIdentifier: string
  issueTitle: string
  executionId?: string
  actorType: "operator" | "agent" | "system"
  actorId?: string
  actorName: string
  title: string
  summary?: string
  detail?: string
  status?: string
  createdAt: string
}
export interface TaskTimeline {
  task: Issue
  issueCount: number
  events: TaskTimelineEvent[]
}
export interface AgentModelConfig {
  provider: string
  model: string
  baseUrl: string
  thinking: string
  pricing: ModelPricing | null
}
export interface ModelPricing {
  input: number
  output: number
  cacheRead: number
  cacheWrite: number
}
export interface PermissionBoundary {
  workspaceScope: "run_workspace"
  allowNetwork: boolean
  allowShell: boolean
  allowWrite: boolean
  approvalMode: "" | "all" | "risky" | "none"
  reworkApprovalMode: "" | "all" | "none"
}
export interface AgentDefinition {
  id: string
  name: string
  description: string
  avatar: string
  category: string
  enabled: boolean
  builtin: boolean
  internal: boolean
  model: AgentModelConfig
  systemPrompt: string
  memo: string
  tools: string[]
  skillIds: string[]
  knowledgeBaseIds: string[]
  permissions: PermissionBoundary
  createdAt: string
  updatedAt: string
}

export type CapabilityKind = "tool" | "skill" | "mcp" | "phone" | "web" | string

export interface CapabilityRef {
  kind: CapabilityKind
  name: string
  version?: string
  config?: Record<string, unknown>
  optional?: boolean
}

export interface CapabilityDescriptor {
  kind: CapabilityKind
  name: string
  dynamic?: boolean
}

export interface CapabilityPattern {
  kind?: CapabilityKind
  name?: string
}

export interface CapabilityPolicy {
  defaultSelection?: "inherit" | "merge" | "replace"
  allowed?: CapabilityPattern[]
  denied?: CapabilityPattern[]
  required?: CapabilityRef[]
  maxCapabilities?: number
}

export interface CoordinationBinding {
  coordinationId: string
  mode: string
  version: string
  config?: {
    capabilityPolicy?: CapabilityPolicy
    [key: string]: unknown
  }
  updatedAt: string
}

export interface TaskPhone {
  id: string
  taskId: string
  agentId: string
  taskAgentId?: string
  name?: string
  executionId?: string
  workspaceId?: string
  installedApps: string[]
  activeAppId?: string
  currentAppId?: string
  currentPageId?: string
  actionCount: number
  lastActionAt?: string
  createdAt: string
  updatedAt: string
}

export interface TaskAgent {
  id: string
  taskId: string
  agentId: string
  name: string
  status: string
  createdAt: string
  updatedAt: string
}
export interface KnowledgeBase {
  id: string
  name: string
  description: string
  retrievalProvider: string
  documentCount: number
  createdAt: string
  updatedAt: string
}
export interface KnowledgeDocument {
  id: string
  knowledgeBaseId: string
  name: string
  content: string
  createdAt: string
  updatedAt: string
}
export interface KnowledgeBaseDetail {
  knowledgeBase: KnowledgeBase
  documents: KnowledgeDocument[]
}
export interface SkillDefinition {
  id: string
  name: string
  displayName: string
  description: string
  content: string
  source: string
  version: string
  builtin: boolean
  createdAt: string
  updatedAt: string
}
export interface SessionSummary {
  execution: Execution
  issueIdentifier: string
  issueTitle: string
  agentName: string
}
export interface SessionDetail {
  session: SessionSummary
  messages: Message[]
  events: ExecutionEvent[]
  progressUpdates: ExecutionProgress[]
  approvals: Approval[]
  messagesPage: PageInfo
  eventsPage: PageInfo
  progressPage: PageInfo
  watermark: string
}
export interface SessionDelta {
  execution: Execution
  messages: Message[]
  events: ExecutionEvent[]
  progressUpdates: ExecutionProgress[]
  watermark: string
}
export interface AppState {
  configured: boolean
  config: ConfigView
  projects: Project[]
  containerProfiles: ContainerProfile[]
  containers: ContainerInstance[]
  tasks: Task[]
  issues: Issue[]
  issueRuntimes: IssueRuntimeView[]
  relations: IssueRelation[]
  executions: Execution[]
  approvals: Approval[]
  agents: AgentDefinition[]
  taskAgents: TaskAgent[]
  skills: SkillDefinition[]
  knowledgeBases: KnowledgeBase[]
  sessions: SessionSummary[]
  updatedAt: string
}
export interface SaveConfigInput {
  language: "zh" | "en"
  provider: string
  model: string
  pricing: ModelPricing
  baseUrl: string
  thinking: string
  authMode: "api_key" | "environment"
  apiKey: string
  workspace: string
  concurrency: number
  approvalMode: "all" | "risky" | "none"
  reworkApprovalMode: "all" | "none"
  validationMode: "fixed" | "automatic"
  maxValidationAttempts: number
  maxIssueDepth: number
  maxChildrenPerRequest: number
  maxDirectChildren: number
  issueBudget: IssueBudgetConfig
  issueHeartbeat: IssueHeartbeatConfig
  webSearch: WebSearchConfig
}
export interface CreateIssueInput {
  projectId?: string
  parentId?: string
  title: string
  description: string
  objective: string
  priority: "high" | "middle" | "low"
  status?: IssueStatus
  workMode: "guided" | "autonomous"
  assigneeAgentId?: string
  workspace: string
  containerProfileId?: string
  context: string
  constraints: string
  timeBudgetMinutes?: number
  humanValidationFallback?: boolean
  blockedBy?: string[]
  attachmentIds?: string[]
}
export interface ConnectionTestResult {
  ok: boolean
  reply?: string
  durationMs: number
  error?: string
}
export interface Finding {
  auditLevel?: AuditLevel
  id: string
  domain: string
  category: string
  severity: string
  title: string
  description: string
  evidence: unknown
  stepsToReproduce: string
  remediation: string
  status: string
  createdAt: string
  updatedAt: string
}

export type AuditLevel = "S" | "A" | "B" | "C" | "D" | "E" | "I"
export interface FindingList {
  findings: Finding[]
  total: number
  page: number
  pageSize: number
}
export interface UncoverEngine {
  id: string
  name: string
  configured: boolean
  anonymous: boolean
  credentialFields: UncoverCredentialField[]
  docsUrl: string
  example: string
}
export interface UncoverCredentialField {
  key: string
  label: string
  description: string
  secret: boolean
  configured: boolean
  maskedValue?: string
}
export interface UncoverStatus {
  engines: UncoverEngine[]
  formats: Array<"txt" | "json" | "jsonl" | "csv">
  textFields: Array<"ip:port" | "host:port" | "ip" | "host" | "port" | "url">
}
export interface SaveUncoverProviderInput {
  values: Record<string, string>
}
export interface UncoverSearchInput {
  engine: string
  query: string
  limit: number
  format: "txt" | "json" | "jsonl" | "csv"
  field: "ip:port" | "host:port" | "ip" | "host" | "port" | "url"
  timeout: number
}
export interface UncoverAsset {
  timestamp: number
  source: string
  ip: string
  port: number
  host: string
  url: string
}
export interface UncoverExportInfo {
  id: string
  name: string
  format: string
  field?: string
  mimeType: string
  size: number
  downloadUrl: string
}
export interface UncoverSearchResult {
  id: string
  engine: string
  query: string
  count: number
  preview: UncoverAsset[]
  previewLimited: boolean
  warnings: string[]
  export: UncoverExportInfo
  attachment?: UncoverExportInfo
  durationMs: number
  createdAt: string
}
