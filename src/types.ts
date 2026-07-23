export interface ConfigView {
  configured: boolean
  nodePath: string
  piPath: string
  provider: string
  model: string
  pricing: ModelPricing
  baseUrl: string
  thinking: string
  authMode: "api_key" | "environment" | "pi_auth" | ""
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
  updatedAt: string
}
export interface IssueBudgetConfig {
  tokenLimit: number | null
  costLimit: number | null
  timeLimitMinutes: number | null
  checkIntervalSeconds: number
}
export interface IssueHeartbeatConfig {
  intervalSeconds: number
}
export interface RuntimeProbe {
  ready: boolean
  nodePath: string
  nodeVersion?: string
  piPath: string
  piVersion?: string
  authFound: boolean
  error?: string
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
  priority: "critical" | "high" | "medium" | "low"
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
export interface ContainerProfile {
  id: string
  name: string
  description: string
  image: string
  nodePath: string
  piPath: string
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
  nodePath: string
  piPath: string
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
  | "cancelled"
export type IssueExecutionPhase =
  | "active"
  | "waiting_children"
  | "resuming"
  | "validating"
  | "summarizing"
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
  priority: "critical" | "high" | "medium" | "low"
  workMode: "guided" | "autonomous"
  executionPhase: IssueExecutionPhase
  requestDepth: number
  assigneeAgentId?: string
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
  | "stopped"
  | "cancelled"
  | "disconnected"
export interface Execution {
  id: string
  issueId: string
  agentId: string
  kind:
    "planning" | "work" | "continuation" | "validation" | "rework" | "wakeup"
  status: ExecutionStatus
  provider: string
  model: string
  pricing: ModelPricing
  thinking: string
  sessionId: string
  issueAgentSessionId?: string
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
  error?: string
  cost: number
  tokens: number
  inputTokens: number
  outputTokens: number
  cacheReadTokens: number
  cacheWriteTokens: number
  messageCount: number
  startedAt: string
  updatedAt: string
  finishedAt?: string
}
export interface IssueAgentSession {
  id: string
  issueId: string
  agentId: string
  sessionId: string
  status: "active" | "broken" | "superseded" | "archived"
  generation: number
  runtimeType: "host" | "container"
  containerProfileId?: string
  workspace: string
  createdAt: string
  updatedAt: string
}
export interface ToolSnapshot {
  name: string
  label: string
  description: string
  source: "pi_builtin" | "aegis_extension" | "unknown"
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
export interface TaskBroadcast {
  id: string
  taskId: string
  sourceIssueId: string
  sourceIssueIdentifier: string
  sourceIssueTitle: string
  sourceExecutionId: string
  sourceAgentId: string
  sourceAgentName: string
  subject: string
  message: string
  importance: "normal" | "important" | "critical"
  deliveredCount: number
  recipientExecutionIds: string[]
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
  children: Issue[]
  blockedBy: Issue[]
  blocks: Issue[]
  executions: Execution[]
  agentSessions: IssueAgentSession[]
  comments: IssueComment[]
  messages: Message[]
  events: ExecutionEvent[]
  approvals: Approval[]
  wakeups: AgentWakeup[]
  decompositions: IssueDecomposition[]
  validations: IssueValidation[]
  broadcasts: TaskBroadcast[]
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
  templateId: string
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
  departmentId?: string
  createdAt: string
  updatedAt: string
}
export interface Department {
  id: string
  parentId?: string
  name: string
  code: string
  description: string
  leaderAgentId?: string
  enabled: boolean
  createdAt: string
  updatedAt: string
}
export interface AgentTemplateMetadata {
  englishName: string
  chineseName: string
  introduction: string
  positions: string[]
}
export interface AgentTemplate {
  id: string
  provider: string
  model: string
  systemPrompt: string
  note: string
  metadata: AgentTemplateMetadata
  hidden: boolean
  builtin: boolean
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
  runtime: RuntimeProbe
  projects: Project[]
  containerProfiles: ContainerProfile[]
  containers: ContainerInstance[]
  tasks: Task[]
  issues: Issue[]
  relations: IssueRelation[]
  executions: Execution[]
  approvals: Approval[]
  agents: AgentDefinition[]
  departments: Department[]
  skills: SkillDefinition[]
  knowledgeBases: KnowledgeBase[]
  sessions: SessionSummary[]
  updatedAt: string
}
export interface SaveConfigInput {
  nodePath: string
  piPath: string
  provider: string
  model: string
  pricing: ModelPricing
  baseUrl: string
  thinking: string
  authMode: "api_key" | "environment" | "pi_auth"
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
}
export interface CreateIssueInput {
  projectId?: string
  parentId?: string
  title: string
  description: string
  objective: string
  priority: "critical" | "high" | "medium" | "low"
  workMode: "guided" | "autonomous"
  assigneeAgentId?: string
  workspace: string
  containerProfileId?: string
  context: string
  constraints: string
  timeBudgetMinutes?: number
  humanValidationFallback?: boolean
  blockedBy?: string[]
}
export interface ConnectionTestResult {
  ok: boolean
  reply?: string
  durationMs: number
  error?: string
}
export interface Finding {
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
