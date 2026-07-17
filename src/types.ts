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
  updatedAt: string
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
export type IssueStatus =
  | "backlog"
  | "todo"
  | "in_progress"
  | "in_review"
  | "done"
  | "blocked"
  | "cancelled"
export type IssueExecutionPhase =
  "active" | "waiting_children" | "resuming" | "completed" | "blocked"
export interface Issue {
  id: string
  number: number
  identifier: string
  projectId: string
  parentId?: string
  title: string
  description: string
  acceptanceCriteria: string
  status: IssueStatus
  priority: "critical" | "high" | "medium" | "low"
  workMode: "guided" | "autonomous"
  executionPhase: IssueExecutionPhase
  requestDepth: number
  assigneeAgentId?: string
  checkoutExecutionId?: string
  currentExecutionId?: string
  workspace: string
  context?: string
  constraints?: string
  result?: string
  error?: string
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
  kind: "planning" | "work" | "continuation" | "wakeup"
  status: ExecutionStatus
  provider: string
  model: string
  pricing: ModelPricing
  thinking: string
  sessionId: string
  pid?: number
  currentTool?: string
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
  status?: "running" | "completed" | "failed"
  inputJson?: string
  outputJson?: string
  isError?: boolean
  createdAt: string
  updatedAt?: string
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
export interface Approval {
  id: string
  executionId: string
  issueId?: string
  title: string
  detail: string
  status: "pending" | "approved" | "rejected" | "expired"
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
export interface IssueDetail {
  issue: Issue
  children: Issue[]
  blockedBy: Issue[]
  blocks: Issue[]
  executions: Execution[]
  comments: IssueComment[]
  messages: Message[]
  events: ExecutionEvent[]
  approvals: Approval[]
  wakeups: AgentWakeup[]
  decompositions: IssueDecomposition[]
}
export interface TaskCancellationResult {
  task: Issue
  totalIssues: number
  cancelledIssues: number
  cancelledExecutions: number
  expiredApprovals: number
  cancelledWakeups: number
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
  tools: string[]
  skillIds: string[]
  knowledgeBaseIds: string[]
  permissions: PermissionBoundary
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
  approvals: Approval[]
}
export interface AppState {
  configured: boolean
  config: ConfigView
  runtime: RuntimeProbe
  projects: Project[]
  issues: Issue[]
  relations: IssueRelation[]
  executions: Execution[]
  approvals: Approval[]
  agents: AgentDefinition[]
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
}
export interface CreateIssueInput {
  projectId?: string
  parentId?: string
  title: string
  description: string
  acceptanceCriteria: string
  priority: "critical" | "high" | "medium" | "low"
  workMode: "guided" | "autonomous"
  assigneeAgentId?: string
  workspace: string
  context: string
  constraints: string
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
