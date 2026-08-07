import * as React from "react"
import {
  ChevronRight,
  Pencil,
  Plus,
  SlidersHorizontal,
  Trash2,
} from "lucide-react"
import { Link, useLocation, useNavigate, useParams } from "react-router-dom"
import { toast } from "sonner"

import { ChatComposer } from "@/components/chat-composer"
import { IssueCommentsList } from "@/components/issue-comments-list"
import { IssueAgentActivity } from "@/components/issue-agent-activity"
import { IssueChildTree } from "@/components/issue-child-tree"
import { IssueRuntimeBadge } from "@/components/issue-runtime-badge"
import { MarkdownContent } from "@/components/markdown-content"
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
import { Textarea } from "@/components/ui/textarea"
import {
  createIssue,
  createIssueComment,
  deleteIssue,
  fetchIssue,
  updateIssue,
} from "@/lib/api"
import { formatTime } from "@/lib/format"
import { issueRuntimeMap, issueRuntimeOrUnavailable } from "@/lib/issue-runtime"
import { issueWorkflowStatus } from "@/lib/issue-workflow"
import { useAppState } from "@/lib/state"
import { cn } from "@/lib/utils"
import type {
  CreateIssueInput,
  Issue,
  IssueDetail,
  IssueStatus,
} from "@/types"

const issueStatuses: IssueStatus[] = [
  "todo",
  "in_progress",
  "in_review",
  "done",
  "cancelled",
]
const issuePriorities = ["high", "middle", "low"] as const

type IssueEditForm = {
  title: string
  description: string
  objective: string
  priority: (typeof issuePriorities)[number]
  status: IssueStatus
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
  const [comment, setComment] = React.useState("")
  const [busy, setBusy] = React.useState(false)
  const [editOpen, setEditOpen] = React.useState(false)
  const [deleteOpen, setDeleteOpen] = React.useState(false)
  const [createOpen, setCreateOpen] = React.useState(false)
  const [childTreeOpen, setChildTreeOpen] = React.useState(false)
  const [childForm, setChildForm] =
    React.useState<ChildIssueForm>(emptyChildIssue)
  const [editForm, setEditForm] = React.useState<IssueEditForm>({
    title: "",
    description: "",
    objective: "",
    priority: "middle",
    status: "todo",
    assigneeAgentId: "",
  })
  const load = React.useCallback(
    () => (issueId ? fetchIssue(issueId) : Promise.resolve(null)),
    [issueId]
  )
  React.useEffect(() => {
    let active = true
    void load()
      .then((next) => {
        if (active && next) setDetail(next)
      })
      .catch((error) =>
        toast.error(error instanceof Error ? error.message : "读取 Issue 失败")
      )
    return () => {
      active = false
    }
  }, [load, state?.updatedAt])
  if (!detail)
    return (
      <div className="flex size-full items-center justify-center">
        <Spinner />
      </div>
    )
  const issue =
    state?.issues.find((candidate) => candidate.id === detail.issue.id) ??
    detail.issue
  const runtime = issueRuntimeOrUnavailable(
    issueRuntimeMap([
      ...(detail.runtime ? [detail.runtime] : []),
      ...(state?.issueRuntimes ?? []),
    ]),
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

  const beginEdit = () => {
    setEditForm({
      title: issue.title,
      description: issue.description,
      objective: issue.objective,
      priority: issue.priority,
      status: issueWorkflowStatus(issue.status),
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
        fromBoard && root ? `/tasks/${root.id}/board` : "/issues"
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
        constraints: issue.constraints ?? "",
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
    if (!body || busy) return
    setBusy(true)
    setComment("")
    try {
      await createIssueComment(issue.id, body)
      const next = await load()
      if (next) setDetail(next)
    } catch (error) {
      setComment(body)
      toast.error(error instanceof Error ? error.message : "评论失败")
    } finally {
      setBusy(false)
    }
  }
  return (
    <div className="relative size-full min-h-0 overflow-hidden">
      <div className="size-full min-h-0 overflow-y-auto pr-1 lg:pr-[380px]">
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
            <h1 className="truncate text-base font-semibold" title={issue.title}>
              {issue.title}
            </h1>
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
              <Popover>
                <PopoverTrigger render={<Button variant="outline" size="sm" />}>
                  <SlidersHorizontal data-icon="inline-start" />
                  属性
                </PopoverTrigger>
                <PopoverContent align="start" className="w-80">
                  <div className="flex flex-col gap-2.5">
                    <Meta label="负责人" value={assignee?.name || "未委派"} />
                    <Meta label="创建者" value={creatorName} />
                    <Meta label="状态" value={issueWorkflowStatus(issue.status)} />
                    <Meta label="优先级" value={issue.priority} />
                    <Meta label="协作模式" value="Board Autonomy" />
                    <Meta label="创建时间" value={formatTime(issue.createdAt)} />
                    {issue.startedAt ? (
                      <Meta label="开始时间" value={formatTime(issue.startedAt)} />
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
                    <Meta label="更新时间" value={formatTime(issue.updatedAt)} />
                  </div>
                </PopoverContent>
              </Popover>
              <IssueRuntimeBadge runtime={runtime} />
              <Badge variant="outline">{issue.priority}</Badge>
              <Badge variant="secondary">{assignee?.name || "未委派"}</Badge>
            </div>
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
              <Button
                size="sm"
                variant="outline"
                onClick={() => {
                  setChildForm(emptyChildIssue)
                  setCreateOpen(true)
                }}
              >
                <Plus />
                新建
              </Button>
            </section>
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
          <div className="mt-4 flex flex-col gap-5">
            <Section
              label="描述"
              value={issue.description || "未填写"}
            />
            <Section label="目标" value={issue.objective || "未填写"} />
            <Section
              label="执行边界"
              value={issue.constraints || "未填写"}
            />
            {issue.result ? (
              <Section label="最新交付" value={issue.result} />
            ) : null}
          </div>
          <section className="mt-4 border-t pt-3">
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
            className="mt-2"
            value={comment}
            onValueChange={setComment}
            onSend={() => void submit()}
            sending={busy}
            placeholder="在 Board 上发表评论…"
            ariaLabel="发表评论"
            sendLabel="发表评论"
            hint=""
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
              <Textarea
                id="issue-edit-description"
                value={editForm.description}
                onChange={(event) =>
                  setEditForm((current) => ({
                    ...current,
                    description: event.target.value,
                  }))
                }
                className="min-h-28"
              />
            </div>
            <div className="grid gap-2">
              <Label htmlFor="issue-edit-objective">目标</Label>
              <Textarea
                id="issue-edit-objective"
                value={editForm.objective}
                onChange={(event) =>
                  setEditForm((current) => ({
                    ...current,
                    objective: event.target.value,
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
                          {status}
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
              <Textarea
                id="child-issue-objective"
                value={childForm.objective}
                onChange={(event) =>
                  setChildForm((current) => ({
                    ...current,
                    objective: event.target.value,
                  }))
                }
              />
            </div>
            <div className="grid gap-2">
              <Label htmlFor="child-issue-description">描述</Label>
              <Textarea
                id="child-issue-description"
                value={childForm.description}
                onChange={(event) =>
                  setChildForm((current) => ({
                    ...current,
                    description: event.target.value,
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

function Section({ label, value }: { label: string; value: string }) {
  return (
    <section>
      <h2 className="mb-2 text-sm font-medium text-muted-foreground">
        {label}
      </h2>
      <MarkdownContent className="text-sm !leading-6">{value}</MarkdownContent>
    </section>
  )
}
function Meta({ label, value }: { label: string; value: string }) {
  return (
    <div className="flex justify-between gap-3">
      <span className="text-muted-foreground">{label}</span>
      <span className="text-right font-medium">{value}</span>
    </div>
  )
}
