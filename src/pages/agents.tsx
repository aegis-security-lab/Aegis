import * as React from "react"
import {
  Bot,
  BrainCircuit,
  Code2,
  Database,
  Pencil,
  Plus,
  Route,
  Server,
  Shield,
  ShieldCheck,
  Trash2,
  Wrench,
} from "lucide-react"
import { toast } from "sonner"

import { PageHeader } from "@/components/page-header"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import {
  Card,
  CardAction,
  CardContent,
  CardDescription,
  CardFooter,
  CardHeader,
  CardTitle,
} from "@/components/ui/card"
import { Checkbox } from "@/components/ui/checkbox"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import {
  Empty,
  EmptyDescription,
  EmptyHeader,
  EmptyMedia,
  EmptyTitle,
} from "@/components/ui/empty"
import {
  Field,
  FieldDescription,
  FieldGroup,
  FieldLabel,
  FieldLegend,
  FieldSet,
  FieldTitle,
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
import { Separator } from "@/components/ui/separator"
import { Spinner } from "@/components/ui/spinner"
import { Switch } from "@/components/ui/switch"
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs"
import { Textarea } from "@/components/ui/textarea"
import {
  createAgent,
  deleteAgent,
  updateAgent,
  type SaveAgentInput,
} from "@/lib/api"
import { useAppState } from "@/lib/state"
import type { AgentDefinition, PermissionBoundary } from "@/types"

const allTools = [
  "read",
  "grep",
  "find",
  "ls",
  "bash",
  "edit",
  "write",
  "aegis_create_subissues",
]
const requiredAgentTools = new Set(["aegis_create_subissues"])

const categoryLabels: Record<string, string> = {
  orchestrator: "调度",
  backend: "后端",
  frontend: "前端",
  security: "安全",
  general: "通用",
}

export function AgentsPage() {
  const { state, refresh } = useAppState()
  const [editing, setEditing] = React.useState<AgentDefinition | "new" | null>(
    null
  )
  const agents = state?.agents ?? []
  const skills = state?.skills ?? []
  const enabled = agents.filter((agent) => agent.enabled).length
  const customModels = agents.filter(
    (agent) => agent.model.provider || agent.model.model
  ).length

  return (
    <div className="flex flex-col gap-7">
      <PageHeader
        eyebrow="Agent definitions"
        title="Agents"
        description="管理可复用的 Agent 定义。Agent 是模型、系统提示词、工具、Skills 与权限边界的组合，不是运行会话。"
        actions={
          <Button onClick={() => setEditing("new")}>
            <Plus data-icon="inline-start" />
            新增 Agent
          </Button>
        }
      />

      <div className="grid gap-4 sm:grid-cols-3">
        <Summary icon={Bot} label="Agent 定义" value={agents.length} />
        <Summary icon={ShieldCheck} label="已启用" value={enabled} />
        <Summary
          icon={BrainCircuit}
          label="独立模型覆盖"
          value={customModels}
        />
      </div>

      {agents.length === 0 ? (
        <Card>
          <CardContent className="py-16">
            <Empty>
              <EmptyHeader>
                <EmptyMedia variant="icon">
                  <Bot />
                </EmptyMedia>
                <EmptyTitle>还没有 Agent 定义</EmptyTitle>
                <EmptyDescription>
                  创建一个 Agent，并配置提示词、工具、Skills 和权限边界。
                </EmptyDescription>
              </EmptyHeader>
            </Empty>
          </CardContent>
        </Card>
      ) : (
        <div className="grid gap-5 lg:grid-cols-2">
          {agents.map((agent) => (
            <AgentCard
              key={agent.id}
              agent={agent}
              skillNames={agent.skillIds.map(
                (id) =>
                  skills.find((skill) => skill.id === id)?.displayName ?? id
              )}
              globalProvider={state?.config.provider ?? "—"}
              globalModel={state?.config.model ?? "—"}
              onEdit={() => setEditing(agent)}
            />
          ))}
        </div>
      )}

      {editing ? (
        <AgentDialog
          key={editing === "new" ? "new" : editing.id}
          agent={editing}
          skills={skills}
          globalProvider={state?.config.provider ?? ""}
          globalModel={state?.config.model ?? ""}
          onOpenChange={(open) => {
            if (!open) setEditing(null)
          }}
          onSaved={async () => {
            setEditing(null)
            await refresh()
          }}
        />
      ) : null}
    </div>
  )
}

function AgentCard({
  agent,
  skillNames,
  globalProvider,
  globalModel,
  onEdit,
}: {
  agent: AgentDefinition
  skillNames: string[]
  globalProvider: string
  globalModel: string
  onEdit: () => void
}) {
  const provider = agent.model.provider || globalProvider
  const model = agent.model.model || globalModel
  return (
    <Card>
      <CardHeader>
        <div className="flex min-w-0 items-start gap-3">
          <span className="flex size-10 shrink-0 items-center justify-center rounded-xl bg-muted text-foreground">
            {categoryGlyph(agent.category)}
          </span>
          <div className="min-w-0">
            <CardTitle className="truncate">{agent.name}</CardTitle>
            <CardDescription className="mt-1 line-clamp-2">
              {agent.description}
            </CardDescription>
          </div>
        </div>
        <CardAction className="flex items-center gap-2">
          {agent.builtin ? <Badge variant="outline">内置</Badge> : null}
          <Badge variant={agent.enabled ? "default" : "secondary"}>
            {agent.enabled ? "启用" : "停用"}
          </Badge>
        </CardAction>
      </CardHeader>
      <CardContent className="flex flex-col gap-4">
        <div className="grid gap-3 sm:grid-cols-2">
          <Metadata label="模型" value={`${provider} / ${model}`} />
          <Metadata
            label="配置来源"
            value={
              agent.model.provider || agent.model.model
                ? "Agent 覆盖"
                : "继承全局"
            }
          />
          <Metadata label="工具" value={`${agent.tools.length} 个`} />
          <Metadata label="Skills" value={`${agent.skillIds.length} 个`} />
        </div>
        <Separator />
        <div className="flex flex-wrap gap-2">
          <Badge variant="secondary">
            {categoryLabels[agent.category] ?? agent.category}
          </Badge>
          <BoundaryBadge allowed={agent.permissions.allowWrite} label="写入" />
          <BoundaryBadge allowed={agent.permissions.allowShell} label="Shell" />
          <BoundaryBadge
            allowed={agent.permissions.allowNetwork}
            label="网络"
          />
          <Badge variant="outline">仅任务工作区</Badge>
        </div>
        {skillNames.length > 0 ? (
          <div className="flex flex-wrap gap-2">
            {skillNames.map((name) => (
              <Badge key={name} variant="outline">
                {name}
              </Badge>
            ))}
          </div>
        ) : null}
      </CardContent>
      <CardFooter className="justify-between gap-3">
        <span className="truncate font-mono text-xs text-muted-foreground">
          {agent.id}
        </span>
        <Button variant="outline" size="sm" onClick={onEdit}>
          <Pencil data-icon="inline-start" />
          配置
        </Button>
      </CardFooter>
    </Card>
  )
}

function AgentDialog({
  agent,
  skills,
  globalProvider,
  globalModel,
  onOpenChange,
  onSaved,
}: {
  agent: AgentDefinition | "new"
  skills: { id: string; displayName: string; description: string }[]
  globalProvider: string
  globalModel: string
  onOpenChange: (open: boolean) => void
  onSaved: () => Promise<void>
}) {
  const [form, setForm] = React.useState<SaveAgentInput>(() =>
    agent === "new" ? blankAgent() : agentInput(agent)
  )
  const [saving, setSaving] = React.useState(false)
  const [removing, setRemoving] = React.useState(false)

  const set = <K extends keyof SaveAgentInput>(
    key: K,
    value: SaveAgentInput[K]
  ) => setForm((current) => ({ ...current, [key]: value }))

  const submit = async (event: React.FormEvent) => {
    event.preventDefault()
    if (!form.name.trim() || !form.systemPrompt.trim() || saving) return
    setSaving(true)
    try {
      if (agent === "new") await createAgent(form)
      else await updateAgent(agent.id, form)
      toast.success(agent === "new" ? "Agent 已创建" : "Agent 配置已保存")
      await onSaved()
    } catch (reason) {
      toast.error(reason instanceof Error ? reason.message : "保存 Agent 失败")
    } finally {
      setSaving(false)
    }
  }

  const remove = async () => {
    if (agent === "new" || agent.builtin || removing) return
    setRemoving(true)
    try {
      await deleteAgent(agent.id)
      toast.success("Agent 已删除")
      await onSaved()
    } catch (reason) {
      toast.error(reason instanceof Error ? reason.message : "删除 Agent 失败")
    } finally {
      setRemoving(false)
    }
  }

  const toggleTool = (tool: string, checked: boolean) => {
    if (requiredAgentTools.has(tool)) return
    set(
      "tools",
      checked
        ? [...new Set([...form.tools, tool])]
        : form.tools.filter((item) => item !== tool)
    )
  }

  const toggleSkill = (skillId: string, checked: boolean) =>
    set(
      "skillIds",
      checked
        ? [...new Set([...form.skillIds, skillId])]
        : form.skillIds.filter((item) => item !== skillId)
    )

  const updatePermission = <K extends keyof PermissionBoundary>(
    key: K,
    value: PermissionBoundary[K]
  ) => set("permissions", { ...form.permissions, [key]: value })

  return (
    <Dialog open onOpenChange={onOpenChange}>
      <DialogContent className="max-h-[92svh] overflow-y-auto sm:max-w-4xl">
        <form onSubmit={submit} className="contents">
          <DialogHeader>
            <DialogTitle>
              {agent === "new" ? "新增 Agent" : `配置 ${form.name}`}
            </DialogTitle>
            <DialogDescription>
              每项配置都属于这个 Agent；空模型字段会在启动 session
              时继承全局设置。
            </DialogDescription>
          </DialogHeader>

          <Tabs defaultValue="identity" className="min-h-0">
            <TabsList
              className="w-full justify-start overflow-x-auto"
              variant="line"
            >
              <TabsTrigger value="identity">身份与模型</TabsTrigger>
              <TabsTrigger value="prompt">系统提示词</TabsTrigger>
              <TabsTrigger value="capabilities">工具与 Skills</TabsTrigger>
              <TabsTrigger value="permissions">权限边界</TabsTrigger>
            </TabsList>

            <TabsContent value="identity" className="pt-4">
              <FieldGroup>
                <div className="grid gap-5 sm:grid-cols-2">
                  <Field>
                    <FieldLabel htmlFor="agent-id">Agent ID</FieldLabel>
                    <Input
                      id="agent-id"
                      value={form.id}
                      disabled={agent !== "new"}
                      placeholder="my-specialist"
                      onChange={(event) => set("id", event.target.value)}
                    />
                  </Field>
                  <Field>
                    <FieldLabel htmlFor="agent-name">名称</FieldLabel>
                    <Input
                      id="agent-name"
                      value={form.name}
                      onChange={(event) => set("name", event.target.value)}
                    />
                  </Field>
                  <Field>
                    <FieldLabel htmlFor="agent-category">类别</FieldLabel>
                    <Select
                      value={form.category}
                      onValueChange={(value) => set("category", String(value))}
                    >
                      <SelectTrigger id="agent-category" className="w-full">
                        <SelectValue />
                      </SelectTrigger>
                      <SelectContent>
                        <SelectGroup>
                          <SelectItem value="general">通用</SelectItem>
                          <SelectItem value="orchestrator">调度</SelectItem>
                          <SelectItem value="backend">后端</SelectItem>
                          <SelectItem value="frontend">前端</SelectItem>
                          <SelectItem value="security">安全</SelectItem>
                        </SelectGroup>
                      </SelectContent>
                    </Select>
                  </Field>
                  <Field
                    orientation="horizontal"
                    className="rounded-lg border p-3"
                  >
                    <FieldLabel htmlFor="agent-enabled" className="flex-1">
                      <span>启用 Agent</span>
                      <FieldDescription>
                        停用后不会被新任务分配。
                      </FieldDescription>
                    </FieldLabel>
                    <Switch
                      id="agent-enabled"
                      checked={form.enabled}
                      onCheckedChange={(checked) => set("enabled", checked)}
                    />
                  </Field>
                </div>
                <Field>
                  <FieldLabel htmlFor="agent-description">职责说明</FieldLabel>
                  <Textarea
                    id="agent-description"
                    rows={3}
                    value={form.description}
                    onChange={(event) => set("description", event.target.value)}
                  />
                </Field>
                <FieldSet>
                  <FieldLegend>模型覆盖</FieldLegend>
                  <FieldDescription>
                    留空即继承全局：{globalProvider || "—"} /{" "}
                    {globalModel || "—"}
                  </FieldDescription>
                  <div className="grid gap-5 sm:grid-cols-2">
                    <Field>
                      <FieldLabel htmlFor="agent-provider">Provider</FieldLabel>
                      <Input
                        id="agent-provider"
                        value={form.model.provider}
                        placeholder={globalProvider || "继承全局"}
                        onChange={(event) =>
                          set("model", {
                            ...form.model,
                            provider: event.target.value,
                          })
                        }
                      />
                    </Field>
                    <Field>
                      <FieldLabel htmlFor="agent-model">Model</FieldLabel>
                      <Input
                        id="agent-model"
                        value={form.model.model}
                        placeholder={globalModel || "继承全局"}
                        onChange={(event) =>
                          set("model", {
                            ...form.model,
                            model: event.target.value,
                          })
                        }
                      />
                    </Field>
                    <Field>
                      <FieldLabel htmlFor="agent-base-url">Base URL</FieldLabel>
                      <Input
                        id="agent-base-url"
                        value={form.model.baseUrl}
                        placeholder="继承全局"
                        onChange={(event) =>
                          set("model", {
                            ...form.model,
                            baseUrl: event.target.value,
                          })
                        }
                      />
                    </Field>
                    <Field>
                      <FieldLabel htmlFor="agent-thinking">Thinking</FieldLabel>
                      <Select
                        value={form.model.thinking || "inherit"}
                        onValueChange={(value) =>
                          set("model", {
                            ...form.model,
                            thinking: value === "inherit" ? "" : String(value),
                          })
                        }
                      >
                        <SelectTrigger id="agent-thinking" className="w-full">
                          <SelectValue />
                        </SelectTrigger>
                        <SelectContent>
                          <SelectGroup>
                            <SelectItem value="inherit">继承全局</SelectItem>
                            <SelectItem value="low">low</SelectItem>
                            <SelectItem value="medium">medium</SelectItem>
                            <SelectItem value="high">high</SelectItem>
                          </SelectGroup>
                        </SelectContent>
                      </Select>
                    </Field>
                  </div>
                </FieldSet>
              </FieldGroup>
            </TabsContent>

            <TabsContent value="prompt" className="pt-4">
              <FieldGroup>
                <Field>
                  <FieldLabel htmlFor="agent-system-prompt">
                    系统提示词
                  </FieldLabel>
                  <Textarea
                    id="agent-system-prompt"
                    className="min-h-96 font-mono text-xs leading-5"
                    value={form.systemPrompt}
                    onChange={(event) =>
                      set("systemPrompt", event.target.value)
                    }
                  />
                  <FieldDescription>
                    启动 Pi RPC 时通过独立的 system prompt
                    注入，不与任务提示词混合保存。
                  </FieldDescription>
                </Field>
              </FieldGroup>
            </TabsContent>

            <TabsContent value="capabilities" className="pt-4">
              <FieldGroup>
                <FieldSet>
                  <FieldLegend>工具集</FieldLegend>
                  <FieldDescription>
                    每个 Agent 保存独立工具数组；Issue
                    拆分是所有 Agent 必备的控制面能力。
                  </FieldDescription>
                  <div
                    data-slot="checkbox-group"
                    className="grid gap-2 sm:grid-cols-2 lg:grid-cols-3"
                  >
                    {allTools.map((tool) => (
                      <Field
                        key={tool}
                        orientation="horizontal"
                        data-disabled={requiredAgentTools.has(tool) || undefined}
                        className="rounded-lg border p-3"
                      >
                        <Checkbox
                          id={`agent-tool-${tool}`}
                          checked={
                            requiredAgentTools.has(tool) ||
                            form.tools.includes(tool)
                          }
                          disabled={requiredAgentTools.has(tool)}
                          onCheckedChange={(checked) =>
                            toggleTool(tool, checked)
                          }
                        />
                        <FieldLabel
                          htmlFor={`agent-tool-${tool}`}
                          className="font-mono font-normal"
                        >
                          {tool}
                          {requiredAgentTools.has(tool) ? "（必备）" : ""}
                        </FieldLabel>
                      </Field>
                    ))}
                  </div>
                </FieldSet>
                <FieldSet>
                  <FieldLegend>Skills / 知识</FieldLegend>
                  <FieldDescription>
                    选中的 SKILL.md 会在创建这个 Agent 的 Pi session
                    时显式加载。
                  </FieldDescription>
                  <div
                    data-slot="checkbox-group"
                    className="grid gap-2 sm:grid-cols-2"
                  >
                    {skills.map((skill) => (
                      <Field
                        key={skill.id}
                        orientation="horizontal"
                        className="rounded-lg border p-3"
                      >
                        <Checkbox
                          id={`agent-skill-${skill.id}`}
                          checked={form.skillIds.includes(skill.id)}
                          onCheckedChange={(checked) =>
                            toggleSkill(skill.id, checked)
                          }
                        />
                        <FieldLabel
                          htmlFor={`agent-skill-${skill.id}`}
                          className="font-normal"
                        >
                          <span>{skill.displayName}</span>
                          <FieldDescription>
                            {skill.description}
                          </FieldDescription>
                        </FieldLabel>
                      </Field>
                    ))}
                  </div>
                </FieldSet>
              </FieldGroup>
            </TabsContent>

            <TabsContent value="permissions" className="pt-4">
              <FieldGroup>
                <Field>
                  <FieldTitle>工作区范围</FieldTitle>
                  <Input value="仅当前任务工作区（run_workspace）" disabled />
                  <FieldDescription>
                    文件工具访问会校验路径；Pi 进程工作目录固定为任务工作区。
                  </FieldDescription>
                </Field>
                <PermissionSwitch
                  id="permission-write"
                  label="允许写入"
                  description="控制 edit、write 以及可识别的 shell 写入命令。"
                  checked={form.permissions.allowWrite}
                  onCheckedChange={(checked) =>
                    updatePermission("allowWrite", checked)
                  }
                />
                <PermissionSwitch
                  id="permission-shell"
                  label="允许 Shell"
                  description="关闭后 bash 工具调用会被 Aegis Guard 拦截。"
                  checked={form.permissions.allowShell}
                  onCheckedChange={(checked) =>
                    updatePermission("allowShell", checked)
                  }
                />
                <PermissionSwitch
                  id="permission-network"
                  label="允许网络访问"
                  description="关闭后 curl、wget、ssh、git fetch、包安装等命令会被拦截。"
                  checked={form.permissions.allowNetwork}
                  onCheckedChange={(checked) =>
                    updatePermission("allowNetwork", checked)
                  }
                />
                <Field>
                  <FieldLabel htmlFor="permission-approval">
                    审批策略
                  </FieldLabel>
                  <Select
                    value={form.permissions.approvalMode || "inherit"}
                    onValueChange={(value) =>
                      updatePermission(
                        "approvalMode",
                        value === "inherit"
                          ? ""
                          : (String(
                              value
                            ) as PermissionBoundary["approvalMode"])
                      )
                    }
                  >
                    <SelectTrigger id="permission-approval" className="w-full">
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      <SelectGroup>
                        <SelectItem value="inherit">继承全局策略</SelectItem>
                        <SelectItem value="all">
                          所有写入与 Shell 都审批
                        </SelectItem>
                        <SelectItem value="risky">仅高风险 Shell</SelectItem>
                        <SelectItem value="none">不审批</SelectItem>
                      </SelectGroup>
                    </SelectContent>
                  </Select>
                </Field>
              </FieldGroup>
            </TabsContent>
          </Tabs>

          <DialogFooter className="justify-between sm:justify-between">
            <div>
              {agent !== "new" && agent && !agent.builtin ? (
                <Button
                  type="button"
                  variant="destructive"
                  disabled={removing || saving}
                  onClick={() => void remove()}
                >
                  {removing ? (
                    <Spinner data-icon="inline-start" />
                  ) : (
                    <Trash2 data-icon="inline-start" />
                  )}
                  删除
                </Button>
              ) : null}
            </div>
            <div className="flex gap-2">
              <Button
                type="button"
                variant="outline"
                onClick={() => onOpenChange(false)}
              >
                取消
              </Button>
              <Button
                type="submit"
                disabled={
                  saving || !form.name.trim() || !form.systemPrompt.trim()
                }
              >
                {saving ? <Spinner data-icon="inline-start" /> : null}
                {saving ? "保存中…" : "保存 Agent"}
              </Button>
            </div>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}

function PermissionSwitch({
  id,
  label,
  description,
  checked,
  onCheckedChange,
}: {
  id: string
  label: string
  description: string
  checked: boolean
  onCheckedChange: (checked: boolean) => void
}) {
  return (
    <Field orientation="horizontal" className="rounded-lg border p-3">
      <FieldLabel htmlFor={id} className="flex-1">
        <span>{label}</span>
        <FieldDescription>{description}</FieldDescription>
      </FieldLabel>
      <Switch id={id} checked={checked} onCheckedChange={onCheckedChange} />
    </Field>
  )
}

function Summary({
  icon: Icon,
  label,
  value,
}: {
  icon: typeof Bot
  label: string
  value: number
}) {
  return (
    <Card>
      <CardContent className="flex items-center gap-4 p-5">
        <span className="flex size-10 items-center justify-center rounded-xl bg-muted">
          <Icon className="size-4" />
        </span>
        <div>
          <p className="text-xs text-muted-foreground">{label}</p>
          <p className="mt-1 text-xl font-semibold tabular-nums">{value}</p>
        </div>
      </CardContent>
    </Card>
  )
}

function Metadata({ label, value }: { label: string; value: string }) {
  return (
    <div>
      <p className="text-xs text-muted-foreground">{label}</p>
      <p className="mt-1 truncate text-sm font-medium">{value}</p>
    </div>
  )
}

function BoundaryBadge({
  allowed,
  label,
}: {
  allowed: boolean
  label: string
}) {
  return (
    <Badge variant={allowed ? "secondary" : "outline"}>
      {label} {allowed ? "允许" : "禁止"}
    </Badge>
  )
}

function categoryGlyph(category: string) {
  const className = "size-5"
  if (category === "orchestrator") return <Route className={className} />
  if (category === "backend") return <Server className={className} />
  if (category === "frontend") return <Code2 className={className} />
  if (category === "security") return <Shield className={className} />
  if (category === "data") return <Database className={className} />
  return <Wrench className={className} />
}

function blankAgent(): SaveAgentInput {
  return {
    id: "",
    name: "",
    description: "",
    avatar: "bot",
    category: "general",
    enabled: true,
    model: { provider: "", model: "", baseUrl: "", thinking: "" },
    systemPrompt: "",
    tools: [...allTools],
    skillIds: [],
    permissions: {
      workspaceScope: "run_workspace",
      allowNetwork: false,
      allowShell: true,
      allowWrite: true,
      approvalMode: "",
    },
  }
}

function agentInput(agent: AgentDefinition): SaveAgentInput {
  return {
    id: agent.id,
    name: agent.name,
    description: agent.description,
    avatar: agent.avatar,
    category: agent.category,
    enabled: agent.enabled,
    model: { ...agent.model },
    systemPrompt: agent.systemPrompt,
    tools: [...agent.tools],
    skillIds: [...agent.skillIds],
    permissions: { ...agent.permissions },
  }
}
