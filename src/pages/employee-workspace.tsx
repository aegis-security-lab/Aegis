import * as React from "react"
import { BriefcaseBusiness, Inbox, Plus, Send, UserRound } from "lucide-react"
import { Link, useNavigate, useParams, useSearchParams } from "react-router-dom"
import { toast } from "sonner"

import { EmployeeActivityConversation } from "@/components/employee-activity-conversation"
import { InputAttachmentList } from "@/components/input-attachments"
import { useInputAttachments } from "@/hooks/use-input-attachments"
import { isSendMessageKey } from "@/lib/keyboard"
import { MarkdownContent } from "@/components/markdown-content"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card"
import {
  Empty,
  EmptyDescription,
  EmptyHeader,
  EmptyMedia,
  EmptyTitle,
} from "@/components/ui/empty"
import {
  InputGroup,
  InputGroupAddon,
  InputGroupButton,
  InputGroupTextarea,
} from "@/components/ui/input-group"
import { ScrollArea } from "@/components/ui/scroll-area"
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import { Spinner } from "@/components/ui/spinner"
import {
  fetchEmployeeWorkspace,
  fetchRelayConversation,
  sendEmployeeMessage,
  sendRelayMessage,
} from "@/lib/api"
import { formatTime } from "@/lib/format"
import { useAppState } from "@/lib/state"
import type { EmployeeWorkspace, RelayConversation } from "@/types"

type Panel = "profile" | "issues" | "relay"

const panelLabels: Record<Panel, string> = {
  profile: "员工详情",
  issues: "负责的 Issues",
  relay: "Relay 聊天",
}

export function EmployeeWorkspacePage() {
  const { agentId } = useParams()
  const navigate = useNavigate()
  const [searchParams] = useSearchParams()
  const taskId = searchParams.get("taskId") ?? ""
  const { state } = useAppState()
  const [workspace, setWorkspace] = React.useState<EmployeeWorkspace | null>(
    null
  )
  const [panel, setPanel] = React.useState<Panel>(() =>
    searchParams.get("panel") === "relay" ? "relay" : "profile"
  )
  const [draft, setDraft] = React.useState("")
  const [sending, setSending] = React.useState(false)
  const fileInputRef = React.useRef<HTMLInputElement>(null)
  const attachments = useInputAttachments("employee", agentId ?? "")
  const [threadId, setThreadId] = React.useState("")
  const [conversation, setConversation] =
    React.useState<RelayConversation | null>(null)

  const employees = React.useMemo(
    () =>
      (state?.agents ?? []).filter(
        (agent) =>
          agent.enabled && !agent.internal && agent.category !== "concierge"
      ),
    [state?.agents]
  )
  React.useEffect(() => {
    if (!agentId && employees[0])
      navigate(`/employees/${employees[0].id}`, { replace: true })
  }, [agentId, employees, navigate])

  const load = React.useCallback(
    () =>
      agentId
        ? fetchEmployeeWorkspace(agentId, taskId || "general")
        : Promise.resolve(null),
    [agentId, taskId]
  )

  React.useEffect(() => {
    let active = true
    void load()
      .then((next) => {
        if (active && next) setWorkspace(next)
      })
      .catch((error) =>
        toast.error(
          error instanceof Error ? error.message : "读取员工工作台失败"
        )
      )
    return () => {
      active = false
    }
  }, [load])

  React.useEffect(() => {
    if (!agentId || workspace?.agent.id !== agentId) return
    let active = true
    let timer: number | undefined
    let latest = workspace
    let reportedError = false
    const refresh = async () => {
      let delay = latest.executions.some(isExecutionActive) ? 1000 : 4000
      try {
        const next = await fetchEmployeeWorkspace(agentId, taskId || "general")
        if (!active) return
        latest = next
        setWorkspace(next)
        reportedError = false
        delay = latest.executions.some(isExecutionActive) ? 1000 : 4000
      } catch (error) {
        if (!reportedError) {
          reportedError = true
          toast.error(
            error instanceof Error ? error.message : "刷新员工动态失败"
          )
        }
      } finally {
        if (active) timer = window.setTimeout(() => void refresh(), delay)
      }
    }
    timer = window.setTimeout(() => void refresh(), 1000)
    return () => {
      active = false
      if (timer) window.clearTimeout(timer)
    }
  }, [agentId, taskId, workspace?.agent.id])

  React.useEffect(() => {
    if (!agentId || !threadId) return
    let active = true
    void fetchRelayConversation(agentId, threadId)
      .then((next) => {
        if (active) setConversation(next)
      })
      .catch((error) =>
        toast.error(
          error instanceof Error ? error.message : "读取 Relay 会话失败"
        )
      )
    return () => {
      active = false
    }
  }, [agentId, threadId, state?.updatedAt])

  const send = async () => {
    const message = draft.trim()
    if (
      !agentId ||
      (!message && attachments.attachmentIds.length === 0) ||
      sending ||
      attachments.uploading ||
      attachments.hasErrors
    )
      return
    setSending(true)
    setDraft("")
    try {
      await sendEmployeeMessage(
        agentId,
        message,
        attachments.attachmentIds,
        taskId || "general"
      )
      attachments.clearBound()
      const next = await load()
      if (next) setWorkspace(next)
    } catch (error) {
      setDraft(message)
      toast.error(error instanceof Error ? error.message : "发送失败")
    } finally {
      setSending(false)
    }
  }

  if (!workspace || workspace.agent.id !== agentId)
    return (
      <div className="flex size-full items-center justify-center">
        <Spinner />
      </div>
    )
  const position = state?.positions.find(
    (item) => item.id === workspace.agent.positionId
  )
  const department = state?.departments.find(
    (item) => item.id === workspace.agent.departmentId
  )
  const manager = employees.find(
    (item) => item.id === workspace.agent.managerAgentId
  )
  const directReports = employees.filter(
    (item) => item.managerAgentId === workspace.agent.id
  )
  const employeeAvailability = state?.employeeAvailability.find(
    (item) => item.agentId === workspace.agent.id
  )
  const isWorking = employeeAvailability?.available === false
  const taskOptions = (() => {
    const issueMap = new Map(
      (state?.issues ?? []).map((issue) => [issue.id, issue])
    )
    const roots = new Map<string, (typeof workspace.issues)[number]>()
    for (const assigned of workspace.issues) {
      let current = issueMap.get(assigned.id) ?? assigned
      const seen = new Set<string>()
      while (current.parentId && !seen.has(current.id)) {
        seen.add(current.id)
        const parent = issueMap.get(current.parentId)
        if (!parent) break
        current = parent
      }
      roots.set(current.id, current)
    }
    return Array.from(roots.values()).sort((a, b) =>
      b.updatedAt.localeCompare(a.updatedAt)
    )
  })()

  return (
    <div className="grid size-full min-h-0 grid-cols-[minmax(0,1fr)_360px] overflow-hidden">
      <section className="flex min-h-0 min-w-0 flex-col">
        <header className="flex h-14 shrink-0 items-center gap-3 border-b px-5">
          <div className="flex size-8 items-center justify-center rounded-full bg-muted">
            <UserRound />
          </div>
          <div className="min-w-0 flex-1">
            <p className="truncate text-sm font-semibold">
              {workspace.agent.name}
            </p>
            <p className="truncate text-xs text-muted-foreground">
              {workspace.agent.englishName} · {position?.name ?? "未分配岗位"}
            </p>
          </div>
          <Badge variant={isWorking ? "secondary" : "outline"}>
            {isWorking ? (
              <span className="size-2 rounded-full bg-emerald-500" />
            ) : null}
            {isWorking ? "工作中" : workspace.agent.enabled ? "空闲" : "停用"}
          </Badge>
        </header>
        <div className="min-h-0 flex-1">
          <EmployeeActivityConversation
            executions={workspace.executions}
            messages={workspace.messages}
            events={workspace.events ?? []}
            attachments={workspace.attachments ?? []}
            issues={workspace.issues}
          />
        </div>
        <div className="shrink-0 px-5 pt-2 pb-4">
          <InputAttachmentList
            items={attachments.items}
            onRemove={(clientId) => void attachments.remove(clientId)}
            className="mb-2"
          />
          <input
            ref={fileInputRef}
            type="file"
            multiple
            className="hidden"
            onChange={(event) => {
              if (event.target.files) attachments.addFiles(event.target.files)
              event.target.value = ""
            }}
          />
          <InputGroup>
            <InputGroupTextarea
              value={draft}
              onChange={(event) => setDraft(event.target.value)}
              onKeyDown={(event) => {
                if (isSendMessageKey(event)) {
                  event.preventDefault()
                  void send()
                }
              }}
              placeholder={`直接和 ${workspace.agent.name} 对话…`}
              className="min-h-20 resize-none"
            />
            <InputGroupAddon align="block-end" className="justify-between">
              <InputGroupButton
                size="icon-sm"
                disabled={sending}
                onClick={() => fileInputRef.current?.click()}
                aria-label="添加附件"
              >
                <Plus />
              </InputGroupButton>
              <InputGroupButton
                variant="default"
                size="icon-sm"
                disabled={
                  sending ||
                  attachments.uploading ||
                  attachments.hasErrors ||
                  (!draft.trim() && attachments.attachmentIds.length === 0)
                }
                onClick={() => void send()}
                aria-label="发送消息"
              >
                {sending ? <Spinner /> : <Send />}
              </InputGroupButton>
            </InputGroupAddon>
          </InputGroup>
        </div>
      </section>
      <aside className="flex min-h-0 flex-col bg-muted/15">
        <header className="flex shrink-0 flex-col gap-2 border-b p-3">
          <Select
            value={taskId || "general"}
            onValueChange={(value) => {
              if (!value) return
              const next = new URLSearchParams(searchParams)
              if (value === "general") next.delete("taskId")
              else next.set("taskId", value)
              navigate(`/employees/${workspace.agent.id}?${next.toString()}`)
            }}
          >
            <SelectTrigger className="w-full">
              <SelectValue placeholder="切换任务">
                {(value) => {
                  if (value === "general") return "通用会话"
                  const issue = state?.issues.find((item) => item.id === value)
                  return issue
                    ? `${issue.identifier} · ${issue.title}`
                    : "任务会话"
                }}
              </SelectValue>
            </SelectTrigger>
            <SelectContent>
              <SelectGroup>
                <SelectItem value="general">通用会话</SelectItem>
                {taskOptions.map((issue) => (
                  <SelectItem key={issue.id} value={issue.id}>
                    {issue.identifier} · {issue.title}
                  </SelectItem>
                ))}
              </SelectGroup>
            </SelectContent>
          </Select>
          <Select
            value={panel}
            onValueChange={(value) => setPanel(value as Panel)}
          >
            <SelectTrigger className="w-full">
              <SelectValue>
                {(value) => panelLabels[value as Panel] ?? "选择面板"}
              </SelectValue>
            </SelectTrigger>
            <SelectContent>
              <SelectGroup>
                <SelectItem value="profile">员工详情</SelectItem>
                <SelectItem value="issues">负责的 Issues</SelectItem>
                <SelectItem value="relay">Relay 聊天</SelectItem>
              </SelectGroup>
            </SelectContent>
          </Select>
        </header>
        <ScrollArea className="min-h-0 flex-1">
          <div className="p-4">
            {panel === "profile" ? (
              <Profile
                workspace={workspace}
                positionName={position?.name}
                departmentName={department?.name}
                managerName={manager?.name}
                directReports={directReports}
              />
            ) : null}
            {panel === "issues" ? <Issues workspace={workspace} /> : null}
            {panel === "relay" ? (
              <RelayPanel
                workspace={workspace}
                threadId={threadId}
                onThread={setThreadId}
                conversation={conversation}
                onSent={async (recipient, body, issueId) => {
                  await sendRelayMessage(
                    workspace.agent.id,
                    recipient,
                    body,
                    issueId
                  )
                  await load()
                }}
              />
            ) : null}
          </div>
        </ScrollArea>
      </aside>
    </div>
  )
}

function isExecutionActive(execution: EmployeeWorkspace["executions"][number]) {
  return ["queued", "starting", "running", "waiting_approval"].includes(
    execution.status
  )
}

function Profile({
  workspace,
  positionName,
  departmentName,
  managerName,
  directReports,
}: {
  workspace: EmployeeWorkspace
  positionName?: string
  departmentName?: string
  managerName?: string
  directReports: EmployeeWorkspace["agent"][]
}) {
  return (
    <div className="flex flex-col gap-4">
      <Card>
        <CardHeader>
          <CardTitle>{workspace.agent.name}</CardTitle>
          <CardDescription>
            {workspace.agent.englishName} · {positionName ?? "未分配岗位"}
          </CardDescription>
        </CardHeader>
        <CardContent className="flex flex-col gap-3 text-sm">
          <Info label="岗位" value={positionName ?? "未分配岗位"} />
          <Info label="所属部门" value={departmentName ?? "未分组"} />
          <Info label="直属上级" value={managerName ?? "无"} />
          <Info
            label="直属下属"
            value={
              directReports.length
                ? directReports.map((item) => item.name).join("、")
                : "无"
            }
          />
          <Info
            label="模型"
            value={`${workspace.agent.model.provider} / ${workspace.agent.model.model}`}
          />
          <Info label="长期会话" value={workspace.session.sessionId} />
        </CardContent>
      </Card>
      <Card>
        <CardHeader>
          <CardTitle>工作概览</CardTitle>
        </CardHeader>
        <CardContent className="flex flex-col gap-3 text-sm">
          <Info label="负责 Issues" value={String(workspace.issues.length)} />
          <Info label="执行轮次" value={String(workspace.executions.length)} />
          <Info
            label="Relay 未读"
            value={String(
              workspace.relay.reduce((sum, item) => sum + item.unreadCount, 0)
            )}
          />
        </CardContent>
      </Card>
    </div>
  )
}
function Info({ label, value }: { label: string; value: string }) {
  return (
    <div className="flex items-start justify-between gap-4">
      <span className="text-muted-foreground">{label}</span>
      <span className="text-right font-medium break-all">{value}</span>
    </div>
  )
}
function Issues({ workspace }: { workspace: EmployeeWorkspace }) {
  return workspace.issues.length ? (
    <div className="flex flex-col gap-2">
      {workspace.issues.map((issue) => (
        <Link
          key={issue.id}
          to={`/issues/${issue.id}`}
          className="rounded-lg border bg-card p-3 transition-colors hover:bg-muted/40"
        >
          <div className="flex items-center justify-between gap-2">
            <span className="font-mono text-xs text-muted-foreground">
              {issue.identifier}
            </span>
            <Badge variant="outline">{issue.status}</Badge>
          </div>
          <p className="mt-2 text-sm font-medium">{issue.title}</p>
        </Link>
      ))}
    </div>
  ) : (
    <Empty>
      <EmptyHeader>
        <EmptyMedia variant="icon">
          <BriefcaseBusiness />
        </EmptyMedia>
        <EmptyTitle>暂无 Issue</EmptyTitle>
        <EmptyDescription>Board 的委派会显示在这里。</EmptyDescription>
      </EmptyHeader>
    </Empty>
  )
}

function RelayPanel({
  workspace,
  threadId,
  onThread,
  conversation,
  onSent,
}: {
  workspace: EmployeeWorkspace
  threadId: string
  onThread: (id: string) => void
  conversation: RelayConversation | null
  onSent: (recipient: string, body: string, issueId: string) => Promise<void>
}) {
  const [body, setBody] = React.useState("")
  const selected = workspace.relay.find((item) => item.thread.id === threadId)
  const recipient =
    selected?.thread.participantIds.find(
      (id) => id !== workspace.agent.id && id !== "board" && id !== "operator"
    ) ?? ""
  return (
    <div className="flex flex-col gap-4">
      {workspace.relay.length ? (
        <Select
          value={threadId}
          onValueChange={(value) => value && onThread(value)}
        >
          <SelectTrigger className="w-full">
            <SelectValue placeholder="选择 Relay 会话">
              {(value) => {
                const item = workspace.relay.find(
                  (candidate) => candidate.thread.id === value
                )
                return item
                  ? item.lastMessage?.senderId || item.thread.title
                  : "选择 Relay 会话"
              }}
            </SelectValue>
          </SelectTrigger>
          <SelectContent>
            <SelectGroup>
              {workspace.relay.map((item) => (
                <SelectItem key={item.thread.id} value={item.thread.id}>
                  {item.lastMessage?.senderId || item.thread.title}
                  {item.unreadCount ? ` · ${item.unreadCount} 未读` : ""}
                </SelectItem>
              ))}
            </SelectGroup>
          </SelectContent>
        </Select>
      ) : null}
      {conversation ? (
        <>
          <div className="flex flex-col gap-3">
            {conversation.messages.map((message) => (
              <Card key={message.id}>
                <CardHeader className="pb-2">
                  <CardTitle className="text-sm">{message.senderId}</CardTitle>
                  <CardDescription>
                    {formatTime(message.createdAt)}
                  </CardDescription>
                </CardHeader>
                <CardContent>
                  <MarkdownContent className="text-[13px] !leading-5">
                    {message.body}
                  </MarkdownContent>
                  {message.issueId ? (
                    <Button
                      variant="link"
                      className="mt-2 px-0"
                      render={<Link to={`/issues/${message.issueId}`} />}
                    >
                      打开关联 Issue
                    </Button>
                  ) : null}
                </CardContent>
              </Card>
            ))}
          </div>
          {recipient ? (
            <InputGroup>
              <InputGroupTextarea
                value={body}
                onChange={(event) => setBody(event.target.value)}
                placeholder={`回复 ${recipient}…`}
              />
              <InputGroupAddon align="block-end" className="justify-end">
                <InputGroupButton
                  variant="default"
                  disabled={!body.trim()}
                  onClick={() => {
                    const value = body.trim()
                    setBody("")
                    void onSent(recipient, value, "").catch(() =>
                      setBody(value)
                    )
                  }}
                >
                  <Send />
                  发送
                </InputGroupButton>
              </InputGroupAddon>
            </InputGroup>
          ) : null}
        </>
      ) : (
        <Empty>
          <EmptyHeader>
            <EmptyMedia variant="icon">
              <Inbox />
            </EmptyMedia>
            <EmptyTitle>Relay</EmptyTitle>
            <EmptyDescription>
              选择会话查看员工之间的异步消息。
            </EmptyDescription>
          </EmptyHeader>
        </Empty>
      )}
    </div>
  )
}
