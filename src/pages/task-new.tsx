import * as React from "react"
import { ArrowLeft, Bot, Route, Send, ShieldCheck } from "lucide-react"
import { Link, useNavigate } from "react-router-dom"
import { toast } from "sonner"

import { PageHeader } from "@/components/page-header"
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
  FieldDescription,
  FieldGroup,
  FieldLabel,
} from "@/components/ui/field"
import { Input } from "@/components/ui/input"
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
import { createIssue } from "@/lib/api"
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
  security: "安全",
  general: "通用",
}

export function TaskNewPage() {
  const { state } = useAppState()
  const navigate = useNavigate()
  const [busy, setBusy] = React.useState(false)
  const [form, setForm] = React.useState<CreateIssueInput>({
    projectId: state?.projects[0]?.id,
    title: "",
    description: "",
    acceptanceCriteria: "",
    priority: "high",
    workMode: "guided",
    workspace: state?.config.workspace ?? "",
    context: "",
    constraints:
      "仅在指定工作目录中操作；避免破坏性命令；完成后运行相关验证。",
  })

  const enabledAgents = (state?.agents ?? []).filter(
    (agent) => agent.enabled && !agent.internal
  )
  const defaultAgent =
    enabledAgents.find((agent) => agent.id === "aegis-orchestrator") ??
    enabledAgents[0]
  const selectedAgentId = form.assigneeAgentId || defaultAgent?.id || ""
  const selectedAgent = enabledAgents.find(
    (agent) => agent.id === selectedAgentId
  )
  const agentItems = enabledAgents.map((agent) => ({
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
      toast.error("请选择一个可用 Agent")
      return
    }
    setBusy(true)
    try {
      const issue = await createIssue({
        ...form,
        assigneeAgentId: selectedAgentId,
      })
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
        description="选择负责顶层 Issue 的 Agent；任何 Agent 都可以在执行中拆分并调度子 Issues。"
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
                <FieldLabel htmlFor="title">任务目标</FieldLabel>
                <Textarea
                  id="title"
                  required
                  rows={4}
                  value={form.title}
                  onChange={(event) => update("title", event.target.value)}
                  placeholder="描述要交付的最终结果…"
                />
                <FieldDescription>
                  这是顶层 Issue 的标题，也是所选 Agent 的执行目标。
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
                <FieldLabel htmlFor="acceptance">总体验收标准</FieldLabel>
                <Textarea
                  id="acceptance"
                  rows={3}
                  value={form.acceptanceCriteria}
                  onChange={(event) =>
                    update("acceptanceCriteria", event.target.value)
                  }
                  placeholder="完成的明确判定条件…"
                />
              </Field>
              <Field>
                <FieldLabel htmlFor="workspace">工作目录</FieldLabel>
                <Input
                  id="workspace"
                  required
                  value={form.workspace}
                  onChange={(event) => update("workspace", event.target.value)}
                />
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
                !selectedAgentId ||
                !form.title.trim() ||
                !form.workspace.trim()
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
              <CardTitle>执行 Agent</CardTitle>
              <CardDescription>
                选择最适合负责整个顶层任务的 Agent。
              </CardDescription>
            </CardHeader>
            <CardContent>
              <FieldGroup>
                <Field data-invalid={enabledAgents.length === 0 || undefined}>
                  <FieldLabel htmlFor="task-agent">Agent</FieldLabel>
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
                      aria-invalid={enabledAgents.length === 0 || undefined}
                    >
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent alignItemWithTrigger={false}>
                      <SelectGroup>
                        {enabledAgents.map((agent) => (
                          <SelectItem key={agent.id} value={agent.id}>
                            {agent.name} · {categoryLabels[agent.category] ?? agent.category}
                          </SelectItem>
                        ))}
                      </SelectGroup>
                    </SelectContent>
                  </Select>
                  <FieldDescription>
                    {enabledAgents.length === 0
                      ? "当前没有已启用的 Agent，请先到 Agents 页面启用。"
                      : selectedAgent?.description ||
                        "该 Agent 会直接执行任务，并可按复杂度拆分子 Issues。"}
                  </FieldDescription>
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
