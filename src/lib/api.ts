import type {
  AgentDefinition,
  AppState,
  Approval,
  ConciergeConversation,
  ConciergeConversationDetail,
  ConnectionTestResult,
  ContainerProfile,
  ContainerInstance,
  ContainerBatchDeleteImpact,
  ContainerBatchDeleteResult,
  ContainerBatchStopResult,
  ContainerDeleteImpact,
  ContainerDeleteResult,
  ContainerProfileDeleteImpact,
  ContainerProfileDeleteResult,
  CapabilityDescriptor,
  CoordinationBinding,
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
  InputAttachment,
  KnowledgeBase,
  KnowledgeBaseDetail,
  KnowledgeDocument,
  Message,
  SaveConfigInput,
  SaveUncoverProviderInput,
  SessionDetail,
  SessionDelta,
  SkillDefinition,
  TaskCancellationResult,
  Task,
  TaskAudit,
  ToolInterruptResult,
  TaskTimeline,
  TaskWorkspace,
  TaskPhone,
  TaskDetail,
  UncoverEngine,
  UncoverSearchInput,
  UncoverSearchResult,
  UncoverStatus,
  WebSearchConfig,
  WebSearchInput,
  WebSearchResult,
} from "@/types"
export type SaveContainerProfileInput = Omit<
  ContainerProfile,
  "id" | "createdAt" | "updatedAt"
>
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
  const headers = new Headers(init?.headers)
  if (!(init?.body instanceof FormData) && !headers.has("Content-Type")) {
    headers.set("Content-Type", "application/json")
  }
  const response = await fetch(path, {
    ...init,
    headers,
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
export const probeDocker = () =>
  request<{ ready: boolean }>("/api/container-profiles/probe/docker")
export const buildWorkerContainerImage = () =>
  request<{ image: string; output: string }>(
    "/api/container-profiles/image/build",
    { method: "POST" }
  )
export const createContainerProfile = (input: SaveContainerProfileInput) =>
  request<ContainerProfile>("/api/container-profiles", {
    method: "POST",
    body: JSON.stringify(input),
  })
export const updateContainerProfile = (
  id: string,
  input: SaveContainerProfileInput
) =>
  request<ContainerProfile>(
    `/api/container-profiles/${encodeURIComponent(id)}`,
    { method: "PUT", body: JSON.stringify(input) }
  )
export const fetchContainerProfileDeleteImpact = (id: string) =>
  request<ContainerProfileDeleteImpact>(
    `/api/container-profiles/${encodeURIComponent(id)}/delete-impact`
  )
export const deleteContainerProfile = (id: string, cascadeIssues: boolean) =>
  request<ContainerProfileDeleteResult>(
    `/api/container-profiles/${encodeURIComponent(id)}?cascadeIssues=${cascadeIssues}`,
    { method: "DELETE" }
  )
export const fetchContainerDeleteImpact = (id: string) =>
  request<ContainerDeleteImpact>(
    `/api/containers/${encodeURIComponent(id)}/delete-impact`
  )
export const deleteContainer = (id: string, cascadeIssues: boolean) =>
  request<ContainerDeleteResult>(
    `/api/containers/${encodeURIComponent(id)}?cascadeIssues=${cascadeIssues}`,
    { method: "DELETE" }
  )
export const startContainer = (id: string) =>
  request<ContainerInstance>(
    `/api/containers/${encodeURIComponent(id)}/start`,
    { method: "POST" }
  )
export const stopContainer = (id: string) =>
  request<ContainerInstance>(`/api/containers/${encodeURIComponent(id)}/stop`, {
    method: "POST",
  })
export const stopContainers = (containerIds: string[]) =>
  request<ContainerBatchStopResult>("/api/containers/batch/stop", {
    method: "POST",
    body: JSON.stringify({ containerIds }),
  })
export const fetchContainerBatchDeleteImpact = (containerIds: string[]) =>
  request<ContainerBatchDeleteImpact>("/api/containers/batch/delete-impact", {
    method: "POST",
    body: JSON.stringify({ containerIds }),
  })
export const deleteContainers = (
  containerIds: string[],
  cascadeIssues: boolean
) =>
  request<ContainerBatchDeleteResult>("/api/containers/batch/delete", {
    method: "POST",
    body: JSON.stringify({ containerIds, cascadeIssues }),
  })
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
export const sendConciergeMessage = (
  id: string,
  message: string,
  attachmentIds: string[] = []
) =>
  request<Message>(
    `/api/concierge/conversations/${encodeURIComponent(id)}/messages`,
    { method: "POST", body: JSON.stringify({ message, attachmentIds }) }
  )
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
export const testWebSearch = (
  config: WebSearchConfig,
  search: WebSearchInput
) =>
  request<WebSearchResult>("/api/settings/web-search/test", {
    method: "POST",
    body: JSON.stringify({ config, search }),
  })
export const saveWebSearch = (config: WebSearchConfig) =>
  request<AppState>("/api/settings/web-search", {
    method: "PUT",
    body: JSON.stringify(config),
  })
export const fetchIssue = (id: string) =>
  request<IssueDetail>(`/api/issues/${encodeURIComponent(id)}`)
function uploadInputAttachment(
  path: string,
  file: File,
  onProgress?: (progress: number) => void
) {
  return new Promise<InputAttachment>((resolve, reject) => {
    const body = new FormData()
    body.append("file", file, file.name)
    const request = new XMLHttpRequest()
    request.open("POST", path)
    request.responseType = "json"
    request.upload.addEventListener("progress", (event) => {
      if (event.lengthComputable) {
        onProgress?.(
          Math.min(100, Math.round((event.loaded / event.total) * 100))
        )
      }
    })
    request.addEventListener("load", () => {
      const response = request.response as
        InputAttachment | { error?: string } | null
      if (request.status >= 200 && request.status < 300 && response) {
        onProgress?.(100)
        resolve(response as InputAttachment)
        return
      }
      reject(
        new Error(
          (response as { error?: string } | null)?.error ??
            `附件上传失败 (${request.status})`
        )
      )
    })
    request.addEventListener("error", () =>
      reject(new Error("附件上传连接失败"))
    )
    request.addEventListener("abort", () => reject(new Error("附件上传已取消")))
    request.send(body)
  })
}

export const uploadTaskAttachment = (
  file: File,
  onProgress?: (progress: number) => void
) => uploadInputAttachment("/api/tasks/attachments", file, onProgress)

export const uploadConciergeAttachment = (
  conversationId: string,
  file: File,
  onProgress?: (progress: number) => void
) =>
  uploadInputAttachment(
    `/api/concierge/conversations/${encodeURIComponent(conversationId)}/attachments`,
    file,
    onProgress
  )

export const deleteInputAttachment = (id: string) =>
  request<void>(`/api/input-attachments/${encodeURIComponent(id)}`, {
    method: "DELETE",
  })
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
export const createTask = (input: CreateIssueInput) =>
  request<{ task: Task; issue: Issue }>("/api/tasks", {
    method: "POST",
    body: JSON.stringify(input),
  })
export const fetchTask = (id: string) =>
  request<TaskDetail>(`/api/tasks/${encodeURIComponent(id)}`)
export const updateIssue = (id: string, input: Partial<Issue>) =>
  request<Issue>(`/api/issues/${encodeURIComponent(id)}`, {
    method: "PATCH",
    body: JSON.stringify(input),
  })
export const deleteIssue = (id: string) =>
  request<{
    issueId: string
    deletedIssues: number
    deletedExecutions: number
    deletedTasks: number
  }>(`/api/issues/${encodeURIComponent(id)}`, { method: "DELETE" })
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
export const restartTask = (id: string) =>
  request<Issue>(`/api/tasks/${encodeURIComponent(id)}/restart`, {
    method: "POST",
  })
export const fetchTaskAudits = (id: string) =>
  request<{ audits: TaskAudit[] }>(
    `/api/tasks/${encodeURIComponent(id)}/audits`
  )
export const createTaskAudit = (id: string) =>
  request<TaskAudit>(`/api/tasks/${encodeURIComponent(id)}/audits`, {
    method: "POST",
  })
export const fetchTaskAudit = (id: string) =>
  request<TaskAudit>(`/api/task-audits/${encodeURIComponent(id)}`)
export const taskAuditReportURL = (id: string) =>
  `/api/task-audits/${encodeURIComponent(id)}/report`
export const updateTaskBudget = (id: string, timeBudgetMinutes: number) =>
  request<Task>(`/api/tasks/${encodeURIComponent(id)}/budget`, {
    method: "PATCH",
    body: JSON.stringify({ timeBudgetMinutes }),
  })
export const fetchTaskWorkspace = (id: string) =>
  request<TaskWorkspace>(`/api/tasks/${encodeURIComponent(id)}/workspace`)
export const fetchTaskPhones = (id: string) =>
  request<{ phones: TaskPhone[] }>(
    `/api/tasks/${encodeURIComponent(id)}/phones`
  )

export const fetchTaskAgents = (id: string) =>
  request<{ agents: import("@/types").TaskAgent[] }>(
    `/api/tasks/${encodeURIComponent(id)}/agents`
  )

export const fetchCapabilityCatalog = () =>
  request<{ enabled: boolean; capabilities: CapabilityDescriptor[] }>(
    "/api/coordination/capabilities"
  )
export const fetchCoordinationBinding = (issueId: string) =>
  request<CoordinationBinding>(
    `/api/issues/${encodeURIComponent(issueId)}/coordination`
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
export const manuallyRejectIssueValidation = (id: string, reason: string) =>
  request<Issue>(
    `/api/issues/${encodeURIComponent(id)}/validation/manual-reject`,
    { method: "POST", body: JSON.stringify({ reason }) }
  )
export const fetchTaskTimeline = (id: string) =>
  request<TaskTimeline>(`/api/tasks/${encodeURIComponent(id)}/timeline`)
export const createIssueComment = (id: string, body: string, objective = "") =>
  request<IssueComment>(`/api/issues/${encodeURIComponent(id)}/comments`, {
    method: "POST",
    body: JSON.stringify({ body, objective }),
  })
export const resolveApproval = (
  id: string,
  approved: boolean,
  reviewContent = ""
) =>
  request<Approval>(`/api/approvals/${encodeURIComponent(id)}`, {
    method: "POST",
    body: JSON.stringify({ approved, reviewContent }),
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
