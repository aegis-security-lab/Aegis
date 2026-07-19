import type {
  AgentDefinition,
  AppState,
  Approval,
  ConciergeConversation,
  ConciergeConversationDetail,
  ConnectionTestResult,
  CreateIssueInput,
  CursorPage,
  Execution,
  Finding,
  FindingList,
  ExecutionEvent,
  ExecutionProgress,
  Issue,
  IssueComment,
  IssueDetail,
  KnowledgeBase,
  KnowledgeBaseDetail,
  KnowledgeDocument,
  Message,
  RuntimeProbe,
  SaveConfigInput,
  SaveUncoverProviderInput,
  SessionDetail,
  SessionDelta,
  SkillDefinition,
  TaskCancellationResult,
  ToolInterruptResult,
  TaskTimeline,
  UncoverEngine,
  UncoverSearchInput,
  UncoverSearchResult,
  UncoverStatus,
} from "@/types"
export type SaveAgentInput = Omit<
  AgentDefinition,
  "builtin" | "createdAt" | "updatedAt"
>
export type SaveSkillInput = Omit<
  SkillDefinition,
  "builtin" | "createdAt" | "updatedAt"
>
export type SaveKnowledgeBaseInput = Pick<
  KnowledgeBase,
  "name" | "description" | "retrievalProvider"
>
export type SaveKnowledgeDocumentInput = Pick<
  KnowledgeDocument,
  "name" | "content"
>
async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const response = await fetch(path, {
    ...init,
    headers: { "Content-Type": "application/json", ...init?.headers },
  })
  if (!response.ok) {
    const body = (await response.json().catch(() => null)) as {
      error?: string
    } | null
    throw new Error(body?.error ?? `请求失败 (${response.status})`)
  }
  if (response.status === 204) return undefined as T
  return response.json() as Promise<T>
}
export const fetchState = () => request<AppState>("/api/state")
export const fetchConciergeConversations = () =>
  request<{ conversations: ConciergeConversation[] }>(
    "/api/concierge/conversations"
  )
export const createConciergeConversation = () =>
  request<ConciergeConversation>("/api/concierge/conversations", {
    method: "POST",
  })
export const fetchConciergeConversation = (id: string) =>
  request<ConciergeConversationDetail>(
    `/api/concierge/conversations/${encodeURIComponent(id)}`
  )
export const deleteConciergeConversation = (id: string) =>
  request<void>(`/api/concierge/conversations/${encodeURIComponent(id)}`, {
    method: "DELETE",
  })
export const sendConciergeMessage = (id: string, message: string) =>
  request<Message>(
    `/api/concierge/conversations/${encodeURIComponent(id)}/messages`,
    { method: "POST", body: JSON.stringify({ message }) }
  )
export const probeRuntime = (nodePath: string, piPath: string) =>
  request<RuntimeProbe>("/api/setup/probe", {
    method: "POST",
    body: JSON.stringify({ nodePath, piPath }),
  })
export const testConnection = (input: SaveConfigInput) =>
  request<ConnectionTestResult>("/api/setup/test", {
    method: "POST",
    body: JSON.stringify(input),
  })
export const completeSetup = (input: SaveConfigInput) =>
  request<AppState>("/api/setup/complete", {
    method: "POST",
    body: JSON.stringify(input),
  })
export const saveSettings = (input: SaveConfigInput) =>
  request<AppState>("/api/settings", {
    method: "PUT",
    body: JSON.stringify(input),
  })
export const fetchIssue = (id: string) =>
  request<IssueDetail>(`/api/issues/${encodeURIComponent(id)}`)
const pageQuery = (before?: string, limit = 50) => {
  const query = new URLSearchParams({ limit: String(limit) })
  if (before) query.set("before", before)
  return query.toString()
}
export const fetchIssueComments = (id: string, before?: string, limit = 50) =>
  request<CursorPage<IssueComment>>(
    `/api/issues/${encodeURIComponent(id)}/comments?${pageQuery(before, limit)}`
  )
export const fetchIssueEvents = (id: string, before?: string, limit = 50) =>
  request<CursorPage<ExecutionEvent>>(
    `/api/issues/${encodeURIComponent(id)}/events?${pageQuery(before, limit)}`
  )
export const fetchIssueExecutions = (id: string, before?: string, limit = 50) =>
  request<CursorPage<Execution>>(
    `/api/issues/${encodeURIComponent(id)}/executions?${pageQuery(before, limit)}`
  )
export const fetchExecutionEvent = (id: string) =>
  request<ExecutionEvent>(`/api/execution-events/${encodeURIComponent(id)}`)
export const createIssue = (input: CreateIssueInput) =>
  request<Issue>("/api/issues", { method: "POST", body: JSON.stringify(input) })
export const updateIssue = (id: string, input: Partial<Issue>) =>
  request<Issue>(`/api/issues/${encodeURIComponent(id)}`, {
    method: "PATCH",
    body: JSON.stringify(input),
  })
export const dispatchIssue = (id: string) =>
  request<{ accepted: boolean }>(
    `/api/issues/${encodeURIComponent(id)}/dispatch`,
    { method: "POST" }
  )
export const cancelTask = (id: string, reason = "") =>
  request<TaskCancellationResult>(
    `/api/tasks/${encodeURIComponent(id)}/cancel`,
    { method: "POST", body: JSON.stringify({ reason }) }
  )
export const abandonIssue = (id: string, reason = "") =>
  request<Issue>(`/api/issues/${encodeURIComponent(id)}/abandon`, {
    method: "POST",
    body: JSON.stringify({ reason }),
  })
export const setIssueValidationDisabled = (id: string, disabled: boolean) =>
  request<Issue>(`/api/issues/${encodeURIComponent(id)}/validation`, {
    method: "PUT",
    body: JSON.stringify({ disabled }),
  })
export const fetchTaskTimeline = (id: string) =>
  request<TaskTimeline>(`/api/tasks/${encodeURIComponent(id)}/timeline`)
export const createIssueComment = (id: string, body: string) =>
  request<IssueComment>(`/api/issues/${encodeURIComponent(id)}/comments`, {
    method: "POST",
    body: JSON.stringify({ body }),
  })
export const sendChat = (id: string, message: string, executionId?: string) =>
  request<Message>(`/api/issues/${encodeURIComponent(id)}/chat`, {
    method: "POST",
    body: JSON.stringify({ message, executionId }),
  })
export const resolveApproval = (id: string, approved: boolean) =>
  request<Approval>(`/api/approvals/${encodeURIComponent(id)}`, {
    method: "POST",
    body: JSON.stringify({ approved }),
  })
export const stopExecution = (id: string) =>
  request<void>(`/api/executions/${encodeURIComponent(id)}/stop`, {
    method: "POST",
  })
export const interruptCurrentTool = (id: string) =>
  request<ToolInterruptResult>(
    `/api/executions/${encodeURIComponent(id)}/interrupt-tool`,
    { method: "POST" }
  )
export const fetchSessionDetail = (id: string) =>
  request<SessionDetail>(`/api/sessions/${encodeURIComponent(id)}`)
export const fetchSessionMessages = (id: string, before?: string, limit = 50) =>
  request<CursorPage<Message>>(
    `/api/sessions/${encodeURIComponent(id)}/messages?${pageQuery(before, limit)}`
  )
export const fetchSessionEvents = (id: string, before?: string, limit = 50) =>
  request<CursorPage<ExecutionEvent>>(
    `/api/sessions/${encodeURIComponent(id)}/events?${pageQuery(before, limit)}`
  )
export const fetchSessionProgress = (id: string, before?: string, limit = 50) =>
  request<CursorPage<ExecutionProgress>>(
    `/api/sessions/${encodeURIComponent(id)}/progress?${pageQuery(before, limit)}`
  )
export const fetchSessionDelta = (id: string, since: string) =>
  request<SessionDelta>(
    `/api/sessions/${encodeURIComponent(id)}/delta?since=${encodeURIComponent(since)}`
  )
export const createAgent = (input: SaveAgentInput) =>
  request<AgentDefinition>("/api/agents", {
    method: "POST",
    body: JSON.stringify(input),
  })
export const updateAgent = (id: string, input: SaveAgentInput) =>
  request<AgentDefinition>(`/api/agents/${encodeURIComponent(id)}`, {
    method: "PUT",
    body: JSON.stringify(input),
  })
export const deleteAgent = (id: string) =>
  request<void>(`/api/agents/${encodeURIComponent(id)}`, { method: "DELETE" })
export const fetchKnowledgeBase = (id: string) =>
  request<KnowledgeBaseDetail>(`/api/knowledge-bases/${encodeURIComponent(id)}`)
export const createKnowledgeBase = (input: SaveKnowledgeBaseInput) =>
  request<KnowledgeBase>("/api/knowledge-bases", {
    method: "POST",
    body: JSON.stringify(input),
  })
export const updateKnowledgeBase = (
  id: string,
  input: SaveKnowledgeBaseInput
) =>
  request<KnowledgeBase>(`/api/knowledge-bases/${encodeURIComponent(id)}`, {
    method: "PUT",
    body: JSON.stringify(input),
  })
export const deleteKnowledgeBase = (id: string) =>
  request<void>(`/api/knowledge-bases/${encodeURIComponent(id)}`, {
    method: "DELETE",
  })
export const createKnowledgeDocument = (
  knowledgeBaseId: string,
  input: SaveKnowledgeDocumentInput
) =>
  request<KnowledgeDocument>(
    `/api/knowledge-bases/${encodeURIComponent(knowledgeBaseId)}/documents`,
    { method: "POST", body: JSON.stringify(input) }
  )
export const updateKnowledgeDocument = (
  id: string,
  input: SaveKnowledgeDocumentInput
) =>
  request<KnowledgeDocument>(
    `/api/knowledge-documents/${encodeURIComponent(id)}`,
    { method: "PUT", body: JSON.stringify(input) }
  )
export const deleteKnowledgeDocument = (id: string) =>
  request<void>(`/api/knowledge-documents/${encodeURIComponent(id)}`, {
    method: "DELETE",
  })
export const createSkill = (input: SaveSkillInput) =>
  request<SkillDefinition>("/api/skills", {
    method: "POST",
    body: JSON.stringify(input),
  })
export const updateSkill = (id: string, input: SaveSkillInput) =>
  request<SkillDefinition>(`/api/skills/${encodeURIComponent(id)}`, {
    method: "PUT",
    body: JSON.stringify(input),
  })
export const deleteSkill = (id: string) =>
  request<void>(`/api/skills/${encodeURIComponent(id)}`, { method: "DELETE" })
export async function importSkill(file: File) {
  const data = new FormData()
  data.append("file", file)
  const response = await fetch("/api/skills/import", {
    method: "POST",
    body: data,
  })
  if (!response.ok)
    throw new Error(
      ((await response.json().catch(() => null)) as { error?: string } | null)
        ?.error ?? "导入失败"
    )
  return response.json() as Promise<SkillDefinition>
}
export const installSkill = (source: string) =>
  request<SkillDefinition>("/api/skills/install", {
    method: "POST",
    body: JSON.stringify({ source }),
  })
export const fetchFindings = (params?: {
  category?: string
  severity?: string
  page?: number
  pageSize?: number
}) => {
  const query = new URLSearchParams()
  if (params?.category) query.set("category", params.category)
  if (params?.severity) query.set("severity", params.severity)
  if (params?.page) query.set("page", String(params.page))
  if (params?.pageSize) query.set("pageSize", String(params.pageSize))
  const qs = query.toString()
  return request<FindingList>("/api/findings" + (qs ? `?${qs}` : ""))
}
export const updateFinding = (id: string, input: Partial<Finding>) =>
  request<Finding>(`/api/findings/${encodeURIComponent(id)}`, {
    method: "PATCH",
    body: JSON.stringify(input),
  })
export const fetchUncoverStatus = () =>
  request<UncoverStatus>("/api/tools/uncover/status")
export const saveUncoverProvider = (
  engine: string,
  input: SaveUncoverProviderInput
) =>
  request<UncoverEngine>(
    `/api/tools/uncover/providers/${encodeURIComponent(engine)}`,
    { method: "PUT", body: JSON.stringify(input) }
  )
export const deleteUncoverProvider = (engine: string) =>
  request<void>(`/api/tools/uncover/providers/${encodeURIComponent(engine)}`, {
    method: "DELETE",
  })
export const searchUncover = (input: UncoverSearchInput) =>
  request<UncoverSearchResult>("/api/tools/uncover/search", {
    method: "POST",
    body: JSON.stringify(input),
  })
export async function exportSkill(id: string) {
  const response = await fetch(`/api/skills/${encodeURIComponent(id)}/export`)
  if (!response.ok) throw new Error("导出失败")
  return response.blob()
}
export function subscribeToState(
  onState: (s: AppState) => void,
  onError: () => void
) {
  const events = new EventSource("/api/events")
  events.addEventListener("state", (e) =>
    onState(JSON.parse((e as MessageEvent<string>).data) as AppState)
  )
  events.onerror = onError
  return () => events.close()
}
