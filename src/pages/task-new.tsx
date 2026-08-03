import * as React from "react"
import {
  ArrowLeft,
  Bot,
  Copy,
  Plus,
  Route,
  Send,
  ShieldCheck,
} from "lucide-react"
import { Link, useLocation, useNavigate } from "react-router-dom"
import { toast } from "sonner"

import { PageHeader } from "@/components/page-header"
import { InputAttachmentList } from "@/components/input-attachments"
import { useInputAttachments } from "@/hooks/use-input-attachments"
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import {
  Card,
  CardContent,
  CardDescription,
  CardFooter,
  CardHeader,
  CardTitle,
} from "@/components/ui/card"
import {
  Field,
  FieldContent,
  FieldDescription,
  FieldGroup,
  FieldLabel,
} from "@/components/ui/field"
import { Input } from "@/components/ui/input"
import { Checkbox } from "@/components/ui/checkbox"
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
import { createTask } from "@/lib/api"
import {
  ISSUE_TITLE_MAX_LENGTH,
  issueTitleLength,
  limitIssueTitle,
} from "@/lib/issue-title"
import { useAppState } from "@/lib/state"
import type { CreateIssueInput } from "@/types"

const priorities: CreateIssueInput["priority"][] = ["high", "middle", "low"]

const categoryLabels: Record<string, string> = {
  orchestrator: "调度",
  backend: "后端",
  frontend: "前端",
  design: "设计",
  security: "安全",
  general: "通用",
}

export function TaskNewPage() {
  const { state } = useAppState()
  const navigate = useNavigate()
  const location = useLocation()
  const clone = (location.state as { clone?: Partial<CreateIssueInput> } | null)
    ?.clone
  const [busy, setBusy] = React.useState(false)
  const fileInputRef = React.useRef<HTMLInputElement>(null)
  const attachments = useInputAttachments()
  const [form, setForm] = React.useState<CreateIssueInput>({
    projectId: clone?.projectId ?? state?.projects[0]?.id,
    title: clone?.title ?? "",
    description: clone?.description ?? "",
    objective: clone?.objective ?? "",
    priority: clone?.priority ?? "high",
    assigneeAgentId: clone?.assigneeAgentId,
    containerProfileId: clone?.containerProfileId,
    workMode: "autonomous",
    workspace: state?.config.workspace ?? "",
    context: clone?.context ?? "",
    constraints:
      clone?.constraints ??
      "仅在指定工作目录中操作；避免破坏性命令；完成后运行相关验证。",
    timeBudgetMinutes: clone?.timeBudgetMinutes,
    humanValidationFallback: clone?.humanValidationFallback,
  })

  const enabledAgentTypes = (state?.agents ?? []).filter(
    (agent) =>
      agent.enabled && !agent.internal && agent.category !== "concierge"
  )
  const enabledContainerProfiles = (state?.containerProfiles ?? []).filter(
    (profile) => profile.enabled
  )
  const defaultContainerProfile =
    enabledContainerProfiles.find(
      (profile) => profile.name.toLowerCase() === "default"
    ) ?? enabledContainerProfiles[0]
  const defaultAgent =
    enabledAgentTypes.find((agent) => agent.id === "aegis-orchestrator") ??
    enabledAgentTypes[0]
  const selectedAgentId = form.assigneeAgentId || defaultAgent?.id || ""
  const selectedAgent = enabledAgentTypes.find(
    (agent) => agent.id === selectedAgentId
  )
  const agentItems = enabledAgentTypes.map((agent) => ({
    label: `${agent.name} · ${categoryLabels[agent.category] ?? agent.category}`,
    value: agent.id,
  }))
  const priorityItems = priorities.map((priority) => ({
    label: priority,
    value: priority,
  }))

  const update = <K extends keyof CreateIssueInput>(
    key: K,
    value: CreateIssueInput[K]
  ) => setForm((current) => ({ ...current, [key]: value }))

  const submit = async (event: React.FormEvent) => {
    event.preventDefault()
    if (!selectedAgentId) {
      toast.error("请选择一个 Agent 类型")
      return
    }
    if (!defaultContainerProfile) {
      toast.error("没有可用的 Docker 容器配置，请先在容器管理中启用一个配置")
      return
    }
    setBusy(true)
    try {
      const { issue } = await createTask({
        ...form,
        assigneeAgentId: selectedAgentId,
        containerProfileId: defaultContainerProfile.id,
        attachmentIds: attachments.attachmentIds,
      })
      attachments.clearBound()
      toast.success(`任务已交给 ${selectedAgent?.name ?? selectedAgentId}`, {
        description: issue.identifier,
      })
      navigate(`/tasks/${issue.id}`)
    } catch (reason) {
      toast.error(reason instanceof Error ? reason.message : "发布失败")
    } finally {
      setBusy(false)
    }
  }

  return (
    <div className="flex flex-col gap-7">
      <PageHeader
        eyebrow="New task"
        title="发布任务"
        description="选择根 Agent 类型。发布后系统会创建任务内临时身份、独立会话和独立手机。"
        actions={
          <Button
            variant="ghost"
            render={<Link to="/tasks" />}
            nativeButton={false}
          >
            <ArrowLeft data-icon="inline-start" />
            返回
          </Button>
        }
      />

      {clone ? (
        <Alert>
          <Copy />
          <AlertTitle>已复制旧任务配置</AlertTitle>
          <AlertDescription>
            标题、目标和负责人等信息已带入；修改后发布会创建一个全新的 Docker
            任务，不会影响原任务。
          </AlertDescription>
        </Alert>
      ) : null}

      <form
        onSubmit={submit}
        className="grid items-start gap-5 xl:grid-cols-[minmax(0,1fr)_360px]"
      >
        <Card>
          <CardHeader>
            <CardTitle>任务定义</CardTitle>
            <CardDescription>
              Agent 将直接收到这些信息，并在真实工作目录中执行。
            </CardDescription>
          </CardHeader>
          <CardContent>
            <FieldGroup>
              <Field>
                <FieldLabel htmlFor="title">任务标题</FieldLabel>
                <Textarea
                  id="title"
                  required
                  rows={4}
                  value={form.title}
                  onChange={(event) =>
                    update("title", limitIssueTitle(event.target.value))
                  }
                  placeholder="描述要交付的最终结果…"
                />
                <FieldDescription className="flex justify-between gap-4">
                  <span>
                    用一句话概括任务；需要自动验收时，再填写下方目标。
                  </span>
                  <span className="shrink-0 tabular-nums">
                    {issueTitleLength(form.title)} / {ISSUE_TITLE_MAX_LENGTH}
                  </span>
                </FieldDescription>
              </Field>
              <Field>
                <FieldLabel htmlFor="context">背景与上下文</FieldLabel>
                <Textarea
                  id="context"
                  rows={5}
                  value={form.context}
                  onChange={(event) => update("context", event.target.value)}
                  placeholder="产品背景、参考路径、技术约束…"
                />
              </Field>
              <Field>
                <FieldLabel htmlFor="description">任务说明（可选）</FieldLabel>
                <Textarea
                  id="description"
                  rows={4}
                  value={form.description}
                  onChange={(event) =>
                    update("description", event.target.value)
                  }
                  placeholder="补充交付范围、业务规则或其他执行说明…"
                />
              </Field>
              <Field>
                <FieldLabel htmlFor="objective">目标（可选）</FieldLabel>
                <Textarea
                  id="objective"
                  rows={5}
                  value={form.objective}
                  onChange={(event) => update("objective", event.target.value)}
                  placeholder="描述最终需要达成的结果，以及可用于判断完成的证据…"
                />
                <FieldDescription>
                  填写后，Worker 结束时会由验收 Agent
                  对比实际产出；留空则不启动验收流程，产出会直接完成。
                </FieldDescription>
              </Field>
              <Field>
                <FieldLabel>审计附件</FieldLabel>
                <FieldDescription>
                  文件会直接流式上传到 Aegis
                  服务端。任务启动前，系统会把它放入任务的独立容器工作区，并通过系统提示告知准确路径；单个文件最大
                  20 GiB。
                </FieldDescription>
                <input
                  ref={fileInputRef}
                  type="file"
                  multiple
                  className="hidden"
                  onChange={(event) => {
                    if (event.target.files) {
                      attachments.addFiles(event.target.files)
                    }
                    event.target.value = ""
                  }}
                />
                <Button
                  type="button"
                  variant="outline"
                  className="w-fit"
                  onClick={() => fileInputRef.current?.click()}
                >
                  <Plus data-icon="inline-start" />
                  添加附件
                </Button>
                <InputAttachmentList
                  items={attachments.items}
                  onRemove={(clientId) => void attachments.remove(clientId)}
                  className="flex-wrap overflow-visible"
                />
                {attachments.hasErrors ? (
                  <FieldDescription className="text-destructive">
                    请移除上传失败的附件后再发布任务。
                  </FieldDescription>
                ) : null}
              </Field>
              <Field>
                <FieldLabel>执行环境</FieldLabel>
                <div className="flex items-center gap-3 rounded-xl border bg-muted/25 px-4 py-3">
                  <Bot className="size-5 shrink-0 text-primary" />
                  <div className="min-w-0">
                    <p className="text-sm font-medium">Docker 隔离执行</p>
                    <p className="truncate text-xs text-muted-foreground">
                      {defaultContainerProfile
                        ? `${defaultContainerProfile.name} · ${defaultContainerProfile.image}`
                        : "尚未配置可用容器环境"}
                    </p>
                  </div>
                </div>
                <FieldDescription>
                  发布任务不再允许使用
                  Host。发布时会立即为任务创建并绑定唯一容器，首次执行时自动启动；每个
                  Issue 在该容器中使用独立工作目录。
                </FieldDescription>
              </Field>
              <Field>
                <FieldLabel htmlFor="constraints">权限与执行边界</FieldLabel>
                <Textarea
                  id="constraints"
                  rows={3}
                  value={form.constraints}
                  onChange={(event) =>
                    update("constraints", event.target.value)
                  }
                />
              </Field>
            </FieldGroup>
          </CardContent>
          <CardFooter className="justify-end">
            <Button
              type="submit"
              size="lg"
              disabled={
                busy ||
                attachments.uploading ||
                attachments.hasErrors ||
                !selectedAgentId ||
                !defaultContainerProfile ||
                !form.title.trim()
              }
            >
              {busy ? (
                <Spinner data-icon="inline-start" />
              ) : (
                <Send data-icon="inline-start" />
              )}
              发布并执行
            </Button>
          </CardFooter>
        </Card>

        <div className="flex flex-col gap-5 xl:sticky xl:top-20">
          <Card>
            <CardHeader>
              <CardTitle>根 Agent</CardTitle>
              <CardDescription>
                这里选择可复用的 Agent 类型，不是在全局员工名册中挑选某个人。
              </CardDescription>
            </CardHeader>
            <CardContent>
              <FieldGroup>
                <Field
                  data-invalid={enabledAgentTypes.length === 0 || undefined}
                >
                  <FieldLabel htmlFor="task-agent">根 Agent 类型</FieldLabel>
                  <Select
                    items={agentItems}
                    value={selectedAgentId}
                    onValueChange={(value) =>
                      update("assigneeAgentId", value || undefined)
                    }
                  >
                    <SelectTrigger
                      id="task-agent"
                      className="w-full"
                      aria-invalid={enabledAgentTypes.length === 0 || undefined}
                    >
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent alignItemWithTrigger={false}>
                      {enabledAgentTypes.map((agent) => (
                        <SelectItem key={agent.id} value={agent.id}>
                          {agent.name} ·{" "}
                          {categoryLabels[agent.category] ?? agent.category}
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                  <FieldDescription>
                    {enabledAgentTypes.length === 0
                      ? "当前没有已启用的 Agent 类型。"
                      : selectedAgent?.description ||
                        "发布后系统会为根 Issue 创建任务内临时身份、独立会话和独立手机。"}
                  </FieldDescription>
                </Field>
                <Field orientation="horizontal">
                  <Checkbox
                    id="task-human-validation"
                    checked={form.humanValidationFallback ?? false}
                    onCheckedChange={(checked) =>
                      update("humanValidationFallback", checked === true)
                    }
                  />
                  <FieldContent>
                    <FieldLabel htmlFor="task-human-validation">
                      启用人工兜底验收
                    </FieldLabel>
                    <FieldDescription>
                      关闭时，固定验收次数耗尽会直接失败并释放依赖；默认关闭。
                    </FieldDescription>
                  </FieldContent>
                </Field>
                {selectedAgent ? (
                  <Field orientation="horizontal" className="flex-wrap gap-2">
                    <Badge variant="secondary">
                      <Bot />
                      {categoryLabels[selectedAgent.category] ??
                        selectedAgent.category}
                    </Badge>
                    <Badge variant="outline">
                      {selectedAgent.skillIds.length} 个 Skills
                    </Badge>
                    <Badge variant="outline">独立 Phone</Badge>
                  </Field>
                ) : null}
              </FieldGroup>
            </CardContent>
          </Card>

          <Card>
            <CardHeader>
              <CardTitle>执行与协作</CardTitle>
            </CardHeader>
            <CardContent>
              <FieldGroup>
                <Field>
                  <FieldLabel>Board Autonomy</FieldLabel>
                  <div className="rounded-md border bg-muted/25 px-3 py-3 text-sm">
                    <div className="flex items-center gap-2 font-medium">
                      <Route className="size-4 text-primary" />
                      唯一协作模式
                    </div>
                  </div>
                  <FieldDescription>
                    根 Agent 可创建并分派子
                    Issue，同时继续工作；一分钟心跳会汇总进度，评论与手机消息可直接引导或唤醒运行中的
                    Agent。
                  </FieldDescription>
                </Field>
                <Field>
                  <FieldLabel htmlFor="task-priority">优先级</FieldLabel>
                  <Select
                    items={priorityItems}
                    value={form.priority}
                    onValueChange={(value) =>
                      update("priority", value as CreateIssueInput["priority"])
                    }
                  >
                    <SelectTrigger id="task-priority" className="w-full">
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      <SelectGroup>
                        {priorities.map((priority) => (
                          <SelectItem key={priority} value={priority}>
                            {priority}
                          </SelectItem>
                        ))}
                      </SelectGroup>
                    </SelectContent>
                  </Select>
                </Field>
                <Field>
                  <FieldLabel htmlFor="task-time-budget">
                    时间预算（分钟）
                  </FieldLabel>
                  <Input
                    id="task-time-budget"
                    type="number"
                    min={1}
                    step={1}
                    value={form.timeBudgetMinutes ?? ""}
                    onChange={(event) => {
                      const value = event.target.value
                      update(
                        "timeBudgetMinutes",
                        value
                          ? Math.max(1, Number.parseInt(value, 10))
                          : undefined
                      )
                    }}
                    placeholder="留空表示不限制"
                  />
                  <FieldDescription>
                    整个 Task 从创建时开始计算；等待、休眠及重新执行子 Issue
                    均不会重置。子 Issue 的单次 Execution 预算在设置中单独配置。
                  </FieldDescription>
                </Field>
              </FieldGroup>
            </CardContent>
          </Card>

          <Alert>
            <Route />
            <AlertTitle>动态 Issue 树</AlertTitle>
            <AlertDescription>
              根 Agent 发现任务过大时，可通过 Board 创建并指派子
              Issues。每个受派实例会领取新的临时名、会话和 Phone，父 Agent
              同时继续推进集成工作。
            </AlertDescription>
          </Alert>
          <Alert>
            <ShieldCheck />
            <AlertTitle>真实执行</AlertTitle>
            <AlertDescription>
              发布后会启动 AgentCore
              execution，并通过任务容器修改文件、调用工具和产生模型费用。
            </AlertDescription>
          </Alert>
        </div>
      </form>
    </div>
  )
}
