import * as React from "react"
/* eslint-disable react-hooks/set-state-in-effect */
import {
  ArrowLeft,
  ChevronDown,
  Clock3,
  GitBranch,
  MessageSquare,
  Play,
  RadioTower,
  RotateCcw,
  Send,
  ShieldCheck,
  ShieldOff,
  Users,
  XCircle,
} from "lucide-react"
import { Link, useParams } from "react-router-dom"
import { toast } from "sonner"
import { ExecutionEvents } from "@/components/execution-events"
import { InterruptToolButton } from "@/components/interrupt-tool-button"
import { IssueCommentsList } from "@/components/issue-comments-list"
import { IssueTree } from "@/components/issue-tree"
import { MarkdownContent } from "@/components/markdown-content"
import { PageHeader } from "@/components/page-header"
import { StatusBadge } from "@/components/status-badge"
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert"
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogMedia,
  AlertDialogTitle,
} from "@/components/ui/alert-dialog"
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
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from "@/components/ui/collapsible"
import {
  Empty,
  EmptyDescription,
  EmptyHeader,
  EmptyTitle,
} from "@/components/ui/empty"
import { Spinner } from "@/components/ui/spinner"
import { Progress } from "@/components/ui/progress"
import { ScrollArea } from "@/components/ui/scroll-area"
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs"
import { Textarea } from "@/components/ui/textarea"
import {
  abandonIssue,
  cancelTask,
  createIssueComment,
  dispatchIssue,
  fetchIssue,
  fetchIssueComments,
  fetchIssueEvents,
  fetchIssueExecutions,
  manuallyRejectIssueValidation,
  setIssueValidationDisabled,
  sendChat,
} from "@/lib/api"
import {
  chronological,
  mergeById,
  reverseChronological,
} from "@/lib/collections"
import { formatTime } from "@/lib/format"
import { useAppState } from "@/lib/state"
import { cn } from "@/lib/utils"
import type {
  AgentDefinition,
  Issue,
  IssueDetail,
  IssueValidation,
  TaskBroadcast,
} from "@/types"
export function IssueDetailPage() {
  const { issueId } = useParams()
  const { state, refresh } = useAppState()
  const [detail, setDetail] = React.useState<IssueDetail | null>(null)
  const [loading, setLoading] = React.useState(true)
  const [body, setBody] = React.useState("")
  const [chat, setChat] = React.useState("")
  const [busy, setBusy] = React.useState(false)
  const [cancelOpen, setCancelOpen] = React.useState(false)
  const [abandonOpen, setAbandonOpen] = React.useState(false)
  const [validationOpen, setValidationOpen] = React.useState(false)
  const [manualRejectOpen, setManualRejectOpen] = React.useState(false)
  const [manualRejectReason, setManualRejectReason] = React.useState("")
  const [loadingMore, setLoadingMore] = React.useState<
    "comments" | "events" | "executions" | null
  >(null)
  const load = React.useCallback(async () => {
    if (!issueId) return
    try {
      const next = await fetchIssue(issueId)
      setDetail((current) => {
        if (!current || current.issue.id !== next.issue.id) return next
        return {
          ...next,
          comments: mergeById(current.comments, next.comments, chronological),
          events: mergeById(current.events, next.events, reverseChronological),
          executions: mergeById(current.executions, next.executions, (a, b) =>
            b.startedAt.localeCompare(a.startedAt)
          ),
          commentsPage:
            current.comments.length > next.comments.length
              ? { ...current.commentsPage, total: next.commentsPage.total }
              : next.commentsPage,
          eventsPage:
            current.events.length > next.events.length
              ? { ...current.eventsPage, total: next.eventsPage.total }
              : next.eventsPage,
          executionsPage:
            current.executions.length > next.executions.length
              ? { ...current.executionsPage, total: next.executionsPage.total }
              : next.executionsPage,
        }
      })
    } catch (e) {
      toast.error(e instanceof Error ? e.message : "读取失败")
    } finally {
      setLoading(false)
    }
  }, [issueId])
  React.useEffect(() => {
    void load()
  }, [load])
  const taskIssueIDs = React.useMemo(() => {
    if (!detail) return new Set<string>()
    const issues = state?.issues ?? [detail.issue, ...detail.children]
    const rootID = findTaskRootID(detail.issue.id, issues)
    return new Set(collectTaskIssues(rootID, issues).map((issue) => issue.id))
  }, [detail, state?.issues])
  const hasLiveExecution =
    (state?.executions ?? detail?.executions ?? []).some(
      (execution) =>
        taskIssueIDs.has(execution.issueId) &&
        ["queued", "starting", "running", "waiting_approval"].includes(
          execution.status
        )
    ) ?? false
  React.useEffect(() => {
    if (!hasLiveExecution) return
    const refreshActivity = async () => {
      if (!issueId) return
      try {
        const [comments, events, executions] = await Promise.all([
          fetchIssueComments(issueId, undefined, 20),
          fetchIssueEvents(issueId, undefined, 20),
          fetchIssueExecutions(issueId, undefined, 20),
        ])
        setDetail((current) => {
          if (!current) return current
          const mergedComments = mergeById(
            current.comments,
            comments.items,
            chronological
          )
          const mergedEvents = mergeById(
            current.events,
            events.items,
            reverseChronological
          )
          const mergedExecutions = mergeById(
            current.executions,
            executions.items,
            (a, b) => b.startedAt.localeCompare(a.startedAt)
          )
          const commentsMissed =
            current.comments.length > 0 &&
            comments.items.length > 0 &&
            !comments.items.some((item) =>
              current.comments.some((existing) => existing.id === item.id)
            ) &&
            comments.page.total > current.commentsPage.total
          const eventsMissed =
            current.events.length > 0 &&
            events.items.length > 0 &&
            !events.items.some((item) =>
              current.events.some((existing) => existing.id === item.id)
            ) &&
            events.page.total > current.eventsPage.total
          const executionsMissed =
            current.executions.length > 0 &&
            executions.items.length > 0 &&
            !executions.items.some((item) =>
              current.executions.some((existing) => existing.id === item.id)
            ) &&
            executions.page.total > current.executionsPage.total
          return {
            ...current,
            comments: commentsMissed ? comments.items : mergedComments,
            events: eventsMissed ? events.items : mergedEvents,
            executions: executionsMissed ? executions.items : mergedExecutions,
            commentsPage: commentsMissed
              ? comments.page
              : { ...current.commentsPage, total: comments.page.total },
            eventsPage: eventsMissed
              ? events.page
              : { ...current.eventsPage, total: events.page.total },
            executionsPage: executionsMissed
              ? executions.page
              : { ...current.executionsPage, total: executions.page.total },
          }
        })
      } catch {
        // The next interval retries; keep the last complete snapshot visible.
      }
    }
    const timer = window.setInterval(() => void refreshActivity(), 2000)
    return () => window.clearInterval(timer)
  }, [hasLiveExecution, issueId])
  if (loading)
    return (
      <div className="flex min-h-64 items-center justify-center">
        <Spinner />
      </div>
    )
  if (!detail)
    return (
      <Empty>
        <EmptyHeader>
          <EmptyTitle>Issue 不存在</EmptyTitle>
          <EmptyDescription>它可能已被删除。</EmptyDescription>
        </EmptyHeader>
      </Empty>
    )
  const issue =
    state?.issues.find((candidate) => candidate.id === detail.issue.id) ??
    detail.issue
  const knownAgents = state?.agents ?? []
  const associatedAgentIds = Array.from(
    new Set(
      [
        issue.assigneeAgentId,
        ...detail.agentSessions.map((session) => session.agentId),
      ].filter((id): id is string => Boolean(id))
    )
  )
  const directChildren = (state?.issues ?? detail.children).filter(
    (candidate) => candidate.parentId === issue.id
  )
  const completedChildren = directChildren.filter((candidate) =>
    ["done", "cancelled"].includes(candidate.status)
  ).length
  const childProgress = directChildren.length
    ? Math.round((completedChildren / directChildren.length) * 100)
    : 0
  const abandonedObjectives = issue.parentId
    ? issue.objectiveAbandoned
      ? [issue]
      : []
    : collectTaskIssues(issue.id, state?.issues ?? [issue]).filter(
        (candidate) => candidate.objectiveAbandoned
      )
  const active = detail.executions.find((e) =>
    ["queued", "starting", "running", "waiting_approval"].includes(e.status)
  )
  const activeToolExecution = detail.executions.find(
    (execution) => execution.status === "running" && execution.currentTool
  )
  const dispatch = async () => {
    setBusy(true)
    try {
      await dispatchIssue(issue.id)
      toast.success("已进入调度队列")
      await load()
    } catch (e) {
      toast.error(e instanceof Error ? e.message : "调度失败")
    } finally {
      setBusy(false)
    }
  }
  const cancel = async () => {
    setBusy(true)
    try {
      const result = await cancelTask(issue.id, "操作员从任务详情取消")
      setCancelOpen(false)
      toast.success("任务树已取消", {
        description: `${result.cancelledIssues} 个 Issues、${result.cancelledExecutions} 个运行被取消`,
      })
      await Promise.all([refresh(), load()])
    } catch (e) {
      toast.error(e instanceof Error ? e.message : "取消任务失败")
    } finally {
      setBusy(false)
    }
  }
  const abandon = async () => {
    setBusy(true)
    try {
      const updated = await abandonIssue(
        issue.id,
        "操作员从 Issue 详情手动放弃目标"
      )
      setAbandonOpen(false)
      toast.success(
        updated.executionPhase === "summarizing"
          ? "已通知负责人停止工作并总结"
          : "目标已放弃"
      )
      await Promise.all([refresh(), load()])
    } catch (e) {
      toast.error(e instanceof Error ? e.message : "放弃目标失败")
    } finally {
      setBusy(false)
    }
  }
  const changeValidation = async () => {
    setBusy(true)
    try {
      const disabled = !issue.validationDisabled
      await setIssueValidationDisabled(issue.id, disabled)
      setValidationOpen(false)
      toast.success(disabled ? "此 Issue 已永久跳过验收" : "已恢复目标验收")
      await Promise.all([refresh(), load()])
    } catch (e) {
      toast.error(e instanceof Error ? e.message : "修改验收设置失败")
    } finally {
      setBusy(false)
    }
  }
  const manualRejectValidation = async () => {
    if (!manualRejectReason.trim()) {
      toast.error("请填写验收不通过原因")
      return
    }
    setBusy(true)
    try {
      await manuallyRejectIssueValidation(issue.id, manualRejectReason.trim())
      setManualRejectOpen(false)
      setManualRejectReason("")
      toast.success("已记录手动不通过，验收 Agent 将继续核验")
      await Promise.all([refresh(), load()])
    } catch (e) {
      toast.error(e instanceof Error ? e.message : "手动验收失败")
    } finally {
      setBusy(false)
    }
  }
  const comment = async (e: React.FormEvent) => {
    e.preventDefault()
    if (!body.trim()) return
    setBusy(true)
    try {
      await createIssueComment(issue.id, body)
      setBody("")
      await load()
    } catch (x) {
      toast.error(x instanceof Error ? x.message : "评论失败")
    } finally {
      setBusy(false)
    }
  }
  const copyComment = async (value: string) => {
    try {
      await copyText(value)
      toast.success("评论已复制")
    } catch {
      toast.error("复制失败，请检查浏览器的剪贴板权限")
    }
  }
  const loadOlder = async (kind: "comments" | "events" | "executions") => {
    if (loadingMore) return
    setLoadingMore(kind)
    try {
      if (kind === "comments") {
        const page = await fetchIssueComments(
          issue.id,
          detail.commentsPage.nextCursor
        )
        setDetail((current) =>
          current
            ? {
                ...current,
                comments: mergeById(
                  current.comments,
                  page.items,
                  chronological
                ),
                commentsPage: page.page,
              }
            : current
        )
      } else if (kind === "events") {
        const page = await fetchIssueEvents(
          issue.id,
          detail.eventsPage.nextCursor
        )
        setDetail((current) =>
          current
            ? {
                ...current,
                events: mergeById(
                  current.events,
                  page.items,
                  reverseChronological
                ),
                eventsPage: page.page,
              }
            : current
        )
      } else {
        const page = await fetchIssueExecutions(
          issue.id,
          detail.executionsPage.nextCursor
        )
        setDetail((current) =>
          current
            ? {
                ...current,
                executions: mergeById(current.executions, page.items, (a, b) =>
                  b.startedAt.localeCompare(a.startedAt)
                ),
                executionsPage: page.page,
              }
            : current
        )
      }
    } catch (reason) {
      toast.error(reason instanceof Error ? reason.message : "读取历史记录失败")
    } finally {
      setLoadingMore(null)
    }
  }
  const steer = async (e: React.FormEvent) => {
    e.preventDefault()
    if (!chat.trim()) return
    setBusy(true)
    try {
      await sendChat(issue.id, chat, active?.id)
      setChat("")
      await load()
    } catch (x) {
      toast.error(x instanceof Error ? x.message : "发送失败")
    } finally {
      setBusy(false)
    }
  }
  return (
    <div className="flex flex-col gap-6">
      <PageHeader
        eyebrow={`${issue.identifier} · ${issue.id}`}
        title={issue.title}
        showIdentity
        actions={
          <>
            <Button
              variant="ghost"
              render={
                <Link
                  to={issue.parentId ? `/issues/${issue.parentId}` : "/tasks"}
                />
              }
              nativeButton={false}
            >
              <ArrowLeft data-icon="inline-start" />
              返回
            </Button>
            {["todo", "backlog"].includes(issue.status) &&
              issue.executionPhase !== "recovering" && (
                <Button disabled={busy} onClick={() => void dispatch()}>
                  <Play />
                  执行
                </Button>
              )}
            {activeToolExecution ? (
              <InterruptToolButton
                execution={activeToolExecution}
                onInterrupted={async () => {
                  await Promise.all([refresh(), load()])
                }}
              />
            ) : null}
            {!issue.parentId &&
              !["done", "cancelled"].includes(issue.status) && (
                <Button
                  variant="destructive"
                  disabled={busy}
                  onClick={() => setCancelOpen(true)}
                >
                  <XCircle data-icon="inline-start" />
                  取消任务
                </Button>
              )}
            {issue.objective.trim() &&
              !["done", "cancelled"].includes(issue.status) && (
                <Button
                  variant="outline"
                  disabled={busy || issue.abandonRequestedAt != null}
                  onClick={() => setValidationOpen(true)}
                >
                  {issue.validationDisabled ? (
                    <ShieldCheck data-icon="inline-start" />
                  ) : (
                    <ShieldOff data-icon="inline-start" />
                  )}
                  {issue.validationDisabled ? "恢复验收" : "取消验收"}
                </Button>
              )}
            {issue.objective.trim() &&
              ["done", "in_review"].includes(issue.status) &&
              detail.validations.some((validation) => validation.status === "passed") && (
                <Button
                  variant="outline"
                  disabled={busy}
                  onClick={() => setManualRejectOpen(true)}
                >
                  <ShieldOff data-icon="inline-start" />
                  手动验收不通过
                </Button>
              )}
            {issue.parentId &&
              !["done", "cancelled"].includes(issue.status) && (
                <Button
                  variant="destructive"
                  disabled={busy || issue.abandonRequestedAt != null}
                  onClick={() => setAbandonOpen(true)}
                >
                  <XCircle data-icon="inline-start" />
                  {issue.abandonRequestedAt ? "取消后总结中" : "放弃目标"}
                </Button>
              )}
          </>
        }
      />
      <AlertDialog open={cancelOpen} onOpenChange={setCancelOpen}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogMedia>
              <XCircle />
            </AlertDialogMedia>
            <AlertDialogTitle>取消整个任务树？</AlertDialogTitle>
            <AlertDialogDescription>
              所有未完成的子 Issues、运行中的 Pi Sessions、排队执行、待审批和
              Agent Wakeups
              都会被取消。已经完成的历史记录会保留，此操作不能直接撤销。
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel disabled={busy}>返回</AlertDialogCancel>
            <AlertDialogAction
              variant="destructive"
              disabled={busy}
              onClick={() => void cancel()}
            >
              {busy ? (
                <Spinner data-icon="inline-start" />
              ) : (
                <XCircle data-icon="inline-start" />
              )}
              确认取消
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
      <AlertDialog open={abandonOpen} onOpenChange={setAbandonOpen}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogMedia>
              <XCircle />
            </AlertDialogMedia>
            <AlertDialogTitle>放弃这个目标？</AlertDialogTitle>
            <AlertDialogDescription>
              此 Issue 的未完成子树会被取消，正在进行的验收会立即停止。Aegis
              会用一条特殊系统消息要求当前负责人停止实施并提交最终总结；总结后直接以“目标已放弃”结束，不再经过验收。
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel disabled={busy}>返回</AlertDialogCancel>
            <AlertDialogAction
              variant="destructive"
              disabled={busy}
              onClick={() => void abandon()}
            >
              {busy ? (
                <Spinner data-icon="inline-start" />
              ) : (
                <XCircle data-icon="inline-start" />
              )}
              确认放弃
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
      <AlertDialog open={validationOpen} onOpenChange={setValidationOpen}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogMedia>
              {issue.validationDisabled ? <ShieldCheck /> : <ShieldOff />}
            </AlertDialogMedia>
            <AlertDialogTitle>
              {issue.validationDisabled ? "恢复目标验收？" : "取消目标验收？"}
            </AlertDialogTitle>
            <AlertDialogDescription>
              {issue.validationDisabled
                ? "此 Issue 的下一次产出会重新交给固定验收会话核对目标。"
                : "此设置会持久保存在 Issue 上，后续所有 Worker 产出都会跳过验收 Agent。若当前正在验收，会立即停止验收并采用现有产出。"}
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel disabled={busy}>返回</AlertDialogCancel>
            <AlertDialogAction
              disabled={busy}
              onClick={() => void changeValidation()}
            >
              {busy ? (
                <Spinner data-icon="inline-start" />
              ) : issue.validationDisabled ? (
                <ShieldCheck data-icon="inline-start" />
              ) : (
                <ShieldOff data-icon="inline-start" />
              )}
              {issue.validationDisabled ? "确认恢复" : "确认取消验收"}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
      <AlertDialog open={manualRejectOpen} onOpenChange={setManualRejectOpen}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogMedia><ShieldOff /></AlertDialogMedia>
            <AlertDialogTitle>手动改判为验收不通过？</AlertDialogTitle>
            <AlertDialogDescription>
              系统会保留原验收通过记录，新增一轮固定验收，并把你的原因作为验收 Agent 后续核验的权威基准。
            </AlertDialogDescription>
          </AlertDialogHeader>
          <Textarea
            value={manualRejectReason}
            onChange={(event) => setManualRejectReason(event.target.value)}
            placeholder="请说明为什么原验收通过不正确，以及需要继续核验或修复的内容…"
            rows={6}
            autoFocus
          />
          <AlertDialogFooter>
            <AlertDialogCancel disabled={busy}>返回</AlertDialogCancel>
            <AlertDialogAction
              variant="destructive"
              disabled={busy || !manualRejectReason.trim()}
              onClick={() => void manualRejectValidation()}
            >
              {busy ? <Spinner data-icon="inline-start" /> : <ShieldOff data-icon="inline-start" />}
              确认不通过并继续验收
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
      <div className="flex flex-wrap gap-2">
        <StatusBadge status={issue.status} />
        <Badge variant="outline">{issue.priority}</Badge>
        <Badge variant="outline">{issue.workMode}</Badge>
        <Badge variant="outline">深度 {issue.requestDepth}</Badge>
        <Badge
          variant={
            !issue.objective.trim() || issue.validationDisabled
              ? "secondary"
              : "outline"
          }
        >
          {!issue.objective.trim()
            ? "无目标 · 不验收"
            : issue.validationDisabled
              ? "验收已关闭"
              : issue.validationMode === "automatic"
                ? "自动验收"
                : `固定验收 · ${detail.validations.length} 轮 / 常规上限 ${issue.maxValidationAttempts}`}
        </Badge>
        {issue.executionPhase === "waiting_children" && (
          <Badge variant="secondary" className="gap-1">
            <Clock3 className="size-3" />
            等待子树
          </Badge>
        )}
        {issue.executionPhase === "resuming" && (
          <Badge variant="secondary" className="gap-1">
            <RotateCcw className="size-3" />
            汇总中
          </Badge>
        )}
        {issue.executionPhase === "validating" && (
          <Badge variant="secondary" className="gap-1">
            <ShieldCheck className="size-3" />
            验收中
          </Badge>
        )}
        {issue.executionPhase === "summarizing" && (
          <Badge variant="secondary" className="gap-1">
            <XCircle className="size-3" />
            取消后总结中
          </Badge>
        )}
        {issue.executionPhase === "recovering" && (
          <Badge variant="secondary">
            <Spinner data-icon="inline-start" />
            重启恢复中
          </Badge>
        )}
        {issue.assigneeAgentId && (
          <Badge variant="secondary">
            {state?.agents.find((a) => a.id === issue.assigneeAgentId)?.name ??
              issue.assigneeAgentId}
          </Badge>
        )}
      </div>
      {abandonedObjectives.length > 0 && (
        <Alert>
          <XCircle />
          <AlertTitle>已放弃目标 {abandonedObjectives.length} 项</AlertTitle>
          <AlertDescription>
            <div className="flex flex-col gap-3">
              {abandonedObjectives.map((candidate) => (
                <div key={candidate.id} className="flex flex-col gap-1">
                  <Link
                    className="font-medium underline-offset-4 hover:underline"
                    to={`/issues/${candidate.id}`}
                  >
                    {candidate.identifier} · {candidate.objective}
                  </Link>
                  <MarkdownContent className="text-sm">
                    {candidate.abandonmentReason || "未记录放弃证明"}
                  </MarkdownContent>
                </div>
              ))}
            </div>
          </AlertDescription>
        </Alert>
      )}
      {["waiting_children", "resuming"].includes(issue.executionPhase) && (
        <Alert className="border-primary/20 bg-primary/5 px-4 py-3">
          {issue.executionPhase === "waiting_children" ? (
            <Clock3 className="size-4" />
          ) : (
            <RotateCcw className="size-4" />
          )}
          <AlertTitle>
            {issue.executionPhase === "waiting_children"
              ? "父 Issue 已释放执行权，正在等待子树"
              : "子树已完成，Agent 正在汇总结果"}
          </AlertTitle>
          <AlertDescription className="space-y-2">
            <p>
              {completedChildren}/{directChildren.length} 个直属子 Issues
              已完成；全部完成后， Aegis 会创建 continuation
              Execution，而不是直接把父 Issue 标记完成。
            </p>
            <Progress
              value={childProgress}
              aria-label="直属子 Issue 完成进度"
            />
          </AlertDescription>
        </Alert>
      )}
      {issue.executionPhase === "validating" && (
        <Alert className="border-primary/20 bg-primary/5 px-4 py-3">
          <ShieldCheck className="size-4" />
          <AlertTitle>验收 Agent 正在核对目标</AlertTitle>
          <AlertDescription>
            Worker
            的产出尚未被标记为完成。若未达到目标，反馈会写入评论，并自动交还原负责人继续执行。
          </AlertDescription>
        </Alert>
      )}
      {issue.executionPhase === "summarizing" && (
        <Alert className="border-destructive/20 bg-destructive/5 px-4 py-3">
          <XCircle className="size-4" />
          <AlertTitle>操作员已放弃目标，正在等待最终总结</AlertTitle>
          <AlertDescription>
            原负责人已收到停止实施的系统消息。它只需总结已完成工作、附件和剩余风险；本次结束不会进入验收。
          </AlertDescription>
        </Alert>
      )}
      {issue.executionPhase === "recovering" && (
        <Alert>
          <RotateCcw />
          <AlertTitle>正在恢复重启前的 Execution</AlertTitle>
          <AlertDescription>
            Aegis 会复用原 Pi
            Session、最近消息、工具事件和执行检查点，并按照并发上限自动继续。
            {issue.recoveryPhase && ` 中断前阶段：${issue.recoveryPhase}。`}
          </AlertDescription>
        </Alert>
      )}
      {issue.validationDisabled && issue.executionPhase !== "summarizing" && (
        <Alert className="border-amber-500/20 bg-amber-500/5 px-4 py-3">
          <ShieldOff className="size-4" />
          <AlertTitle>此 Issue 已关闭目标验收</AlertTitle>
          <AlertDescription>
            后续 Worker 产出会直接完成，不再交给验收
            Agent。可在上方随时恢复验收。
          </AlertDescription>
        </Alert>
      )}
      <div className="grid gap-5 xl:grid-cols-[minmax(0,1fr)_380px]">
        <div className="min-w-0 space-y-5">
          <Card className="h-[360px] gap-0 overflow-hidden py-0 sm:h-[420px]">
            <CardHeader className="shrink-0 border-b py-4">
              <CardTitle>Issue 定义</CardTitle>
            </CardHeader>
            <CardContent className="min-h-0 flex-1 p-0">
              <ScrollArea className="h-full">
                <div className="flex flex-col gap-5 p-6 text-sm">
                  <Block
                    label="描述"
                    value={issue.description || issue.context || "未填写"}
                  />
                  <Block label="目标" value={issue.objective || "未填写"} />
                  <Block
                    label="执行边界"
                    value={issue.constraints || "未填写"}
                  />
                  {issue.result && (
                    <Block label="最新结果" value={issue.result} />
                  )}
                  {issue.error && (
                    <p className="rounded-lg bg-destructive/10 p-3 text-destructive">
                      {issue.error}
                    </p>
                  )}
                </div>
              </ScrollArea>
            </CardContent>
          </Card>
          {detail.validations.length > 0 && (
            <Card>
              <CardHeader>
                <CardTitle className="flex items-center gap-2">
                  <ShieldCheck className="size-4" />
                  目标验收
                </CardTitle>
                <CardDescription>
                  同一个固定验收会话中的逐轮记录；验收者可以看到此前结论与反馈。
                </CardDescription>
              </CardHeader>
              <CardContent>
                <ValidationHistory validations={detail.validations} />
              </CardContent>
            </Card>
          )}
          {detail.children.length > 0 && (
            <Card className="overflow-hidden p-0">
              <CardHeader>
                <CardTitle className="flex items-center gap-2">
                  <GitBranch className="size-4" />子 Issue 树
                </CardTitle>
                <CardDescription>
                  可展开任意深度；进度按直属子项计算。共{" "}
                  {detail.decompositions.length} 次 Agent 拆解。
                </CardDescription>
              </CardHeader>
              <IssueTree
                key={issue.id}
                issues={state?.issues ?? detail.children}
                relations={state?.relations ?? []}
                agents={state?.agents ?? []}
                executions={state?.executions ?? detail.executions}
                rootIds={detail.children.map((child) => child.id)}
              />
            </Card>
          )}
          <Card>
            <CardHeader>
              <CardTitle>协作记录</CardTitle>
              <CardDescription>
                发表评论会唤醒当前负责人；结构化 Agent mention
                会同时唤醒其他协作者。
              </CardDescription>
            </CardHeader>
            <CardContent>
              <Tabs defaultValue="comments">
                <TabsList>
                  <TabsTrigger value="comments">
                    评论 {detail.commentsPage.total}
                  </TabsTrigger>
                  <TabsTrigger value="events">
                    事件 {detail.eventsPage.total}
                  </TabsTrigger>
                  <TabsTrigger value="broadcasts">
                    广播 {detail.broadcasts.length}
                  </TabsTrigger>
                </TabsList>
                <TabsContent value="comments" className="pt-3">
                  <div className="flex h-[520px] flex-col gap-3 sm:h-[560px]">
                    <IssueCommentsList
                      comments={detail.comments}
                      hasMore={detail.commentsPage.hasMore}
                      loadingMore={loadingMore === "comments"}
                      onLoadMore={() => void loadOlder("comments")}
                      onCopy={(value) => void copyComment(value)}
                    />
                    <form
                      onSubmit={comment}
                      className="flex shrink-0 flex-col gap-3"
                    >
                      <Textarea
                        value={body}
                        onChange={(e) => setBody(e.target.value)}
                        rows={4}
                        placeholder="评论会通知当前负责人；也可使用 [@后端工程师](agent://backend-engineer) 邀请其他 Agent…"
                      />
                      <Button type="submit" disabled={busy || !body.trim()}>
                        <Send />
                        发表评论
                      </Button>
                    </form>
                  </div>
                </TabsContent>
                <TabsContent value="events" className="pt-3">
                  <ExecutionEvents
                    events={detail.events}
                    issues={state?.issues ?? []}
                    hasMore={detail.eventsPage.hasMore}
                    loadingMore={loadingMore === "events"}
                    onLoadMore={() => void loadOlder("events")}
                  />
                </TabsContent>
                <TabsContent value="broadcasts" className="pt-3">
                  <BroadcastHistory
                    broadcasts={detail.broadcasts}
                    issues={state?.issues ?? [issue]}
                    agents={state?.agents ?? []}
                  />
                </TabsContent>
              </Tabs>
            </CardContent>
          </Card>
        </div>
        <aside className="space-y-5">
          <Card>
            <CardHeader>
              <CardTitle className="flex items-center gap-2">
                <GitBranch className="size-4" />
                依赖关系
              </CardTitle>
            </CardHeader>
            <CardContent className="space-y-4">
              <Relation title="Blocked by" items={detail.blockedBy} />
              <Relation title="Blocks" items={detail.blocks} />
            </CardContent>
          </Card>
          <Card>
            <CardHeader>
              <CardTitle className="flex items-center gap-2">
                <Users className="size-4" />
                关联 Agent
              </CardTitle>
              <CardDescription>
                参与过当前 Issue 的 Agent；负责人负责最终交付与子任务协调。
              </CardDescription>
            </CardHeader>
            <CardContent className="space-y-3">
              {associatedAgentIds.length ? (
                associatedAgentIds.map((agentId) => {
                  const agent = knownAgents.find(
                    (candidate) => candidate.id === agentId
                  )
                  const bindings = detail.agentSessions.filter(
                    (session) => session.agentId === agentId
                  )
                  const bindingStatus = bindings.some(
                    (session) => session.status === "active"
                  )
                    ? "已关联"
                    : bindings.length
                      ? "历史关联"
                      : "待首次执行"
                  return (
                    <div key={agentId} className="rounded-lg border p-3">
                      <div className="flex items-start justify-between gap-3">
                        <div className="min-w-0">
                          <p className="truncate text-sm font-medium">
                            {agent?.name ?? agentId}
                          </p>
                          <p className="mt-1 truncate text-xs text-muted-foreground">
                            {agentId}
                          </p>
                        </div>
                        <div className="flex shrink-0 flex-wrap justify-end gap-1.5">
                          {agentId === issue.assigneeAgentId ? (
                            <Badge>负责人</Badge>
                          ) : (
                            <Badge variant="secondary">
                              {agent?.internal ? "验收 Agent" : "协作 Agent"}
                            </Badge>
                          )}
                          <Badge variant="outline">{bindingStatus}</Badge>
                        </div>
                      </div>
                      {agent ? (
                        <p className="mt-2 text-xs text-muted-foreground">
                          {agent.category || "未分类"} · {agent.model.provider}{" "}
                          / {agent.model.model}
                        </p>
                      ) : null}
                    </div>
                  )
                })
              ) : (
                <p className="text-sm text-muted-foreground">
                  当前 Issue 尚未关联 Agent。
                </p>
              )}
            </CardContent>
          </Card>
          <Card>
            <CardHeader>
              <CardTitle className="flex items-center gap-2">
                <MessageSquare className="size-4" />
                与运行中 Agent 对话
              </CardTitle>
              <CardDescription>
                {active
                  ? `发送到 ${active.agentId} 的 Pi session。`
                  : "当前没有连接中的执行。"}
              </CardDescription>
            </CardHeader>
            <CardContent>
              <form onSubmit={steer} className="space-y-3">
                <Textarea
                  value={chat}
                  onChange={(e) => setChat(e.target.value)}
                  disabled={!active}
                  rows={4}
                  placeholder="补充要求或调整方向…"
                />
                <Button
                  type="submit"
                  disabled={!active || busy || !chat.trim()}
                >
                  <Send />
                  发送
                </Button>
              </form>
            </CardContent>
          </Card>
        </aside>
      </div>
    </div>
  )
}
async function copyText(value: string) {
  if (typeof navigator !== "undefined" && navigator.clipboard?.writeText) {
    try {
      await navigator.clipboard.writeText(value)
      return
    } catch {
      // Fall back for browsers or embedded webviews that deny Clipboard API.
    }
  }

  const textarea = document.createElement("textarea")
  textarea.value = value
  textarea.setAttribute("readonly", "")
  textarea.style.position = "fixed"
  textarea.style.left = "-9999px"
  document.body.appendChild(textarea)
  textarea.select()
  const copied = document.execCommand("copy")
  textarea.remove()
  if (!copied) throw new Error("copy command was rejected")
}
function Block({ label, value }: { label: string; value: string }) {
  return (
    <div>
      <h3 className="mb-1.5 text-xs font-medium tracking-wide text-muted-foreground uppercase">
        {label}
      </h3>
      <MarkdownContent>{value}</MarkdownContent>
    </div>
  )
}

function ValidationHistory({
  validations,
}: {
  validations: IssueValidation[]
}) {
  return (
    <div className="flex flex-col gap-2">
      {validations.map((validation) => (
        <ValidationHistoryItem key={validation.id} validation={validation} />
      ))}
    </div>
  )
}

function ValidationHistoryItem({
  validation,
}: {
  validation: IssueValidation
}) {
  const [open, setOpen] = React.useState(validation.status === "running")
  const label =
    validation.status === "passed"
      ? "通过"
      : validation.status === "running"
        ? "进行中"
        : validation.status === "abandoned"
          ? "已放弃"
          : validation.status === "skipped"
            ? "已跳过"
            : validation.status === "interrupted"
              ? "等待恢复"
              : validation.status === "failed"
                ? "未通过"
                : "异常"
  const variant =
    validation.status === "passed"
      ? "default"
      : validation.status === "running" ||
          validation.status === "skipped" ||
          validation.status === "interrupted"
        ? "secondary"
        : "destructive"
  return (
    <Collapsible open={open} onOpenChange={setOpen}>
      <div className="rounded-lg border bg-muted/20">
        <CollapsibleTrigger className="flex w-full items-center justify-between gap-3 p-4 text-left">
          <span className="text-sm font-medium">
            第 {validation.attempt} 次验收
          </span>
          <span className="flex items-center gap-2">
            <Badge variant={variant}>{label}</Badge>
            <ChevronDown
              className={cn(
                "size-4 text-muted-foreground transition-transform",
                open && "rotate-180"
              )}
            />
          </span>
        </CollapsibleTrigger>
        <CollapsibleContent>
          <div className="border-t p-4">
            {validation.summary ? (
              <MarkdownContent className="text-sm">
                {validation.summary}
              </MarkdownContent>
            ) : null}
            {validation.feedback ? (
              <div className="mt-3 rounded-md bg-destructive/5 p-3">
                <p className="mb-1 text-xs font-medium text-destructive">
                  续作要求
                </p>
                <MarkdownContent className="text-sm">
                  {validation.feedback}
                </MarkdownContent>
              </div>
            ) : null}
            {validation.abandonmentProof ? (
              <div className="mt-3 rounded-md bg-muted p-3">
                <p className="mb-1 text-xs font-medium">无法达到目标的证明</p>
                <MarkdownContent className="text-sm">
                  {validation.abandonmentProof}
                </MarkdownContent>
              </div>
            ) : null}
            {validation.error ? (
              <p className="mt-3 text-sm text-destructive">
                {validation.error}
              </p>
            ) : null}
            {validation.manualOverrideReason ? (
              <div className="mt-3 rounded-md border border-amber-300/60 bg-amber-50/60 p-3 dark:bg-amber-950/20">
                <p className="mb-1 text-xs font-medium text-amber-800 dark:text-amber-300">
                  用户手动验收基准
                </p>
                <MarkdownContent className="text-sm">
                  {validation.manualOverrideReason}
                </MarkdownContent>
              </div>
            ) : null}
          </div>
        </CollapsibleContent>
      </div>
    </Collapsible>
  )
}

function BroadcastHistory({
  broadcasts,
  issues,
  agents,
}: {
  broadcasts: TaskBroadcast[]
  issues: Issue[]
  agents: AgentDefinition[]
}) {
  if (broadcasts.length === 0) {
    return (
      <Empty className="h-[520px] sm:h-[560px]">
        <EmptyHeader>
          <EmptyTitle>还没有任务广播</EmptyTitle>
          <EmptyDescription>
            Agent 分享对其他 Issues 有价值的发现后，广播会显示在这里。
          </EmptyDescription>
        </EmptyHeader>
      </Empty>
    )
  }
  return (
    <ScrollArea className="h-[520px] pr-3 sm:h-[560px]">
      <div className="flex flex-col gap-3">
        {broadcasts.map((broadcast) => {
          const sourceIssue = issues.find(
            (candidate) => candidate.id === broadcast.sourceIssueId
          )
          const sourceAgent = agents.find(
            (candidate) => candidate.id === broadcast.sourceAgentId
          )
          return (
            <Card key={broadcast.id} size="sm">
              <CardHeader>
                <div className="flex items-start justify-between gap-3">
                  <div className="flex min-w-0 items-start gap-3">
                    <RadioTower className="mt-0.5 size-4 shrink-0 text-muted-foreground" />
                    <div className="flex min-w-0 flex-col gap-1">
                      <CardTitle>{broadcast.subject}</CardTitle>
                      <CardDescription>
                        {sourceAgent?.name ??
                          broadcast.sourceAgentName ??
                          broadcast.sourceAgentId}
                        {sourceIssue ? (
                          <>
                            {" · "}
                            <Link
                              to={`/issues/${sourceIssue.id}`}
                              className="underline-offset-4 hover:underline"
                            >
                              {sourceIssue.identifier}
                            </Link>
                          </>
                        ) : broadcast.sourceIssueIdentifier ? (
                          <> · {broadcast.sourceIssueIdentifier}</>
                        ) : null}
                        {" · "}
                        {formatTime(broadcast.createdAt)}
                      </CardDescription>
                    </div>
                  </div>
                  <div className="flex shrink-0 flex-wrap justify-end gap-1.5">
                    <Badge
                      variant={
                        broadcast.importance === "critical"
                          ? "destructive"
                          : broadcast.importance === "important"
                            ? "secondary"
                            : "outline"
                      }
                    >
                      {broadcast.importance === "critical"
                        ? "关键"
                        : broadcast.importance === "important"
                          ? "重要"
                          : "普通"}
                    </Badge>
                    <Badge variant="outline">
                      已投递 {broadcast.deliveredCount}
                    </Badge>
                  </div>
                </div>
              </CardHeader>
              <CardContent>
                <MarkdownContent>{broadcast.message}</MarkdownContent>
              </CardContent>
            </Card>
          )
        })}
      </div>
    </ScrollArea>
  )
}
function Relation({
  title,
  items,
}: {
  title: string
  items: IssueDetail["blockedBy"]
}) {
  return (
    <div>
      <p className="mb-2 text-xs text-muted-foreground">{title}</p>
      {items.length ? (
        items.map((i) => (
          <Link
            key={i.id}
            to={`/issues/${i.id}`}
            className="mb-2 flex items-center justify-between rounded-lg border px-3 py-2 text-sm hover:bg-muted/40"
          >
            <span>{i.identifier}</span>
            <StatusBadge status={i.status} />
          </Link>
        ))
      ) : (
        <p className="text-sm text-muted-foreground">无</p>
      )}
    </div>
  )
}

function collectTaskIssues(rootID: string, issues: Issue[]) {
  const ids = new Set([rootID])
  let changed = true
  while (changed) {
    changed = false
    for (const issue of issues) {
      if (issue.parentId && ids.has(issue.parentId) && !ids.has(issue.id)) {
        ids.add(issue.id)
        changed = true
      }
    }
  }
  return issues.filter((issue) => ids.has(issue.id))
}

function findTaskRootID(issueID: string, issues: Issue[]) {
  const byID = new Map(issues.map((issue) => [issue.id, issue]))
  let current = byID.get(issueID)
  const seen = new Set<string>()
  while (current?.parentId && !seen.has(current.id)) {
    seen.add(current.id)
    const parent = byID.get(current.parentId)
    if (!parent) break
    current = parent
  }
  return current?.id ?? issueID
}
