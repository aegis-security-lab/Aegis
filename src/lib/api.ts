import type {
  AgentDefinition,
  AppState,
  Approval,
  ConnectionTestResult,
  CreateIssueInput,
  Finding,
  FindingList,
  Issue,
  IssueComment,
  IssueDetail,
  Message,
  RuntimeProbe,
  SaveConfigInput,
  SessionDetail,
  SkillDefinition,
  TaskCancellationResult,
} from "@/types"
export type SaveAgentInput = Omit<
  AgentDefinition,
  "builtin" | "createdAt" | "updatedAt"
>
export type SaveSkillInput = Omit<
  SkillDefinition,
  "builtin" | "createdAt" | "updatedAt"
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
export const fetchSessionDetail = (id: string) =>
  request<SessionDetail>(`/api/sessions/${encodeURIComponent(id)}`)
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
