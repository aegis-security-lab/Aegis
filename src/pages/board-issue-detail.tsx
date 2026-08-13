import * as React from "react"
import {
  ArrowDown,
  ArrowUp,
  CheckCircle2,
  CircleDashed,
  History,
  ChevronRight,
  MessageSquareWarning,
  Pencil,
  Plus,
  RefreshCw,
  SlidersHorizontal,
  Trash2,
} from "lucide-react"
import { Link, useLocation, useNavigate, useParams } from "react-router-dom"
import { toast } from "sonner"

import { ChatComposer } from "@/components/chat-composer"
import { IssueCommentsList } from "@/components/issue-comments-list"
import { IssueMentionTextarea } from "@/components/issue-mention-input"
import { IssueAgentActivity } from "@/components/issue-agent-activity"
import { IssueChildTree } from "@/components/issue-child-tree"
import { IssueRuntimeBadge } from "@/components/issue-runtime-badge"
import { MarkdownContent } from "@/components/markdown-content"
import { ManualRejectValidationDialog } from "@/components/manual-reject-validation-dialog"
import { Badge } from "@/components/ui/badge"
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@/components/ui/alert-dialog"
import { Button } from "@/components/ui/button"
import {
  Empty,
  EmptyContent,
  EmptyDescription,
  EmptyHeader,
  EmptyMedia,
  EmptyTitle,
} from "@/components/ui/empty"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from "@/components/ui/popover"
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import { Spinner } from "@/components/ui/spinner"
import { Switch } from "@/components/ui/switch"
import {
  createIssue,
  createIssueComment,
  deleteIssue,
  fetchIssue,
  updateIssue,
} from "@/lib/api"
import { formatTime } from "@/lib/format"
import { issuesForTask } from "@/lib/collections"
import { issueRuntimeMap, issueRuntimeOrUnavailable } from "@/lib/issue-runtime"
import {
  issueLabelMeta,
  issueLabelOptions,
  issueLabelsOf,
  issueWorkflowStatus,
  workflowStatusLabels,
  workflowStatuses,
} from "@/lib/issue-workflow"
import { useAppState } from "@/lib/state"
import { cn } from "@/lib/utils"
import type {
  CreateIssueInput,
  Issue,
  IssueDetail,
  IssueLabel,
  IssueStatus,
} from "@/types"

function newestProjection<T extends { updatedAt: string }>(
  detailValue: T | undefined,
  stateValue: T | undefined
) {
  if (!detailValue) return stateValue
  if (!stateValue) return detailValue
  const detailTime = Date.parse(detailValue.updatedAt)
  const stateTime = Date.parse(stateValue.updatedAt)
  if (Number.isNaN(stateTime)) return detailValue
  if (Number.isNaN(detailTime)) return stateValue
  return stateTime > detailTime ? stateValue : detailValue
}

const issueStatuses: IssueStatus[] = workflowStatuses
const issuePriorities = ["high", "middle", "low"] as const

type IssueEditForm = {
  title: string
  description: string
  objective: string
  priority: (typeof issuePriorities)[number]
  status: IssueStatus
  labels: IssueLabel[]
  assigneeAgentId: string
}

type ChildIssueForm = {
  title: string
  description: string
  objective: string
  priority: CreateIssueInput["priority"]
  assigneeAgentId: string
}

const emptyChildIssue: ChildIssueForm = {
  title: "",
  description: "",
  objective: "",
  priority: "middle",
  assigneeAgentId: "",
}

export function BoardIssueDetailPage() {
  const { issueId } = useParams()
  const navigate = useNavigate()
  const location = useLocation()
  const { state } = useAppState()
  const fromBoard = new URLSearchParams(location.search).get("view") === "board"
  const [detail, setDetail] = React.useState<IssueDetail | null>(null)
  const [loadError, setLoadError] = React.useState("")
  const [retryRevision, setRetryRevision] = React.useState(0)
  const [comment, setComment] = React.useState("")
  const [newObjectiveEnabled, setNewObjectiveEnabled] = React.useState(false)
  const [newObjective, setNewObjective] = React.useState("")
  const [objectiveHistoryOpen, setObjectiveHistoryOpen] = React.useState(false)
  const [busy, setBusy] = React.useState(false)
  const [editOpen, setEditOpen] = React.useState(false)
  const [deleteOpen, setDeleteOpen] = React.useState(false)
  const [manualRejectOpen, setManualRejectOpen] = React.useState(false)
  const [createOpen, setCreateOpen] = React.useState(false)
  const [childTreeOpen, setChildTreeOpen] = React.useState(false)
  const contentScrollRef = React.useRef<HTMLDivElement>(null)
  const detailRequestRef = React.useRef(0)
  const [childForm, setChildForm] =
    React.useState<ChildIssueForm>(emptyChildIssue)
  const [editForm, setEditForm] = React.useState<IssueEditForm>({
    title: "",
    description: "",
    objective: "",
    priority: "middle",
    status: "todo",
    labels: [],
    assigneeAgentId: "",
  })
  const load = React.useCallback(
    () => (issueId ? fetchIssue(issueId) : Promise.resolve(null)),
    [issueId]
  )
  const stateIssue = state?.issues.find((candidate) => candidate.id === issueId)
  const stateRuntime = state?.issueRuntimes?.find(
    (candidate) => candidate.issueId === issueId
  )
  const detailRevision = issueId
    ? state?.issueDetailRevisions?.[issueId]
    : undefined
  const hasDetail = detail !== null
  React.useEffect(() => {
    const request = ++detailRequestRef.current
    let active = true
    const timer = window.setTimeout(
      () => {
        void load()
          .then((next) => {
            if (active && request === detailRequestRef.current && next) {
              setLoadError("")
              setDetail(next)
            }
          })
          .catch((error) => {
            if (active && request === detailRequestRef.current) {
              setLoadError(
                error instanceof Error ? error.message : "读取 Issue 失败"
              )
            }
          })
      },
      hasDetail ? 150 : 0
    )
    return () => {
      active = false
      window.clearTimeout(timer)
    }
  }, [
    load,
    stateIssue?.updatedAt,
    stateRuntime?.updatedAt,
    detailRevision,
    retryRevision,
    hasDetail,
  ])
  if (!detail && loadError)
    return (
      <Empty className="size-full border-0">
        <EmptyHeader>
          <EmptyMedia variant="icon">
            <MessageSquareWarning />
          </EmptyMedia>
          <EmptyTitle>Issue 读取失败</EmptyTitle>
          <EmptyDescription>{loadError}。检查服务连接后重试。</EmptyDescription>
        </EmptyHeader>
        <EmptyContent>
          <Button
            variant="outline"
            onClick={() => setRetryRevision((current) => current + 1)}
          >
            <RefreshCw data-icon="inline-start" />
            重新加载
          </Button>
        </EmptyContent>
      </Empty>
    )
  if (!detail)
    return (
      <div className="flex size-full items-center justify-center">
        <Spinner />
      </div>
    )
  const issue = newestProjection(detail.issue, stateIssue) ?? detail.issue
  const resolvedRuntime = newestProjection(detail.runtime, stateRuntime)
  const runtime = issueRuntimeOrUnavailable(
    issueRuntimeMap(resolvedRuntime ? [resolvedRuntime] : []),
    issue.id
  )
  const assignee = state?.agents.find(
    (agent) => agent.id === issue.assigneeAgentId
  )
  const creator = state?.agents.find((agent) => agent.id === issue.createdBy)
  const creatorName =
    creator?.name ??
    (issue.createdBy === "operator"
      ? "操作员"
      : issue.createdBy === "system"
        ? "系统"
        : issue.createdBy || "未知")
  const deleteIssueCount = countIssueTree(issue.id, state?.issues ?? [])
  const parentIssue = issue.parentId
    ? state?.issues.find((candidate) => candidate.id === issue.parentId)
    : undefined
  const subtreeIssues = collectSubtree(issue.id, state?.issues ?? [])
  const rootIssue = rootIssueOf(issue.id, state?.issues ?? [])
  const taskSourceId = issue.taskSourceId ?? rootIssue?.taskSourceId
  const taskIssues = taskSourceId
    ? issuesForTask(taskSourceId, state?.issues ?? [])
    : []

  const beginEdit = () => {
    setEditForm({
      title: issue.title,
      description: issue.description,
      objective: issue.objective,
      priority: issue.priority,
      status: issueWorkflowStatus(issue.status),
      labels: issueLabelsOf(issue),
      assigneeAgentId: issue.assigneeAgentId ?? "",
    })
    setEditOpen(true)
  }
  const saveEdit = async () => {
    if (!editForm.title.trim() || busy) return
    setBusy(true)
    try {
      const input: Partial<typeof issue> = {
        title: editForm.title.trim(),
        description: editForm.description,
        objective: editForm.objective,
        priority: editForm.priority,
        labels: editForm.labels,
        assigneeAgentId: editForm.assigneeAgentId,
      }
      if (editForm.status !== issueWorkflowStatus(issue.status)) {
        input.status = editForm.status
      }
      const updated = await updateIssue(issue.id, input)
      setDetail((current) =>
        current ? { ...current, issue: updated } : current
      )
      setEditOpen(false)
      toast.success("Issue 已更新")
    } catch (error) {
      toast.error(error instanceof Error ? error.message : "更新 Issue 失败")
    } finally {
      setBusy(false)
    }
  }
  const removeIssue = async () => {
    if (busy) return
    setBusy(true)
    try {
      const result = await deleteIssue(issue.id)
      toast.success(`已删除 ${result.deletedIssues} 个 Issue`)
      const root = rootIssueOf(issue.id, state?.issues ?? [])
      navigate(
        fromBoard && root?.taskSourceId
          ? `/tasks/${root.taskSourceId}/board`
          : "/issues"
      )
    } catch (error) {
      toast.error(error instanceof Error ? error.message : "删除 Issue 失败")
    } finally {
      setBusy(false)
      setDeleteOpen(false)
    }
  }
  const createChildIssue = async () => {
    if (!childForm.title.trim() || busy) return
    setBusy(true)
    try {
      await createIssue({
        parentId: issue.id,
        title: childForm.title.trim(),
        description: childForm.description,
        objective: childForm.objective,
        priority: childForm.priority,
        status: "todo",
        workMode: "autonomous",
        assigneeAgentId: childForm.assigneeAgentId || undefined,
        workspace: issue.workspace,
      })
      const next = await load()
      if (next) setDetail(next)
      setChildForm(emptyChildIssue)
      setCreateOpen(false)
      toast.success(
        childForm.assigneeAgentId
          ? "子 Issue 已创建并分配"
          : "未分配的子 Issue 已加入 Todo"
      )
    } catch (error) {
      toast.error(error instanceof Error ? error.message : "创建子 Issue 失败")
    } finally {
      setBusy(false)
    }
  }
  const submit = async () => {
    const body = comment.trim()
    const objective = newObjectiveEnabled ? newObjective.trim() : ""
    if ((!body && !objective) || busy) return
    setBusy(true)
    setComment("")
    setNewObjective("")
    try {
      await createIssueComment(issue.id, body, objective)
      const next = await load()
      if (next) setDetail(next)
      setNewObjectiveEnabled(false)
    } catch (error) {
      setComment(body)
      setNewObjective(objective)
      toast.error(error instanceof Error ? error.message : "评论失败")
    } finally {
      setBusy(false)
    }
  }
  return (
    <div className="relative size-full min-h-0 overflow-hidden">
      <div
        ref={contentScrollRef}
        className="size-full min-h-0 overflow-y-auto pr-1 lg:pr-[380px]"
      >
        <div className="flex min-h-full flex-col gap-4">
          {parentIssue ? (
            <Link
              to={`/issues/${parentIssue.id}${fromBoard ? "?view=board" : ""}`}
              className="flex h-6 min-w-0 items-center text-xs text-muted-foreground hover:text-foreground"
              title={parentIssue.title}
            >
              <span className="truncate">
                {parentIssue.identifier} · {parentIssue.title}
              </span>
            </Link>
          ) : null}
          <header className="flex flex-col gap-2">
            <div className="flex min-w-0 items-center gap-2.5">
              <h1
                className="min-w-0 truncate text-base font-semibold"
                title={issue.title}
              >
                {issue.title}
              </h1>
              <div className="shrink-0" title={runtime.detail}>
                <IssueRuntimeBadge runtime={runtime} />
              </div>
            </div>
            <div className="flex flex-wrap items-center gap-2">
              <Button variant="outline" size="sm" onClick={beginEdit}>
                <Pencil />
                编辑
              </Button>
              <Button
                variant="destructive"
                size="sm"
                onClick={() => setDeleteOpen(true)}
              >
                <Trash2 />
                删除
              </Button>
              {(issue.status === "done" || issue.status === "in_review") &&
              detail.validations.some(
                (validation) => validation.status === "passed"
              ) ? (
                <Button
                  variant="outline"
                  size="sm"
                  onClick={() => setManualRejectOpen(true)}
                >
                  <MessageSquareWarning />
                  改判验收
                </Button>
              ) : null}
              <Popover>
                <PopoverTrigger render={<Button variant="outline" size="sm" />}>
                  <SlidersHorizontal data-icon="inline-start" />
                  属性
                </PopoverTrigger>
                <PopoverContent align="start" className="w-80">
                  <div className="flex flex-col gap-2.5">
                    <Meta label="负责人" value={assignee?.name || "未委派"} />
                    <Meta label="创建者" value={creatorName} />
                    <Meta
                      label="状态"
                      value={
                        workflowStatusLabels[issueWorkflowStatus(issue.status)]
                      }
                    />
                    <Meta
                      label="标签"
                      value={
                        issueLabelsOf(issue)
                          .map((label) => issueLabelMeta[label].label)
                          .join("、") || "无"
                      }
                    />
                    <Meta label="优先级" value={issue.priority} />
                    <Meta label="协作模式" value="Board Autonomy" />
                    <Meta
                      label="创建时间"
                      value={formatTime(issue.createdAt)}
                    />
                    {issue.startedAt ? (
                      <Meta
                        label="开始时间"
                        value={formatTime(issue.startedAt)}
                      />
                    ) : null}
                    {issue.completedAt ? (
                      <Meta
                        label="完成时间"
                        value={formatTime(issue.completedAt)}
                      />
                    ) : null}
                    {issue.cancelledAt ? (
                      <Meta
                        label="取消时间"
                        value={formatTime(issue.cancelledAt)}
                      />
                    ) : null}
                    <Meta
                      label="更新时间"
                      value={formatTime(issue.updatedAt)}
                    />
                  </div>
                </PopoverContent>
              </Popover>
              {issueLabelsOf(issue).map((label) => (
                <Badge
                  key={label}
                  variant="outline"
                  className={cn("font-medium", issueLabelMeta[label].className)}
                >
                  {issueLabelMeta[label].label}
                </Badge>
              ))}
              <Badge variant="outline">{issue.priority}</Badge>
              <Badge variant="secondary">{assignee?.name || "未委派"}</Badge>
            </div>
            {detail.children.length > 0 ? (
              <section className="flex min-w-0 items-center gap-2">
                <button
                  type="button"
                  onClick={() => setChildTreeOpen((open) => !open)}
                  aria-expanded={childTreeOpen}
                  className="flex h-7 items-center gap-1.5 rounded-md px-2 text-xs font-medium text-muted-foreground hover:bg-muted hover:text-foreground"
                >
                  <ChevronRight
                    className={cn(
                      "size-3.5 transition-transform",
                      childTreeOpen && "rotate-90"
                    )}
                  />
                  子 Issues
                  <span className="tabular-nums">{detail.children.length}</span>
                </button>
              </section>
            ) : null}
            {childTreeOpen ? (
              <div className="min-w-0 pl-4">
                <IssueChildTree
                  rootId={issue.id}
                  issues={subtreeIssues}
                  runtimes={state?.issueRuntimes ?? []}
                  view={fromBoard ? "board" : undefined}
                />
              </div>
            ) : null}
          </header>
          <div className="mt-4 flex flex-col gap-5 rounded-xl bg-card px-4 py-4">
            <MarkdownContent className="text-sm !leading-7">
              {issue.description || "未填写 Issue 描述"}
            </MarkdownContent>
            {issue.objective ? (
              <div className="overflow-hidden rounded-lg bg-muted/45">
                <div className="flex items-start gap-3 px-4 py-3">
                  <div className="min-w-0 flex-1">
                    <div className="mb-1.5 flex items-center gap-2 text-[11px] font-medium text-muted-foreground">
                      <span>当前目标</span>
                      {detail.objectives?.[0]?.validationRounds ? (
                        <span>
                          已验收 {detail.objectives[0].validationRounds} 轮
                        </span>
                      ) : (
                        <span>尚未验收</span>
                      )}
                    </div>
                    <MarkdownContent className="text-sm !leading-6 text-foreground/85">
                      {issue.objective}
                    </MarkdownContent>
                  </div>
                  {detail.objectives?.[0]?.validationPassed ? (
                    <CheckCircle2
                      className="mt-0.5 size-4 shrink-0 text-emerald-600"
                      aria-label="验收通过"
                    />
                  ) : (
                    <CircleDashed
                      className="mt-0.5 size-4 shrink-0 text-muted-foreground"
                      aria-label="等待验收"
                    />
                  )}
                </div>
                {(detail.objectives?.length ?? 0) > 1 ? (
                  <>
                    <button
                      type="button"
                      className="flex h-9 w-full items-center gap-2 border-t border-border/60 px-4 text-xs text-muted-foreground hover:bg-muted/60 hover:text-foreground"
                      onClick={() => setObjectiveHistoryOpen((open) => !open)}
                      aria-expanded={objectiveHistoryOpen}
                    >
                      <History className="size-3.5" />
                      {objectiveHistoryOpen
                        ? "收起目标历史"
                        : `查看目标历史 · ${detail.objectives.length - 1}`}
                      <ChevronRight
                        className={cn(
                          "ml-auto size-3.5 transition-transform",
                          objectiveHistoryOpen && "rotate-90"
                        )}
                      />
                    </button>
                    {objectiveHistoryOpen ? (
                      <div className="border-t border-border/60 px-4 py-1">
                        {detail.objectives.slice(1).map((objective) => (
                          <div
                            key={objective.id}
                            className="grid grid-cols-[auto_minmax(0,1fr)] gap-x-3 border-b border-border/50 py-3 last:border-b-0"
                          >
                            <span className="pt-0.5 font-mono text-[11px] text-muted-foreground">
                              v{objective.version}
                            </span>
                            <div className="min-w-0">
                              <MarkdownContent className="text-sm !leading-6 text-muted-foreground">
                                {objective.content}
                              </MarkdownContent>
                              <div className="mt-1.5 flex flex-wrap gap-x-3 text-[11px] text-muted-foreground">
                                <span>{formatTime(objective.createdAt)}</span>
                                <span>
                                  {objective.validationRounds
                                    ? `${objective.validationRounds} 轮验收 · ${objective.validationPassed ? "通过" : objective.validationStatus}`
                                    : "未验收"}
                                </span>
                              </div>
                            </div>
                          </div>
                        ))}
                      </div>
                    ) : null}
                  </>
                ) : null}
              </div>
            ) : null}
          </div>
          <section className="mt-4" aria-label="评论">
            <IssueCommentsList
              embedded
              comments={detail.comments}
              hasMore={false}
              loadingMore={false}
              onLoadMore={() => {}}
              onCopy={(value) => void navigator.clipboard.writeText(value)}
            />
          </section>
          <ChatComposer
            className="mt-2 px-1 pb-1"
            value={comment}
            onValueChange={setComment}
            onSend={() => void submit()}
            sending={busy}
            placeholder="在 Board 上发表评论…"
            ariaLabel="发表评论"
            hint=""
            mentionIssues={taskIssues}
            additionalCanSend={
              newObjectiveEnabled && Boolean(newObjective.trim())
            }
            extra={
              <label className="flex h-7 cursor-pointer items-center gap-2 rounded-full border border-input bg-background px-2.5 text-xs text-muted-foreground transition-colors hover:bg-muted/60 hover:text-foreground">
                <Switch
                  size="sm"
                  checked={newObjectiveEnabled}
                  onCheckedChange={setNewObjectiveEnabled}
                />
                设置新目标
              </label>
            }
            beforeAddon={
              newObjectiveEnabled ? (
                <div className="border-t border-border/60 px-3 py-2">
                  <textarea
                    value={newObjective}
                    onChange={(event) => setNewObjective(event.target.value)}
                    placeholder="输入新的验收目标；提交后将成为 Agent 执行与验收的当前目标…"
                    aria-label="新的验收目标"
                    rows={2}
                    className="block min-h-12 w-full resize-none bg-transparent text-[13px] leading-6 outline-none placeholder:text-muted-foreground"
                  />
                </div>
              ) : null
            }
            inputGroupClassName="has-[[data-slot=input-group-control]:focus-visible]:ring-1"
          />
        </div>
      </div>
      <aside className="absolute inset-y-0 right-2 hidden w-[360px] flex-col overflow-hidden rounded-xl bg-card lg:flex">
        <IssueAgentActivity
          executions={detail.executions}
          messages={detail.messages}
          events={detail.events}
          validations={detail.validations}
          approvals={detail.approvals}
        />
      </aside>
      <div className="fixed right-6 bottom-6 z-30 flex flex-col gap-2 lg:right-[392px]">
        <Button
          type="button"
          variant="outline"
          size="icon-sm"
          className="rounded-full bg-background/90 shadow-md backdrop-blur"
          aria-label="回到顶部"
          title="回到顶部"
          onClick={() =>
            contentScrollRef.current?.scrollTo({ top: 0, behavior: "smooth" })
          }
        >
          <ArrowUp />
        </Button>
        <Button
          type="button"
          variant="outline"
          size="icon-sm"
          className="rounded-full bg-background/90 shadow-md backdrop-blur"
          aria-label="前往底部"
          title="前往底部"
          onClick={() => {
            const target = contentScrollRef.current
            if (target)
              target.scrollTo({ top: target.scrollHeight, behavior: "smooth" })
          }}
        >
          <ArrowDown />
        </Button>
      </div>
      <Dialog open={editOpen} onOpenChange={setEditOpen}>
        <DialogContent className="max-h-[90svh] overflow-y-auto sm:max-w-2xl">
          <DialogHeader>
            <DialogTitle>编辑 {issue.identifier}</DialogTitle>
            <DialogDescription>
              修改 Board 中的 Issue 定义、状态和具体负责人。
            </DialogDescription>
          </DialogHeader>
          <div className="grid gap-5 py-2">
            <div className="grid gap-2">
              <Label htmlFor="issue-edit-title">标题</Label>
              <Input
                id="issue-edit-title"
                value={editForm.title}
                onChange={(event) =>
                  setEditForm((current) => ({
                    ...current,
                    title: event.target.value,
                  }))
                }
              />
            </div>
            <div className="grid gap-2">
              <Label htmlFor="issue-edit-description">描述</Label>
              <IssueMentionTextarea
                id="issue-edit-description"
                value={editForm.description}
                issues={taskIssues}
                onValueChange={(value) =>
                  setEditForm((current) => ({
                    ...current,
                    description: value,
                  }))
                }
                className="min-h-28"
              />
            </div>
            <div className="grid gap-2">
              <Label htmlFor="issue-edit-objective">目标</Label>
              <IssueMentionTextarea
                id="issue-edit-objective"
                value={editForm.objective}
                issues={taskIssues}
                onValueChange={(value) =>
                  setEditForm((current) => ({
                    ...current,
                    objective: value,
                  }))
                }
                className="min-h-28"
              />
            </div>
            <div className="grid gap-4 sm:grid-cols-3">
              <div className="grid gap-2">
                <Label>状态</Label>
                <Select
                  value={editForm.status}
                  onValueChange={(value) =>
                    value &&
                    setEditForm((current) => ({
                      ...current,
                      status: value as IssueStatus,
                    }))
                  }
                >
                  <SelectTrigger>
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectGroup>
                      {issueStatuses.map((status) => (
                        <SelectItem key={status} value={status}>
                          {workflowStatusLabels[status]}
                        </SelectItem>
                      ))}
                    </SelectGroup>
                  </SelectContent>
                </Select>
              </div>
              <div className="grid gap-2">
                <Label>优先级</Label>
                <Select
                  value={editForm.priority}
                  onValueChange={(value) =>
                    value &&
                    setEditForm((current) => ({
                      ...current,
                      priority: value as IssueEditForm["priority"],
                    }))
                  }
                >
                  <SelectTrigger>
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectGroup>
                      {issuePriorities.map((priority) => (
                        <SelectItem key={priority} value={priority}>
                          {priority}
                        </SelectItem>
                      ))}
                    </SelectGroup>
                  </SelectContent>
                </Select>
              </div>
              <div className="grid gap-2">
                <Label>负责人</Label>
                <Select
                  value={editForm.assigneeAgentId || "unassigned"}
                  onValueChange={(value) =>
                    setEditForm((current) => ({
                      ...current,
                      assigneeAgentId:
                        !value || value === "unassigned" ? "" : value,
                    }))
                  }
                >
                  <SelectTrigger>
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectGroup>
                      <SelectItem value="unassigned">未委派</SelectItem>
                      {(state?.agents ?? [])
                        .filter(
                          (agent) =>
                            agent.enabled &&
                            !agent.internal &&
                            agent.category !== "concierge"
                        )
                        .map((agent) => (
                          <SelectItem key={agent.id} value={agent.id}>
                            {agent.name}
                          </SelectItem>
                        ))}
                    </SelectGroup>
                  </SelectContent>
                </Select>
              </div>
            </div>
            <div className="grid gap-2">
              <Label>标签</Label>
              <div className="flex flex-wrap items-center gap-2">
                {issueLabelOptions.map((label) => {
                  const selected = editForm.labels.includes(label)
                  return (
                    <Button
                      key={label}
                      type="button"
                      variant={selected ? "default" : "outline"}
                      size="sm"
                      className="h-7 px-2.5 text-xs"
                      onClick={() =>
                        setEditForm((current) => ({
                          ...current,
                          labels: selected
                            ? current.labels.filter(
                                (candidate) => candidate !== label
                              )
                            : [...current.labels, label],
                        }))
                      }
                    >
                      {issueLabelMeta[label].label}
                    </Button>
                  )
                })}
              </div>
            </div>
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setEditOpen(false)}>
              取消
            </Button>
            <Button
              disabled={busy || !editForm.title.trim()}
              onClick={() => void saveEdit()}
            >
              保存修改
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
      <Dialog open={createOpen} onOpenChange={setCreateOpen}>
        <DialogContent className="sm:max-w-2xl">
          <DialogHeader>
            <DialogTitle>新建子 Issue</DialogTitle>
            <DialogDescription>
              负责人可留空；未分配 Issue 会保留在 Todo，之后再由 Agent
              或操作员认领。
            </DialogDescription>
          </DialogHeader>
          <div className="grid gap-4 py-2">
            <div className="grid gap-2">
              <Label htmlFor="child-issue-title">标题</Label>
              <Input
                id="child-issue-title"
                value={childForm.title}
                onChange={(event) =>
                  setChildForm((current) => ({
                    ...current,
                    title: event.target.value,
                  }))
                }
              />
            </div>
            <div className="grid gap-2">
              <Label htmlFor="child-issue-objective">目标</Label>
              <IssueMentionTextarea
                id="child-issue-objective"
                value={childForm.objective}
                issues={taskIssues}
                onValueChange={(value) =>
                  setChildForm((current) => ({
                    ...current,
                    objective: value,
                  }))
                }
              />
            </div>
            <div className="grid gap-2">
              <Label htmlFor="child-issue-description">描述</Label>
              <IssueMentionTextarea
                id="child-issue-description"
                value={childForm.description}
                issues={taskIssues}
                onValueChange={(value) =>
                  setChildForm((current) => ({
                    ...current,
                    description: value,
                  }))
                }
              />
            </div>
            <div className="grid gap-4 sm:grid-cols-2">
              <div className="grid gap-2">
                <Label>优先级</Label>
                <Select
                  value={childForm.priority}
                  onValueChange={(value) =>
                    value &&
                    setChildForm((current) => ({
                      ...current,
                      priority: value as CreateIssueInput["priority"],
                    }))
                  }
                >
                  <SelectTrigger>
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectGroup>
                      {issuePriorities.map((priority) => (
                        <SelectItem key={priority} value={priority}>
                          {priority}
                        </SelectItem>
                      ))}
                    </SelectGroup>
                  </SelectContent>
                </Select>
              </div>
              <div className="grid gap-2">
                <Label>负责人（可选）</Label>
                <Select
                  value={childForm.assigneeAgentId || "unassigned"}
                  onValueChange={(value) =>
                    setChildForm((current) => ({
                      ...current,
                      assigneeAgentId:
                        !value || value === "unassigned" ? "" : value,
                    }))
                  }
                >
                  <SelectTrigger>
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectGroup>
                      <SelectItem value="unassigned">未分配</SelectItem>
                      {(state?.agents ?? [])
                        .filter(
                          (agent) =>
                            agent.enabled &&
                            !agent.internal &&
                            agent.category !== "concierge"
                        )
                        .map((agent) => (
                          <SelectItem key={agent.id} value={agent.id}>
                            {agent.name} · {agent.category}
                          </SelectItem>
                        ))}
                    </SelectGroup>
                  </SelectContent>
                </Select>
              </div>
            </div>
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setCreateOpen(false)}>
              取消
            </Button>
            <Button
              disabled={busy || !childForm.title.trim()}
              onClick={() => void createChildIssue()}
            >
              {busy ? <Spinner /> : <Plus />}
              创建 Issue
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
      <AlertDialog open={deleteOpen} onOpenChange={setDeleteOpen}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>永久删除 {issue.identifier}？</AlertDialogTitle>
            <AlertDialogDescription>
              将永久删除该 Issue
              {deleteIssueCount > 1
                ? ` 及其 ${deleteIssueCount - 1} 个子 Issue`
                : ""}
              ，同时删除关联的执行记录、评论、附件和依赖关系。正在执行的 Issue
              必须先取消或归档。此操作无法撤销。
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel disabled={busy}>取消</AlertDialogCancel>
            <AlertDialogAction
              variant="destructive"
              disabled={busy}
              onClick={(event) => {
                event.preventDefault()
                void removeIssue()
              }}
            >
              {busy ? <Spinner /> : <Trash2 />}
              确认删除 {deleteIssueCount} 项
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
      <ManualRejectValidationDialog
        open={manualRejectOpen}
        onOpenChange={setManualRejectOpen}
        issueId={issue.id}
        onRejected={async () => {
          const next = await load()
          if (next) setDetail(next)
        }}
      />
    </div>
  )
}

function countIssueTree(issueID: string, issues: IssueDetail["children"]) {
  const children = new Map<string, string[]>()
  for (const issue of issues) {
    if (!issue.parentId) continue
    const values = children.get(issue.parentId) ?? []
    values.push(issue.id)
    children.set(issue.parentId, values)
  }
  const visited = new Set<string>()
  const visit = (id: string) => {
    if (visited.has(id)) return
    visited.add(id)
    for (const childID of children.get(id) ?? []) visit(childID)
  }
  visit(issueID)
  return visited.size
}

function collectSubtree(rootId: string, issues: Issue[]) {
  const byID = new Map(issues.map((issue) => [issue.id, issue]))
  const result: Issue[] = []
  const seen = new Set<string>()
  const visit = (id: string) => {
    if (seen.has(id)) return
    seen.add(id)
    const issue = byID.get(id)
    if (!issue) return
    result.push(issue)
    for (const candidate of issues) {
      if (candidate.parentId === id) visit(candidate.id)
    }
  }
  visit(rootId)
  return result
}

function rootIssueOf(issueId: string, issues: Issue[]) {
  const byID = new Map(issues.map((issue) => [issue.id, issue]))
  let current = byID.get(issueId)
  const seen = new Set<string>()
  while (current?.parentId && !seen.has(current.id)) {
    seen.add(current.id)
    current = byID.get(current.parentId)
  }
  return current
}

function Meta({ label, value }: { label: string; value: string }) {
  return (
    <div className="flex justify-between gap-3">
      <span className="text-muted-foreground">{label}</span>
      <span className="text-right font-medium">{value}</span>
    </div>
  )
}
