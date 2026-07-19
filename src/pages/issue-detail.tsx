import * as React from "react"
/* eslint-disable react-hooks/set-state-in-effect */
import {
  ArrowLeft,
  Clock3,
  Copy,
  Download,
  FileText,
  GitBranch,
  MessageSquare,
  Play,
  RadioTower,
  RotateCcw,
  Send,
  ShieldCheck,
  ShieldOff,
  TerminalSquare,
  XCircle,
} from "lucide-react"
import { Link, useParams } from "react-router-dom"
import { toast } from "sonner"
import { ExecutionEvents } from "@/components/execution-events"
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
import {
  Attachment,
  AttachmentActions,
  AttachmentContent,
  AttachmentDescription,
  AttachmentGroup,
  AttachmentMedia,
  AttachmentTitle,
} from "@/components/ui/attachment"
import { Button, buttonVariants } from "@/components/ui/button"
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
  setIssueValidationDisabled,
  sendChat,
} from "@/lib/api"
import { formatBytes, formatTime, formatTokens } from "@/lib/format"
import { useAppState } from "@/lib/state"
import type {
  AgentDefinition,
  Issue,
  IssueDetail,
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
  const load = React.useCallback(async () => {
    if (!issueId) return
    try {
      setDetail(await fetchIssue(issueId))
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
    const timer = window.setInterval(() => void load(), 2000)
    return () => window.clearInterval(timer)
  }, [hasLiveExecution, load])
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
  const { issue } = detail
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
              <CardContent className="space-y-3">
                {detail.validations.map((validation) => (
                  <div
                    key={validation.id}
                    className="rounded-lg border bg-muted/20 p-4"
                  >
                    <div className="flex flex-wrap items-center justify-between gap-2">
                      <p className="text-sm font-medium">
                        第 {validation.attempt} 次验收
                      </p>
                      <Badge
                        variant={
                          validation.status === "passed"
                            ? "default"
                            : validation.status === "running" ||
                                validation.status === "skipped" ||
                                validation.status === "interrupted"
                              ? "secondary"
                              : "destructive"
                        }
                      >
                        {validation.status === "passed"
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
                                    : "异常"}
                      </Badge>
                    </div>
                    {validation.summary && (
                      <MarkdownContent className="mt-3 text-sm">
                        {validation.summary}
                      </MarkdownContent>
                    )}
                    {validation.feedback && (
                      <div className="mt-3 rounded-md bg-destructive/5 p-3">
                        <p className="mb-1 text-xs font-medium text-destructive">
                          续作要求
                        </p>
                        <MarkdownContent className="text-sm">
                          {validation.feedback}
                        </MarkdownContent>
                      </div>
                    )}
                    {validation.abandonmentProof && (
                      <div className="mt-3 rounded-md bg-muted p-3">
                        <p className="mb-1 text-xs font-medium">
                          无法达到目标的证明
                        </p>
                        <MarkdownContent className="text-sm">
                          {validation.abandonmentProof}
                        </MarkdownContent>
                      </div>
                    )}
                    {validation.error && (
                      <p className="mt-3 text-sm text-destructive">
                        {validation.error}
                      </p>
                    )}
                  </div>
                ))}
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
                    评论 {detail.comments.length}
                  </TabsTrigger>
                  <TabsTrigger value="events">
                    事件 {detail.events.length}
                  </TabsTrigger>
                  <TabsTrigger value="broadcasts">
                    广播 {detail.broadcasts.length}
                  </TabsTrigger>
                </TabsList>
                <TabsContent value="comments" className="pt-3">
                  <div className="flex h-[520px] flex-col gap-3 sm:h-[560px]">
                    <ScrollArea className="min-h-0 flex-1 rounded-xl border">
                      <div className="flex flex-col gap-3 p-3">
                        {detail.comments.length ? (
                          detail.comments.map((c) => (
                            <div key={c.id} className="rounded-xl border p-4">
                              <div className="flex items-center justify-between gap-3">
                                <span className="text-xs font-medium">
                                  {c.authorId}
                                </span>
                                <div className="flex items-center gap-1">
                                  <span className="text-xs text-muted-foreground">
                                    {formatTime(c.createdAt)}
                                  </span>
                                  <Button
                                    type="button"
                                    variant="ghost"
                                    size="icon-sm"
                                    title="复制评论"
                                    aria-label={`复制 ${c.authorId} 的评论`}
                                    onClick={() => void copyComment(c.body)}
                                  >
                                    <Copy data-icon="inline-start" />
                                  </Button>
                                </div>
                              </div>
                              <MarkdownContent className="mt-2 text-sm">
                                {c.body}
                              </MarkdownContent>
                              {c.attachments?.length > 0 && (
                                <AttachmentGroup className="mt-3">
                                  {c.attachments.map((attachment) => (
                                    <Attachment
                                      key={attachment.id}
                                      size="sm"
                                      className="w-72"
                                    >
                                      <AttachmentMedia>
                                        <FileText />
                                      </AttachmentMedia>
                                      <AttachmentContent>
                                        <AttachmentTitle
                                          title={attachment.name}
                                        >
                                          {attachment.name}
                                        </AttachmentTitle>
                                        <AttachmentDescription
                                          title={
                                            attachment.description ||
                                            attachment.sourcePath
                                          }
                                        >
                                          {attachment.description ||
                                            attachment.sourcePath}
                                          {" · "}
                                          {formatBytes(attachment.size)}
                                        </AttachmentDescription>
                                      </AttachmentContent>
                                      <AttachmentActions>
                                        <a
                                          className={buttonVariants({
                                            variant: "ghost",
                                            size: "icon-xs",
                                          })}
                                          aria-label={`下载 ${attachment.name}`}
                                          title="下载附件"
                                          href={`/api/attachments/${encodeURIComponent(attachment.id)}`}
                                          download={attachment.name}
                                        >
                                          <Download data-icon="inline-start" />
                                        </a>
                                      </AttachmentActions>
                                    </Attachment>
                                  ))}
                                </AttachmentGroup>
                              )}
                            </div>
                          ))
                        ) : (
                          <div className="flex min-h-40 items-center justify-center text-sm text-muted-foreground">
                            还没有评论
                          </div>
                        )}
                      </div>
                    </ScrollArea>
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
                  <ScrollArea className="h-[520px] pr-3 sm:h-[560px]">
                    <ExecutionEvents
                      events={detail.events}
                      issues={state?.issues ?? []}
                    />
                  </ScrollArea>
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
                <TerminalSquare className="size-4" />
                Executions
              </CardTitle>
              <CardDescription>
                Worker 运行独立记录；同一 Issue 的所有验收轮次复用一个验收会话。
              </CardDescription>
            </CardHeader>
            <CardContent className="space-y-3">
              {detail.executions.map((e) => (
                <Link
                  key={e.id}
                  to={`/sessions/${e.id}`}
                  className="block rounded-lg border p-3 hover:bg-muted/40"
                >
                  <div className="flex justify-between gap-2">
                    <span className="text-xs font-medium">
                      {e.kind} · {e.agentId}
                    </span>
                    <StatusBadge status={e.status} />
                  </div>
                  <p className="mt-2 font-mono text-xs text-muted-foreground">
                    {e.sessionId}
                  </p>
                  <p className="mt-1 text-xs text-muted-foreground">
                    {formatTokens(e.tokens)} tokens · ${e.cost.toFixed(4)}
                  </p>
                </Link>
              ))}
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
