import * as React from "react"
import {
  Copy,
  Send,
} from "lucide-react"
import { useLocation, useNavigate } from "react-router-dom"
import { toast } from "sonner"

import { ChatComposer } from "@/components/chat-composer"
import { useInputAttachments } from "@/hooks/use-input-attachments"
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert"
import { Button } from "@/components/ui/button"
import {
  Card,
  CardContent,
  CardFooter,
  CardHeader,
  CardTitle,
} from "@/components/ui/card"
import {
  Field,
  FieldContent,
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
    constraints: clone?.constraints ?? "",
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
    const description = form.description.trim()
    if (!description) {
      toast.error("请填写任务说明")
      return
    }
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
      const title = form.title.trim() || limitIssueTitle(description)
      const { issue } = await createTask({
        ...form,
        title,
        description,
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
          </CardHeader>
          <CardContent>
            <FieldGroup>
              <Field>
                <FieldLabel htmlFor="description">任务说明</FieldLabel>
                <ChatComposer
                  value={form.description}
                  onValueChange={(value) => update("description", value)}
                  onSend={() => {}}
                  sending={false}
                  showSend={false}
                  hint=""
                  placeholder="描述交付范围、业务规则和其他执行说明…"
                  ariaLabel="任务说明"
                  attachments={attachments}
                />
                <div className="flex items-center justify-end gap-2 pt-1.5">
                  <span className="text-xs text-muted-foreground">
                    负责 Agent
                  </span>
                  <Select
                    items={agentItems}
                    value={selectedAgentId}
                    onValueChange={(value) =>
                      update("assigneeAgentId", value || undefined)
                    }
                  >
                    <SelectTrigger
                      id="task-agent"
                      size="sm"
                      className="w-44"
                      aria-invalid={enabledAgentTypes.length === 0 || undefined}
                    >
                      <SelectValue placeholder="选择 Agent 类型" />
                    </SelectTrigger>
                    <SelectContent alignItemWithTrigger={false}>
                      <SelectGroup>
                        {enabledAgentTypes.map((agent) => (
                          <SelectItem key={agent.id} value={agent.id}>
                            {agent.name} ·{" "}
                            {categoryLabels[agent.category] ?? agent.category}
                          </SelectItem>
                        ))}
                      </SelectGroup>
                    </SelectContent>
                  </Select>
                </div>
              </Field>
              <Field>
                <FieldLabel htmlFor="title">
                  任务标题（可选，留空则取任务说明开头）
                </FieldLabel>
                <Input
                  id="title"
                  value={form.title}
                  onChange={(event) =>
                    update("title", limitIssueTitle(event.target.value))
                  }
                  placeholder="默认使用任务说明的前几个字符…"
                />
                <span className="text-right text-[11px] tabular-nums text-muted-foreground">
                  {issueTitleLength(form.title) || "—"} /{" "}
                  {ISSUE_TITLE_MAX_LENGTH}
                </span>
              </Field>
              <Field>
                <FieldLabel htmlFor="objective">目标（可选）</FieldLabel>
                <Textarea
                  id="objective"
                  rows={3}
                  value={form.objective}
                  onChange={(event) => update("objective", event.target.value)}
                  placeholder="描述最终需要达成的结果，以及可用于判断完成的证据…"
                />
              </Field>
              <Field>
                <FieldLabel htmlFor="constraints">
                  权限与执行边界（可选）
                </FieldLabel>
                <Textarea
                  id="constraints"
                  rows={3}
                  value={form.constraints}
                  onChange={(event) =>
                    update("constraints", event.target.value)
                  }
                  placeholder="留空使用默认边界…"
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
                !form.description.trim() ||
                !selectedAgentId ||
                !defaultContainerProfile
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
              <CardTitle>执行与协作</CardTitle>
            </CardHeader>
            <CardContent>
              <FieldGroup>
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
                  </FieldContent>
                </Field>
              </FieldGroup>
            </CardContent>
          </Card>
        </div>
      </form>
    </div>
  )
}
