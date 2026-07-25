import * as React from "react"
import { ChevronDown, ChevronRight, ClipboardList, Code2, GitBranch, LayoutTemplate, MessageSquareText, PanelRightClose, PanelRightOpen, Plus, Send, ShieldOff, TerminalSquare, XCircle } from "lucide-react"
import { Link } from "react-router-dom"
import { toast } from "sonner"

import { IssueCommentsList } from "@/components/issue-comments-list"
import { MarkdownContent } from "@/components/markdown-content"
import { Bubble, BubbleContent } from "@/components/ui/bubble"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from "@/components/ui/collapsible"
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from "@/components/ui/dialog"
import { InputGroup, InputGroupAddon, InputGroupButton, InputGroupTextarea } from "@/components/ui/input-group"
import { Message, MessageContent } from "@/components/ui/message"
import { MessageScroller, MessageScrollerButton, MessageScrollerContent, MessageScrollerItem, MessageScrollerProvider, MessageScrollerViewport } from "@/components/ui/message-scroller"
import { Select, SelectContent, SelectGroup, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { createIssueComment, fetchIssueAgentTimeline, sendChat, sendChatWithAttachments } from "@/lib/api"
import { cn } from "@/lib/utils"
import { StatusBadge } from "@/components/status-badge"
import type { AgentDefinition, AppState, Execution, ExecutionEvent, Issue, IssueAgentTimeline, IssueDetail, Message as ChatMessage } from "@/types"

type SideTab = "details" | "broadcasts" | "dependencies" | "abandoned"
const sideTabLabels: Record<SideTab, string> = { details: "Issue 详情", broadcasts: "广播", dependencies: "依赖关系", abandoned: "放弃的目标" }

export function IssueChatWorkspace({ detail, state, busy, onBusyChange, onReload, onCancel, onManualReject }: { detail: IssueDetail; state: AppState | null; busy: boolean; onBusyChange: (busy: boolean) => void; onReload: () => Promise<void>; onCancel: () => void; onManualReject: () => void }) {
  const issue = state?.issues.find((candidate) => candidate.id === detail.issue.id) ?? detail.issue
  const agents = state?.agents ?? []
  const associated = React.useMemo(() => Array.from(new Set([issue.assigneeAgentId, ...detail.agentSessions.map((item) => item.agentId)].filter(Boolean) as string[])), [detail.agentSessions, issue.assigneeAgentId])
  const [selectedAgentID, setAgentID] = React.useState(issue.assigneeAgentId || associated[0] || "")
  const [text, setText] = React.useState("")
  const [files, setFiles] = React.useState<File[]>([])
  const [sideOpen, setSideOpen] = React.useState(true)
  const [tab, setTab] = React.useState<SideTab>("details")
  const fileInput = React.useRef<HTMLInputElement>(null)
  const agentID = selectedAgentID || issue.assigneeAgentId || associated[0] || ""
  const [timeline, setTimeline] = React.useState<IssueAgentTimeline | null>(null)
  const [comment, setComment] = React.useState("")

  React.useEffect(() => {
    if (!agentID) return
    let cancelled = false
    let timer: number | undefined
    const refreshTimeline = async () => {
      let delay = 4000
      try {
        const next = await fetchIssueAgentTimeline(issue.id, agentID)
        if (!cancelled) setTimeline((current) => timelineSignature(current) === timelineSignature(next) ? current : next)
        if (next.executions.some((execution) => isExecutionActive(execution))) delay = 1000
      } catch {
        // Keep the last complete timeline during a transient polling failure.
      } finally {
        if (!cancelled) timer = window.setTimeout(() => void refreshTimeline(), delay)
      }
    }
    void refreshTimeline()
    return () => { cancelled = true; if (timer) window.clearTimeout(timer) }
  }, [agentID, issue.id])

  const executions = React.useMemo(() => timeline?.executions ?? [], [timeline])
  const messages = React.useMemo(() => timeline?.messages ?? [], [timeline])
  const events = React.useMemo(() => timeline?.events ?? [], [timeline])
  const active = executions.find((execution) => ["queued", "starting", "running", "waiting_approval"].includes(execution.status))
  const delivery = executions.find((execution) => execution.finalResultSubmitted && execution.finalResult)
  const finalResult = delivery?.finalResult || (agentID === issue.assigneeAgentId ? issue.result : "") || ""
  const turns = React.useMemo(() => buildChatTurns(executions, messages, events, finalResult), [events, executions, finalResult, messages])

  const submit = async (event: React.FormEvent) => {
    event.preventDefault()
    if (!text.trim() && files.length === 0) return
    onBusyChange(true)
    try {
      if (files.length) {
        if (!active) throw new Error("请先发送一条消息连接 Agent，再上传附件")
        await sendChatWithAttachments(issue.id, text.trim() || "请查看附件。", active.id, files)
      } else await sendChat(issue.id, text.trim(), active?.id, agentID)
      setText(""); setFiles([]); await onReload()
    } catch (error) { toast.error(error instanceof Error ? error.message : "发送失败") }
    finally { onBusyChange(false) }
  }

  const submitComment = async (event: React.FormEvent) => {
    event.preventDefault()
    if (!comment.trim()) return
    onBusyChange(true)
    try {
      const agent = agents.find((candidate) => candidate.id === agentID)
      const mention = agent && agent.id !== issue.assigneeAgentId ? `[@${agent.name}](agent://${agent.id})\n\n` : ""
      await createIssueComment(issue.id, mention + comment.trim())
      setComment("")
      await onReload()
    } catch (error) { toast.error(error instanceof Error ? error.message : "评论失败") }
    finally { onBusyChange(false) }
  }

  return (
    <div className={cn("grid size-full min-h-0 overflow-hidden bg-background", sideOpen ? "xl:grid-cols-[minmax(0,1fr)_380px]" : "grid-cols-1")}>
      <section className="flex min-h-0 min-w-0 flex-col">
        <header className="flex h-14 shrink-0 items-center gap-3 border-b px-4">
          <Select value={agentID} onValueChange={(value) => value && setAgentID(value)}>
            <SelectTrigger><SelectValue placeholder="选择关联 Agent" /></SelectTrigger>
            <SelectContent><SelectGroup>{associated.map((id) => <SelectItem key={id} value={id}>{agents.find((agent) => agent.id === id)?.name ?? id}</SelectItem>)}</SelectGroup></SelectContent>
          </Select>
          <h1 className="min-w-0 flex-1 truncate text-sm font-semibold sm:text-base">{issue.title}</h1>
          {!sideOpen ? <Button type="button" variant="ghost" size="icon-sm" onClick={() => setSideOpen(true)} aria-label="打开侧栏"><PanelRightOpen /></Button> : null}
        </header>
        <MessageScrollerProvider autoScroll>
          <MessageScroller className="min-h-0 flex-1">
            <MessageScrollerViewport className="px-5 py-4 sm:px-8">
              <MessageScrollerContent className="mx-auto w-full max-w-4xl gap-3">
                {turns.length === 0 ? <div className="m-auto text-sm text-muted-foreground">这个 Session 还没有消息</div> : null}
                {turns.map((turn) => <ChatTurnView key={turn.id} turn={turn} issue={issue} />)}
              </MessageScrollerContent>
            </MessageScrollerViewport>
            <MessageScrollerButton />
          </MessageScroller>
        </MessageScrollerProvider>
        <form onSubmit={submit} className="shrink-0 border-t p-3">
          {files.length ? <div className="mb-2 flex flex-wrap gap-2">{files.map((file, index) => <button type="button" key={`${file.name}-${index}`} className="rounded-md bg-muted px-2 py-1 text-xs" onClick={() => setFiles((current) => current.filter((_, item) => item !== index))}>{file.name} ×</button>)}</div> : null}
          <input ref={fileInput} type="file" multiple className="hidden" onChange={(event) => setFiles(Array.from(event.target.files ?? []))} />
          <InputGroup className="min-h-24 items-stretch rounded-2xl">
            <InputGroupTextarea
              value={text}
              onChange={(event) => setText(event.target.value)}
              onKeyDown={(event) => {
                if (event.key !== "Enter" || event.shiftKey || event.nativeEvent.isComposing) return
                event.preventDefault()
                event.currentTarget.form?.requestSubmit()
              }}
              placeholder="直接和 Agent 聊天（内容将原样发送，不会发布为 Issue 评论）…"
              className="min-h-14 resize-none text-[13px]"
            />
            <InputGroupAddon align="block-end" className="justify-between">
              <div className="flex items-center gap-1">
                <InputGroupButton size="icon-sm" onClick={() => fileInput.current?.click()} aria-label="添加附件"><Plus /></InputGroupButton>
                <span className="px-1 text-xs text-muted-foreground">直接聊天 · 不写入 Issue 评论</span>
                {!issue.parentId && !["done", "cancelled"].includes(issue.status) ? <InputGroupButton variant="ghost" onClick={onCancel}><XCircle data-icon="inline-start" />取消任务</InputGroupButton> : null}
                {issue.objective.trim() && ["done", "in_review"].includes(issue.status) && detail.validations.some((validation) => validation.status === "passed") ? <InputGroupButton variant="ghost" onClick={onManualReject}><ShieldOff data-icon="inline-start" />手动验收不通过</InputGroupButton> : null}
              </div>
              <InputGroupButton type="submit" size="icon-sm" variant="default" disabled={busy || (!text.trim() && files.length === 0)} aria-label="发送"><Send /></InputGroupButton>
            </InputGroupAddon>
          </InputGroup>
        </form>
      </section>
      {sideOpen ? <aside className="min-h-0 min-w-0 overflow-hidden bg-muted/10">
        <div className="flex size-full min-h-0 flex-col">
          <div className="flex h-14 shrink-0 items-center gap-2 border-b px-2">
            <Select value={tab} onValueChange={(value) => value && setTab(value as SideTab)}>
              <SelectTrigger className="min-w-0 flex-1"><SelectValue /></SelectTrigger>
              <SelectContent><SelectGroup>{(Object.keys(sideTabLabels) as SideTab[]).map((value) => <SelectItem key={value} value={value}>{sideTabLabels[value]}</SelectItem>)}</SelectGroup></SelectContent>
            </Select>
            <Button variant="ghost" size="icon-sm" onClick={() => setSideOpen(false)} aria-label="关闭侧栏"><PanelRightClose /></Button>
          </div>
          <div className="min-h-0 flex-1 overflow-y-auto p-5">
            {tab === "details" ? <IssueDetails issue={issue} detail={detail} agents={agents} comment={comment} busy={busy} onCommentChange={setComment} onComment={submitComment} /> : null}
            {tab === "broadcasts" ? <div className="flex flex-col gap-3">{detail.broadcasts.length ? detail.broadcasts.map((item) => <div key={item.id} className="rounded-md border p-3"><p className="font-medium">{item.subject}</p><MarkdownContent className="mt-2 text-sm">{item.message}</MarkdownContent></div>) : <p className="text-sm text-muted-foreground">还没有广播</p>}</div> : null}
            {tab === "dependencies" ? <Dependencies blockedBy={detail.blockedBy} blocks={detail.blocks} /> : null}
            {tab === "abandoned" ? <Abandoned issue={issue} issues={state?.issues ?? [issue]} /> : null}
          </div>
        </div>
      </aside> : null}
    </div>
  )
}

interface ChatTurn {
  id: string
  user?: ChatMessage
  intermediate: ChatMessage[]
  events: ExecutionEvent[]
  final?: ChatMessage
  finalContent: string
  execution?: Execution
  startedAt: string
  promptKind?: "task" | "comment"
}

function ChatTurnView({ turn, issue }: { turn: ChatTurn; issue: Issue }) {
  return <MessageScrollerItem messageId={turn.id} scrollAnchor={Boolean(turn.user)}>
    <div className="flex flex-col gap-3">
      {turn.user ? turn.promptKind ? <PromptCard turn={turn} issue={issue} /> : <Message align="end"><MessageContent><Bubble variant="secondary" align="end"><BubbleContent className="text-[13px] leading-5"><MarkdownContent className="!leading-5 [&>*+*]:!mt-2">{visibleUserContent(turn.user.content)}</MarkdownContent></BubbleContent></Bubble></MessageContent></Message> : null}
      <ProcessArea key={`${turn.execution?.id ?? turn.id}-${isExecutionActive(turn.execution) ? "active" : "settled"}`} execution={turn.execution} events={turn.events} messages={turn.intermediate} />
      {turn.finalContent ? <Message><MessageContent><MarkdownContent className="text-[13px] !leading-5 [&>*+*]:!mt-2">{turn.finalContent}</MarkdownContent>{turn.final?.streaming ? <span className="text-xs text-muted-foreground">正在生成…</span> : null}</MessageContent></Message> : null}
    </div>
  </MessageScrollerItem>
}

function PromptCard({ turn, issue }: { turn: ChatTurn; issue: Issue }) {
  const [raw, setRaw] = React.useState(false)
  const content = turn.user?.content ?? ""
  const comment = extractCommentPrompt(content)
  const isComment = turn.promptKind === "comment"
  return <Message align="end"><MessageContent className="w-full max-w-[46rem]">
    <Card className="group/prompt relative w-full gap-3 py-4">
      <CardHeader className="px-4">
        <div className="flex items-start gap-3">
          <span className="flex size-9 shrink-0 items-center justify-center rounded-lg bg-muted text-muted-foreground">{isComment ? <MessageSquareText className="size-4" /> : <ClipboardList className="size-4" />}</span>
          <div className="min-w-0 flex-1">
            <CardTitle className="text-sm">{isComment ? "Issue 评论" : "任务已创建"}</CardTitle>
            <CardDescription className="mt-1 line-clamp-3 break-words">{isComment ? `${comment.author || "操作员"} 评论了 ${issue.identifier}` : `${issue.identifier} · ${issue.title}`}</CardDescription>
          </div>
          <Badge variant="outline" className="shrink-0">{raw ? "原始模式" : "渲染模式"}</Badge>
        </div>
      </CardHeader>
      <CardContent className="px-4">
        {raw ? <pre className="overflow-x-auto whitespace-pre-wrap break-all font-mono text-xs leading-5 text-muted-foreground">{content}</pre> : isComment ? <MarkdownContent className="text-[13px] !leading-5 break-words [&>*+*]:!mt-2">{comment.body || visibleUserContent(content)}</MarkdownContent> : <div className="flex flex-col gap-4">
          <div className="flex flex-wrap gap-2"><StatusBadge status={issue.status} /><Badge variant="outline">{issue.priority}</Badge><Badge variant="outline">{issue.workMode}</Badge>{turn.execution?.agentId ? <Badge variant="secondary">{turn.execution.agentId}</Badge> : null}</div>
          <PromptField label="描述" value={issue.description || issue.context || "未填写"} />
          <PromptField label="目标" value={issue.objective || "未填写"} />
          {issue.constraints ? <PromptField label="执行边界" value={issue.constraints} /> : null}
        </div>}
      </CardContent>
      <div className="pointer-events-none absolute right-3 bottom-3 opacity-0 transition-opacity group-hover/prompt:opacity-100 group-focus-within/prompt:opacity-100">
        <Button className="pointer-events-auto shadow-sm" type="button" variant="secondary" size="sm" onClick={() => setRaw((value) => !value)} aria-label={raw ? "切换到渲染模式" : "切换到原始模式"} title={raw ? "切换到渲染模式" : "切换到原始模式"}>
          {raw ? <LayoutTemplate data-icon="inline-start" /> : <Code2 data-icon="inline-start" />}
          {raw ? "渲染模式" : "原始模式"}
        </Button>
      </div>
    </Card>
  </MessageContent></Message>
}

function PromptField({ label, value }: { label: string; value: string }) {
  return <div><p className="mb-1 text-xs font-medium text-muted-foreground">{label}</p><MarkdownContent className="text-[13px] !leading-5 [&>*+*]:!mt-2">{value}</MarkdownContent></div>
}

function extractCommentPrompt(content: string) {
  return {
    author: content.match(/Comment from ([^:\n]+):/)?.[1]?.trim() ?? "",
    body: content.match(/<comment>\s*([\s\S]*?)\s*<\/comment>/)?.[1]?.trim() ?? "",
  }
}

function timelineSignature(timeline: IssueAgentTimeline | null) {
  if (!timeline) return ""
  return [
    timeline.session.updatedAt,
    ...timeline.executions.map((item) => `${item.id}:${item.status}:${item.updatedAt}:${item.currentTool ?? ""}`),
    ...timeline.messages.map((item) => `${item.id}:${item.updatedAt}:${item.content.length}:${item.streaming ? 1 : 0}`),
    ...timeline.events.map((item) => `${item.id}:${item.createdAt}:${item.outputJson?.length ?? 0}`),
  ].join("|")
}

function ProcessArea({ execution, events, messages }: { execution?: Execution; events: ExecutionEvent[]; messages: ChatMessage[] }) {
  const [open, setOpen] = React.useState(() => isExecutionActive(execution))
  const [event, setEvent] = React.useState<ExecutionEvent | null>(null)
  if (events.length === 0 && messages.length === 0) return null
  const elapsed = execution ? elapsedLabel(execution.startedAt, execution.finishedAt) : ""
  return <><Collapsible open={open} onOpenChange={setOpen}>
    <CollapsibleTrigger className="flex w-full items-center gap-2 py-1 text-xs text-muted-foreground">{open ? <ChevronDown /> : <ChevronRight />}<span>{open ? `收起执行过程 · ${events.length + messages.length} 条` : `已处理 ${elapsed} · ${events.length + messages.length} 条过程消息`}</span></CollapsibleTrigger>
    <CollapsibleContent><div className="flex max-h-64 flex-col gap-0 overflow-y-auto py-1">{messages.map((message) => <MarkdownContent key={message.id} className="py-0.5 text-xs !leading-4 [&>*+*]:!mt-1">{message.content}</MarkdownContent>)}{events.map((item) => <button type="button" key={item.id} onClick={() => setEvent(item)} className="flex min-h-7 w-full items-center gap-2 rounded-md px-2 py-0.5 text-left text-xs leading-4 text-muted-foreground hover:bg-muted"><TerminalSquare className="size-4 shrink-0" /><span className="truncate">{toolDescription(item, execution)}</span></button>)}</div></CollapsibleContent>
  </Collapsible><Dialog open={Boolean(event)} onOpenChange={(value) => !value && setEvent(null)}><DialogContent className="h-[520px] max-w-2xl overflow-hidden"><DialogHeader><DialogTitle>{event ? toolDescription(event, execution) : "工具详情"}</DialogTitle><DialogDescription>工具调用的输入、输出和执行状态</DialogDescription></DialogHeader><div className="min-h-0 flex-1 overflow-y-auto rounded-md bg-muted p-4 font-mono text-xs whitespace-pre-wrap">{event ? [event.detail, event.inputJson, event.outputJson].filter(Boolean).join("\n\n") : null}</div></DialogContent></Dialog></>
}

function isExecutionActive(execution?: Execution) {
  return Boolean(execution && ["queued", "starting", "running", "waiting_approval"].includes(execution.status))
}

function visibleUserContent(content: string) {
  return content
    .replace(/\s*<agent_permission_boundary>[\s\S]*?<\/agent_permission_boundary>\s*/g, "")
    .trim()
}

function buildChatTurns(executions: Execution[], messages: ChatMessage[], events: ExecutionEvent[], fallbackFinal: string): ChatTurn[] {
  const result: ChatTurn[] = []
  const orderedExecutions = [...executions].sort((left, right) => left.startedAt.localeCompare(right.startedAt))
  for (const execution of orderedExecutions) {
    const executionMessages = messages.filter((message) => message.executionId === execution.id).sort((left, right) => left.createdAt.localeCompare(right.createdAt))
    const executionEvents = events.filter((event) => event.executionId === execution.id).sort((left, right) => left.createdAt.localeCompare(right.createdAt))
    const executionTurns: Array<ChatTurn & { assistants: ChatMessage[] }> = []
    let current: (ChatTurn & { assistants: ChatMessage[] }) | undefined
    for (const message of executionMessages) {
      if (message.role === "user") {
        current = { id: message.id, user: message, assistants: [], intermediate: [], events: [], finalContent: "", execution, startedAt: message.createdAt }
        executionTurns.push(current)
      } else if (message.role === "assistant") {
        if (!current) {
          current = { id: `${execution.id}-output`, assistants: [], intermediate: [], events: [], finalContent: "", execution, startedAt: message.createdAt }
          executionTurns.push(current)
        }
        current.assistants.push(message)
      }
    }
    if (executionTurns.length === 0 && (executionEvents.length > 0 || execution.finalResult)) {
      executionTurns.push({ id: `${execution.id}-turn`, assistants: [], intermediate: [], events: [], finalContent: "", execution, startedAt: execution.startedAt })
    }
    for (const event of executionEvents) {
      const target = [...executionTurns].reverse().find((turn) => turn.startedAt <= event.createdAt) ?? executionTurns[0]
      target?.events.push(event)
    }
    for (const [turnIndex, turn] of executionTurns.entries()) {
      if (turnIndex === 0 && turn.user) {
        if (["wakeup", "rework"].includes(execution.kind)) turn.promptKind = "comment"
        else if (["planning", "work", "continuation"].includes(execution.kind)) turn.promptKind = "task"
      }
      turn.final = turn.assistants[turn.assistants.length - 1]
      turn.finalContent = turn.final?.content ?? ""
      turn.intermediate = turn.assistants.slice(0, -1)
      if (execution.finalResultSubmitted && execution.finalResult && turn === executionTurns[executionTurns.length - 1]) {
        if (turn.finalContent && turn.finalContent !== execution.finalResult) turn.intermediate.push(turn.final!)
        turn.finalContent = execution.finalResult
      }
      result.push(turn)
    }
  }
  if (fallbackFinal && !result.some((turn) => turn.finalContent === fallbackFinal)) {
    const last = result[result.length - 1]
    if (last) {
      if (last.finalContent) last.intermediate.push(last.final!)
      last.finalContent = fallbackFinal
    } else {
      result.push({ id: "issue-final", intermediate: [], events: [], finalContent: fallbackFinal, startedAt: "" })
    }
  }
  return result
}
function toolDescription(event: ExecutionEvent, execution?: Execution) { return execution?.toolsSnapshot.find((tool) => tool.name === event.toolName)?.description || event.title || event.toolName || "调用工具" }
function elapsedLabel(start: string, finish?: string) { const ms = Math.max(0, new Date(finish ?? Date.now()).getTime() - new Date(start).getTime()); const seconds = Math.floor(ms / 1000); return seconds >= 60 ? `${Math.floor(seconds / 60)}m ${seconds % 60}s` : `${seconds}s` }
function IssueDetails({ issue, detail, agents, comment, busy, onCommentChange, onComment }: { issue: Issue; detail: IssueDetail; agents: AgentDefinition[]; comment: string; busy: boolean; onCommentChange: (value: string) => void; onComment: (event: React.FormEvent) => void }) {
  const assignee = agents.find((agent) => agent.id === issue.assigneeAgentId)
  const validation = !issue.objective.trim() ? "无目标 · 不验收" : issue.validationDisabled ? "验收已关闭" : issue.validationMode === "automatic" ? "自动验收" : `固定验收 · ${detail.validations.length} 轮 / 常规上限 ${issue.maxValidationAttempts}`
  return <div className="flex flex-col gap-6">
    <div className="flex flex-col gap-1"><h2 className="text-lg font-semibold">{issue.title}</h2><p className="font-mono text-xs text-muted-foreground">{issue.identifier} · {issue.id}</p></div>
    <div className="flex flex-wrap gap-2"><StatusBadge status={issue.status} /><Badge variant="outline">{issue.priority}</Badge><Badge variant="outline">{issue.workMode}</Badge><Badge variant="outline">深度 {issue.requestDepth}</Badge><Badge variant="outline">{validation}</Badge><Badge variant="secondary">{issue.executionPhase}</Badge>{issue.assigneeAgentId ? <Badge variant="secondary">{assignee?.name ?? issue.assigneeAgentId}</Badge> : null}</div>
    <div><h3 className="mb-2 text-sm font-medium">描述</h3><MarkdownContent className="text-[13px] !leading-5 [&>*+*]:!mt-2">{issue.description || issue.context || "未填写"}</MarkdownContent></div>
    <div><h3 className="mb-2 text-sm font-medium">目标</h3><MarkdownContent className="text-[13px] !leading-5 [&>*+*]:!mt-2">{issue.objective || "未填写"}</MarkdownContent></div>
    <div><h3 className="mb-2 text-sm font-medium">执行边界</h3><MarkdownContent className="text-[13px] !leading-5 [&>*+*]:!mt-2">{issue.constraints || "未填写"}</MarkdownContent></div>
    {issue.error ? <div><h3 className="mb-2 text-sm font-medium">错误</h3><p className="text-sm text-destructive">{issue.error}</p></div> : null}
    <div className="border-t pt-5">
      <h3 className="mb-3 text-sm font-medium">Issue 评论 · {detail.commentsPage.total}</h3>
      <div className="flex h-80 flex-col"><IssueCommentsList comments={detail.comments} hasMore={false} loadingMore={false} onLoadMore={() => {}} onCopy={() => {}} /></div>
      <form onSubmit={onComment} className="mt-4 flex flex-col gap-2">
        <InputGroup>
          <InputGroupTextarea value={comment} onChange={(event) => onCommentChange(event.target.value)} placeholder="评论 Issue 并通过评论上下文唤醒 Agent…" className="min-h-20 resize-none text-[13px]" />
          <InputGroupAddon align="block-end" className="justify-between">
            <span className="text-xs text-muted-foreground">此内容和 Agent 回复都会写入 Issue 评论</span>
            <InputGroupButton type="submit" size="sm" variant="default" disabled={busy || !comment.trim()}><Send data-icon="inline-start" />发表评论</InputGroupButton>
          </InputGroupAddon>
        </InputGroup>
      </form>
    </div>
  </div>
}
function Dependencies({ blockedBy, blocks }: { blockedBy: Issue[]; blocks: Issue[] }) { return <div className="space-y-6"><RelationList title="被以下 Issue 阻塞" items={blockedBy} /><RelationList title="阻塞以下 Issue" items={blocks} /></div> }
function RelationList({ title, items }: { title: string; items: Issue[] }) { return <div><h3 className="mb-3 flex items-center gap-2 font-medium"><GitBranch className="size-4" />{title}</h3>{items.length ? items.map((item) => <Link key={item.id} to={`/issues/${item.id}`} className="mb-2 block rounded-md border p-3 text-sm hover:bg-muted">{item.identifier} · {item.title}</Link>) : <p className="text-sm text-muted-foreground">无</p>}</div> }
function Abandoned({ issue, issues }: { issue: Issue; issues: Issue[] }) { const items = issues.filter((item) => item.objectiveAbandoned && (item.id === issue.id || item.parentId === issue.id)); return <div><h3 className="mb-4 flex items-center gap-2 font-medium"><XCircle className="size-4" />放弃的目标</h3>{items.length ? items.map((item) => <div key={item.id} className="mb-3 rounded-md border p-3"><p className="font-medium">{item.identifier} · {item.objective}</p><MarkdownContent className="mt-2 text-sm">{item.abandonmentReason || "未记录原因"}</MarkdownContent></div>) : <p className="text-sm text-muted-foreground">没有放弃的目标</p>}</div> }
