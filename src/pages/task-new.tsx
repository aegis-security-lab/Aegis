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
  SelectLabel,
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

const priorities: CreateIssueInput["priority"][] = [
  "critical",
  "high",
  "medium",
  "low",
]

const workModeItems: Array<{
  label: string
  value: CreateIssueInput["workMode"]
}> = [
  { label: "引导模式", value: "guided" },
  { label: "自治模式", value: "autonomous" },
]

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
  const attachments = useInputAttachments("task")
  const [form, setForm] = React.useState<CreateIssueInput>({
    projectId: clone?.projectId ?? state?.projects[0]?.id,
    title: clone?.title ?? "",
    description: clone?.description ?? "",
    objective: clone?.objective ?? "",
    priority: clone?.priority ?? "high",
    assigneeAgentId: clone?.assigneeAgentId,
    containerProfileId: clone?.containerProfileId,
    workMode: clone?.workMode ?? "guided",
    workspace: state?.config.workspace ?? "",
    context: clone?.context ?? "",
    constraints:
      clone?.constraints ??
      "仅在指定工作目录中操作；避免破坏性命令；完成后运行相关验证。",
    timeBudgetMinutes: clone?.timeBudgetMinutes,
    humanValidationFallback: clone?.humanValidationFallback,
  })

  const enabledEmployees = (state?.agents ?? []).filter(
    (agent) =>
      agent.enabled && !agent.internal && agent.category !== "concierge"
  )
  const availability = new Map(
    (state?.employeeAvailability ?? []).map((item) => [item.agentId, item])
  )
  const isAvailable = (agentId: string) =>
    availability.get(agentId)?.available ?? true
  const enabledContainerProfiles = (state?.containerProfiles ?? []).filter(
    (profile) => profile.enabled
  )
  const defaultContainerProfile =
    enabledContainerProfiles.find(
      (profile) => profile.name.toLowerCase() === "default"
    ) ?? enabledContainerProfiles[0]
  const defaultAgent =
    enabledEmployees.find((agent) => agent.id === "aegis-orchestrator") ??
    enabledEmployees[0]
  const selectedAgentId = form.assigneeAgentId || defaultAgent?.id || ""
  const selectedAgent = enabledEmployees.find(
    (agent) => agent.id === selectedAgentId
  )
  const selectedAvailability = availability.get(selectedAgentId)
  const positions = (state?.positions ?? []).filter(
    (position) => position.enabled
  )
  const agentItems = enabledEmployees.map((agent) => ({
    label: `${agent.name} · ${positions.find((position) => position.id === agent.positionId)?.name ?? categoryLabels[agent.category] ?? agent.category}${isAvailable(agent.id) ? "" : " · 其他任务工作中（将新建会话）"}`,
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
      toast.error("请选择一名可用员工")
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
        description="把顶层任务指派给一名具体员工；岗位仅用于分类，不能作为负责人。"
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
              <CardTitle>任务负责人</CardTitle>
              <CardDescription>
                新任务可选择任意已启用员工；正在工作的员工会使用新的任务会话。
              </CardDescription>
            </CardHeader>
            <CardContent>
              <FieldGroup>
                <Field
                  data-invalid={enabledEmployees.length === 0 || undefined}
                >
                  <FieldLabel htmlFor="task-agent">负责人</FieldLabel>
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
                      aria-invalid={enabledEmployees.length === 0 || undefined}
                    >
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent alignItemWithTrigger={false}>
                      {positions.map((position) => {
                        const people = enabledEmployees.filter(
                          (agent) => agent.positionId === position.id
                        )
                        if (people.length === 0) return null
                        return (
                          <SelectGroup key={position.id}>
                            <SelectLabel>
                              {position.name} · {people.length} 人
                            </SelectLabel>
                            {people.map((agent) => {
                              const status = availability.get(agent.id)
                              return (
                                <SelectItem key={agent.id} value={agent.id}>
                                  {agent.name} · {agent.englishName}
                                  {!isAvailable(agent.id)
                                    ? ` · 其他任务工作中${status?.currentIssueIdentifier ? `（${status.currentIssueIdentifier}）` : ""}，将新建会话`
                                    : ""}
                                </SelectItem>
                              )
                            })}
                          </SelectGroup>
                        )
                      })}
                      {enabledEmployees.some((agent) => !agent.positionId) ? (
                        <SelectGroup>
                          <SelectLabel>未分配岗位</SelectLabel>
                          {enabledEmployees
                            .filter((agent) => !agent.positionId)
                            .map((agent) => (
                              <SelectItem key={agent.id} value={agent.id}>
                                {agent.name} ·{" "}
                                {categoryLabels[agent.category] ??
                                  agent.category}
                              </SelectItem>
                            ))}
                        </SelectGroup>
                      ) : null}
                    </SelectContent>
                  </Select>
                  <FieldDescription>
                    {enabledEmployees.length === 0
                      ? "当前没有已启用员工。"
                      : selectedAvailability && !selectedAvailability.available
                        ? `${selectedAgent?.name ?? "该员工"}正在处理 ${selectedAvailability.currentIssueIdentifier ?? "其他任务"}；发布后会为本任务创建独立会话。`
                        : selectedAgent?.description ||
                          "该员工会直接执行任务，并可按组织关系委派直属下属。"}
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
                      {selectedAgent.tools.length} 个工具
                    </Badge>
                    <Badge variant="outline">支持拆分 Issues</Badge>
                  </Field>
                ) : null}
              </FieldGroup>
            </CardContent>
          </Card>

          <Card>
            <CardHeader>
              <CardTitle>执行策略</CardTitle>
            </CardHeader>
            <CardContent>
              <FieldGroup>
                <Field>
                  <FieldLabel htmlFor="task-work-mode">工作模式</FieldLabel>
                  <Select
                    items={workModeItems}
                    value={form.workMode}
                    onValueChange={(value) =>
                      update("workMode", value as CreateIssueInput["workMode"])
                    }
                  >
                    <SelectTrigger id="task-work-mode" className="w-full">
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      <SelectGroup>
                        <SelectItem value="guided">引导模式</SelectItem>
                        <SelectItem value="autonomous">自治模式</SelectItem>
                      </SelectGroup>
                    </SelectContent>
                  </Select>
                  <FieldDescription>
                    {form.workMode === "guided"
                      ? "子 Issue 完成后进入人工复核。"
                      : "依赖满足后自动推进并完成父任务。"}
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
                    placeholder="使用全局配置"
                  />
                  <FieldDescription>
                    留空使用设置中的全局预算；填写后仅本任务根 Issue
                    使用该预算，优先级高于全局时间预算，子 Issue 不重复计时。
                  </FieldDescription>
                </Field>
              </FieldGroup>
            </CardContent>
          </Card>

          <Alert>
            <Route />
            <AlertTitle>动态 Issue 树</AlertTitle>
            <AlertDescription>
              所选 Agent 发现任务过大时，可创建 2–8 个带依赖关系的子
              Issues；调度器随后分配并执行它们。
            </AlertDescription>
          </Alert>
          <Alert>
            <ShieldCheck />
            <AlertTitle>真实执行</AlertTitle>
            <AlertDescription>
              发布后会启动本机 Pi 进程，并可能修改工作目录文件、产生模型费用。
            </AlertDescription>
          </Alert>
        </div>
      </form>
    </div>
  )
}
