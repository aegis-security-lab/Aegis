import * as React from "react"
import {
  Bot,
  BrainCircuit,
  Code2,
  Database,
  LibraryBig,
  Palette,
  Pencil,
  Plus,
  Route,
  Server,
  Shield,
  ShieldCheck,
  Sparkles,
  Trash2,
  Wrench,
} from "lucide-react"
import { toast } from "sonner"
import { useSearchParams } from "react-router-dom"

import { PageHeader } from "@/components/page-header"
import { ModelPricingFields } from "@/components/model-pricing-fields"
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
  createAgentTemplate,
  deleteAgent,
  fetchAgentTemplates,
  updateAgent,
  type SaveAgentInput,
} from "@/lib/api"
import { agentTemplateId } from "@/lib/agent-template"
import { useAppState } from "@/lib/state"
import type {
  AgentDefinition,
  AgentTemplate,
  ModelPricing,
  PermissionBoundary,
  Position,
} from "@/types"

const zeroPricing: ModelPricing = {
  input: 0,
  output: 0,
  cacheRead: 0,
  cacheWrite: 0,
}

const allTools = [
  "read",
  "grep",
  "find",
  "ls",
  "bash",
  "edit",
  "write",
  "aegis_create_task",
  "aegis_create_subissues",
  "aegis_board",
  "aegis_relay",
  "aegis_publish_attachment",
  "aegis_report_progress",
  "aegis_get_memo",
  "aegis_update_memo",
  "aegis_request_rework",
  "aegis_uncover_search",
]
const requiredAgentTools = new Set([
  "aegis_create_subissues",
  "aegis_board",
  "aegis_relay",
  "aegis_publish_attachment",
  "aegis_report_progress",
  "aegis_get_memo",
  "aegis_update_memo",
  "aegis_request_rework",
])

const categoryLabels: Record<string, string> = {
  orchestrator: "调度",
  backend: "后端",
  frontend: "前端",
  design: "设计",
  security: "安全",
  knowledge: "检索",
  concierge: "管家",
  general: "通用",
}

export function AgentsPage() {
  const { state, refresh } = useAppState()
  const [searchParams, setSearchParams] = useSearchParams()
  const [templates, setTemplates] = React.useState<AgentTemplate[]>([])
  const [editing, setEditing] = React.useState<AgentDefinition | "new" | null>(
    null
  )
  const agents = React.useMemo(() => state?.agents ?? [], [state?.agents])
  const skills = state?.skills ?? []
  const knowledgeBases = state?.knowledgeBases ?? []
  const enabled = agents.filter((agent) => agent.enabled).length
  const customModels = agents.filter(
    (agent) => agent.model.provider || agent.model.model
  ).length
  const requestedTemplate = templates.find(
    (item) => item.id === searchParams.get("template")
  )
  const requestedAgent = agents.find(
    (item) => item.id === searchParams.get("agent")
  )
  const effectiveEditing =
    editing ?? requestedAgent ?? (requestedTemplate ? "new" : null)
  const editingAgent =
    effectiveEditing === "new"
      ? "new"
      : effectiveEditing
        ? (agents.find((agent) => agent.id === effectiveEditing.id) ??
          effectiveEditing)
        : null
  React.useEffect(() => {
    void fetchAgentTemplates().then(setTemplates)
  }, [])
  return (
    <div className="flex flex-col gap-7">
      <PageHeader
        eyebrow="Agent definitions"
        title="员工管理"
        description="管理员工及其工具、Skills、知识库、权限和备忘录；身份、模型与提示词来自人才模板。"
        actions={
          <Button onClick={() => setEditing("new")}>
            <Plus data-icon="inline-start" />
            新增员工
          </Button>
        }
      />

      <div className="grid gap-4 sm:grid-cols-3">
        <Summary icon={Bot} label="员工数量" value={agents.length} />
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
                <EmptyTitle>还没有员工</EmptyTitle>
                <EmptyDescription>
                  从人才库聘用，或创建新人才模板并配置员工能力与权限。
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
              knowledgeBaseNames={agent.knowledgeBaseIds.map(
                (id) =>
                  knowledgeBases.find((item) => item.id === id)?.name ?? id
              )}
              globalProvider={state?.config.provider ?? "—"}
              globalModel={state?.config.model ?? "—"}
              departmentName={
                state?.departments?.find(
                  (department) => department.id === agent.departmentId
                )?.name
              }
              positionName={
                state?.positions?.find(
                  (position) => position.id === agent.positionId
                )?.name
              }
              onEdit={() => setEditing(agent)}
            />
          ))}
        </div>
      )}

      {editingAgent ? (
        <AgentDialog
          key={editingAgent === "new" ? "new" : editingAgent.id}
          agent={editingAgent}
          skills={skills}
          knowledgeBases={knowledgeBases}
          departments={state?.departments ?? []}
          positions={state?.positions ?? []}
          employees={agents.filter(
            (candidate) =>
              !candidate.internal && candidate.category !== "concierge"
          )}
          globalProvider={state?.config.provider ?? ""}
          globalModel={state?.config.model ?? ""}
          globalPricing={state?.config.pricing ?? zeroPricing}
          templates={templates}
          lockedTemplate={
            editingAgent === "new" ? requestedTemplate : undefined
          }
          onOpenChange={(open) => {
            if (!open) setEditing(null)
            if (
              !open &&
              (searchParams.has("template") || searchParams.has("agent"))
            )
              setSearchParams({})
          }}
          onSaved={async () => {
            setEditing(null)
            if (searchParams.has("template") || searchParams.has("agent"))
              setSearchParams({})
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
  knowledgeBaseNames,
  globalProvider,
  globalModel,
  departmentName,
  positionName,
  onEdit,
}: {
  agent: AgentDefinition
  skillNames: string[]
  knowledgeBaseNames: string[]
  globalProvider: string
  globalModel: string
  departmentName?: string
  positionName?: string
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
            <p className="mt-0.5 truncate text-xs text-muted-foreground">
              {agent.englishName || agent.id} · {positionName ?? "未分配岗位"}
            </p>
            <CardDescription className="mt-1 line-clamp-2">
              {agent.description}
            </CardDescription>
          </div>
        </div>
        <CardAction className="flex items-center gap-2">
          {agent.builtin ? <Badge variant="outline">内置</Badge> : null}
          {agent.internal ? (
            <Badge variant="secondary">系统 Agent</Badge>
          ) : null}
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
          <Metadata
            label="知识库"
            value={`${agent.knowledgeBaseIds.length} 个`}
          />
        </div>
        <Separator />
        <div className="flex flex-wrap gap-2">
          <Badge variant="secondary">
            {categoryLabels[agent.category] ?? agent.category}
          </Badge>
          <Badge variant="outline">{departmentName ?? "未分配部门"}</Badge>
          <Badge variant="outline">{positionName ?? "未分配岗位"}</Badge>
          <BoundaryBadge allowed={agent.permissions.allowWrite} label="写入" />
          <BoundaryBadge allowed={agent.permissions.allowShell} label="Shell" />
          <BoundaryBadge
            allowed={agent.permissions.allowNetwork}
            label="网络"
          />
          <Badge variant="outline">容器内全路径</Badge>
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
        {knowledgeBaseNames.length > 0 ? (
          <div className="flex flex-wrap gap-2">
            {knowledgeBaseNames.map((name) => (
              <Badge key={name} variant="secondary">
                <LibraryBig data-icon="inline-start" />
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
  knowledgeBases,
  departments,
  positions,
  employees,
  globalProvider,
  globalModel,
  globalPricing,
  templates,
  lockedTemplate,
  onOpenChange,
  onSaved,
}: {
  agent: AgentDefinition | "new"
  skills: { id: string; displayName: string; description: string }[]
  knowledgeBases: { id: string; name: string; description: string }[]
  departments: { id: string; name: string }[]
  positions: Position[]
  employees: AgentDefinition[]
  globalProvider: string
  globalModel: string
  globalPricing: ModelPricing
  templates: AgentTemplate[]
  lockedTemplate?: AgentTemplate
  onOpenChange: (open: boolean) => void
  onSaved: () => Promise<void>
}) {
  const [form, setForm] = React.useState<SaveAgentInput>(() =>
    agent === "new"
      ? lockedTemplate
        ? templateAgentInput(lockedTemplate)
        : blankAgent()
      : agentInput(agent)
  )
  const [saving, setSaving] = React.useState(false)
  const [removing, setRemoving] = React.useState(false)
  const previousMemo = React.useRef(agent === "new" ? "" : agent.memo)
  const selectedPosition = positions.find(
    (position) => position.id === form.positionId
  )

  React.useEffect(() => {
    if (agent === "new") return
    const prior = previousMemo.current
    previousMemo.current = agent.memo
    setForm((current) =>
      current.memo === prior && current.memo !== agent.memo
        ? { ...current, memo: agent.memo }
        : current
    )
  }, [agent])

  const set = <K extends keyof SaveAgentInput>(
    key: K,
    value: SaveAgentInput[K]
  ) => setForm((current) => ({ ...current, [key]: value }))

  const submit = async (event: React.FormEvent) => {
    event.preventDefault()
    if (
      !form.name.trim() ||
      !form.englishName.trim() ||
      !form.systemPrompt.trim() ||
      saving
    )
      return
    if (agent === "new" || !agent.internal) {
      if (!form.positionId?.trim()) {
        toast.error("员工必须先选择岗位")
        return
      }
    }
    setSaving(true)
    try {
      const provider = form.model.provider || globalProvider
      const model = form.model.model || globalModel
      const templateId = form.positionId
        ? form.templateId
        : await agentTemplateId(provider, model, form.systemPrompt)
      let template = templates.find((item) => item.id === templateId)
      if (!template) {
        const chineseName = window
          .prompt("新增人才模板：中文名", form.name)
          ?.trim()
        if (!chineseName) return
        const englishName = window
          .prompt("新增人才模板：英文名", form.id || form.name)
          ?.trim()
        if (!englishName) return
        const introduction = window
          .prompt("新增人才模板：个人介绍", form.description)
          ?.trim()
        if (!introduction) return
        const positions = window
          .prompt("新增人才模板：职位（多个用逗号分隔）", form.category)
          ?.split(/[,，]/)
          .map((value) => value.trim())
          .filter(Boolean)
        if (!positions?.length) return
        template = await createAgentTemplate({
          provider,
          model,
          systemPrompt: form.systemPrompt,
          note: "",
          metadata: { englishName, chineseName, introduction, positions },
        })
      }
      const payload = { ...form, templateId: template.id }
      if (agent === "new") await createAgent(payload)
      else await updateAgent(agent.id, payload)
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
    if (form.category !== "concierge" && requiredAgentTools.has(tool)) return
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

  const toggleKnowledgeBase = (knowledgeBaseId: string, checked: boolean) =>
    set(
      "knowledgeBaseIds",
      checked
        ? [...new Set([...form.knowledgeBaseIds, knowledgeBaseId])]
        : form.knowledgeBaseIds.filter((item) => item !== knowledgeBaseId)
    )

  const updatePermission = <K extends keyof PermissionBoundary>(
    key: K,
    value: PermissionBoundary[K]
  ) => set("permissions", { ...form.permissions, [key]: value })

  const applyPosition = (positionId: string) => {
    const position = positions.find((item) => item.id === positionId)
    if (!position) return
    setForm((current) => ({
      ...current,
      positionId: position.id,
      departmentId: position.departmentId,
      templateId: position.templateId,
      description: position.description,
      avatar: position.avatar,
      category: position.category,
      model: {
        ...position.model,
        pricing: position.model.pricing ? { ...position.model.pricing } : null,
      },
      systemPrompt: position.systemPrompt,
      tools: [...position.tools],
      skillIds: [...position.skillIds],
      knowledgeBaseIds: [...position.knowledgeBaseIds],
      permissions: { ...position.permissions },
    }))
  }

  return (
    <Dialog open onOpenChange={onOpenChange}>
      <DialogContent className="max-h-[92svh] overflow-y-auto sm:max-w-4xl">
        <form onSubmit={submit} className="contents">
          <DialogHeader>
            <DialogTitle>
              {agent === "new" ? "新增员工" : `配置 ${form.name}`}
            </DialogTitle>
            <DialogDescription>
              人才模板提供身份、模型和系统提示词；此处配置员工的运行能力、知识和权限。
            </DialogDescription>
          </DialogHeader>

          <Tabs defaultValue="identity" className="min-h-0">
            <TabsList
              className="w-full justify-start overflow-x-auto"
              variant="line"
            >
              <TabsTrigger value="identity">身份与模型</TabsTrigger>
              <TabsTrigger value="prompt">系统提示词</TabsTrigger>
              <TabsTrigger value="capabilities">能力与知识</TabsTrigger>
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
                    <FieldLabel htmlFor="agent-name">中文姓名</FieldLabel>
                    <Input
                      id="agent-name"
                      value={form.name}
                      onChange={(event) => set("name", event.target.value)}
                    />
                  </Field>
                  <Field>
                    <FieldLabel htmlFor="agent-english-name">
                      英文姓名
                    </FieldLabel>
                    <Input
                      id="agent-english-name"
                      value={form.englishName}
                      placeholder="例如：Michael Chen"
                      onChange={(event) =>
                        set("englishName", event.target.value)
                      }
                    />
                  </Field>
                  <Field>
                    <FieldLabel htmlFor="agent-position">岗位</FieldLabel>
                    <Select
                      value={form.positionId ?? "none"}
                      onValueChange={(value) => applyPosition(String(value))}
                    >
                      <SelectTrigger id="agent-position" className="w-full">
                        <SelectValue placeholder="选择岗位" />
                      </SelectTrigger>
                      <SelectContent>
                        {positions
                          .filter((position) => position.enabled)
                          .map((position) => (
                            <SelectItem key={position.id} value={position.id}>
                              {position.name} · {position.englishName}
                            </SelectItem>
                          ))}
                      </SelectContent>
                    </Select>
                  </Field>
                  <Field>
                    <FieldLabel htmlFor="agent-category">能力类别</FieldLabel>
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
                          <SelectItem value="concierge">管家</SelectItem>
                          <SelectItem value="orchestrator">调度</SelectItem>
                          <SelectItem value="backend">后端</SelectItem>
                          <SelectItem value="frontend">前端</SelectItem>
                          <SelectItem value="design">设计</SelectItem>
                          <SelectItem value="security">安全</SelectItem>
                        </SelectGroup>
                      </SelectContent>
                    </Select>
                  </Field>
                  <Field>
                    <FieldLabel htmlFor="agent-department">所属部门</FieldLabel>
                    <Input
                      id="agent-department"
                      value={
                        departments.find(
                          (department) =>
                            department.id === selectedPosition?.departmentId
                        )?.name ?? "请先选择岗位"
                      }
                      disabled
                    />
                  </Field>
                  <Field>
                    <FieldLabel htmlFor="agent-manager">直属上级</FieldLabel>
                    <Select
                      value={form.managerAgentId || "none"}
                      onValueChange={(value) =>
                        set(
                          "managerAgentId",
                          value === "none" ? "" : String(value)
                        )
                      }
                    >
                      <SelectTrigger id="agent-manager" className="w-full">
                        <SelectValue placeholder="选择直属上级" />
                      </SelectTrigger>
                      <SelectContent>
                        <SelectItem value="none">无直属上级</SelectItem>
                        {employees
                          .filter(
                            (candidate) =>
                              agent === "new" || candidate.id !== agent.id
                          )
                          .map((candidate) => (
                            <SelectItem key={candidate.id} value={candidate.id}>
                              {candidate.name} ·{" "}
                              {positions.find(
                                (position) =>
                                  position.id === candidate.positionId
                              )?.name ?? candidate.category}
                            </SelectItem>
                          ))}
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
                      disabled={agent !== "new" && agent.internal}
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
                        disabled={Boolean(lockedTemplate)}
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
                        disabled={Boolean(lockedTemplate)}
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
                <FieldSet>
                  <FieldLegend>价格覆盖</FieldLegend>
                  <FieldDescription>
                    默认继承全局模型价格；启用后，该 Agent 的新 Session
                    使用独立价格。
                  </FieldDescription>
                  <Field
                    orientation="horizontal"
                    className="rounded-lg border p-3"
                  >
                    <FieldLabel
                      htmlFor="agent-pricing-override"
                      className="flex-1"
                    >
                      <span>使用独立价格</span>
                      <FieldDescription>
                        关闭时使用当前全局模型的四类 token 单价。
                      </FieldDescription>
                    </FieldLabel>
                    <Switch
                      id="agent-pricing-override"
                      checked={form.model.pricing !== null}
                      onCheckedChange={(checked) =>
                        set("model", {
                          ...form.model,
                          pricing: checked ? { ...globalPricing } : null,
                        })
                      }
                    />
                  </Field>
                </FieldSet>
                {form.model.pricing ? (
                  <ModelPricingFields
                    idPrefix="agent-price"
                    value={form.model.pricing}
                    onChange={(pricing) =>
                      set("model", { ...form.model, pricing })
                    }
                  />
                ) : null}
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
                    readOnly={Boolean(lockedTemplate)}
                    onChange={(event) =>
                      set("systemPrompt", event.target.value)
                    }
                  />
                  <FieldDescription>
                    启动 Pi RPC 时通过独立的 system prompt
                    注入，不与任务提示词混合保存。
                  </FieldDescription>
                </Field>
                <Field>
                  <FieldLabel htmlFor="agent-memo">Agent 备忘录</FieldLabel>
                  <Textarea
                    id="agent-memo"
                    className="min-h-64 font-mono text-xs leading-5"
                    maxLength={20000}
                    value={form.memo}
                    onChange={(event) => set("memo", event.target.value)}
                    placeholder="记录稳定的用户/Leader 偏好、长期工作倾向、反复纠错和常见错误提醒。"
                  />
                  <FieldDescription>
                    跨会话持久保存，并在新会话的第一条任务提示中注入。Agent
                    也可以通过固有工具读取和更新。不要保存密码、令牌、敏感个人信息或一次性任务状态。
                  </FieldDescription>
                </Field>
              </FieldGroup>
            </TabsContent>

            <TabsContent value="capabilities" className="pt-4">
              {agent !== "new" && agent.internal ? (
                <Card size="sm">
                  <CardHeader>
                    <CardTitle>受保护的只读能力</CardTitle>
                    <CardDescription>
                      这个系统 Agent
                      只接收检索服务传入的候选文档片段，实际运行时使用
                      --no-tools，不能访问文件系统、Shell、网络或写入任何内容。
                    </CardDescription>
                  </CardHeader>
                  <CardContent>
                    <div className="flex flex-wrap gap-2">
                      <Badge variant="outline">无工具</Badge>
                      <Badge variant="outline">无 Skills</Badge>
                      <Badge variant="outline">无外部知识库关联</Badge>
                    </div>
                  </CardContent>
                </Card>
              ) : (
                <FieldGroup>
                  <FieldSet>
                    <FieldLegend>工具集</FieldLegend>
                    <FieldDescription>
                      每个 Agent 保存独立工具数组；Issue 拆分是所有 Agent
                      必备的控制面能力。
                    </FieldDescription>
                    <div
                      data-slot="checkbox-group"
                      className="grid gap-2 sm:grid-cols-2 lg:grid-cols-3"
                    >
                      {allTools.map((tool) => (
                        <Field
                          key={tool}
                          orientation="horizontal"
                          data-disabled={
                            (form.category !== "concierge" &&
                              requiredAgentTools.has(tool)) ||
                            undefined
                          }
                          className="rounded-lg border p-3"
                        >
                          <Checkbox
                            id={`agent-tool-${tool}`}
                            checked={
                              (form.category !== "concierge" &&
                                requiredAgentTools.has(tool)) ||
                              form.tools.includes(tool)
                            }
                            disabled={
                              form.category !== "concierge" &&
                              requiredAgentTools.has(tool)
                            }
                            onCheckedChange={(checked) =>
                              toggleTool(tool, checked)
                            }
                          />
                          <FieldLabel
                            htmlFor={`agent-tool-${tool}`}
                            className="font-mono font-normal"
                          >
                            {tool}
                            {form.category !== "concierge" &&
                            requiredAgentTools.has(tool)
                              ? "（必备）"
                              : ""}
                          </FieldLabel>
                        </Field>
                      ))}
                    </div>
                  </FieldSet>
                  <FieldSet>
                    <FieldLegend>Skills / 方法</FieldLegend>
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
                  <FieldSet>
                    <FieldLegend>关联知识库</FieldLegend>
                    <FieldDescription>
                      关联后会把知识库介绍注入系统提示词，并自动为这个 Agent
                      增加只读的 aegis_search_knowledge 工具。
                    </FieldDescription>
                    {knowledgeBases.length === 0 ? (
                      <div className="rounded-lg border border-dashed p-4 text-sm text-muted-foreground">
                        暂无知识库，请先在知识库页面创建并添加 Markdown 文档。
                      </div>
                    ) : (
                      <div
                        data-slot="checkbox-group"
                        className="grid gap-2 sm:grid-cols-2"
                      >
                        {knowledgeBases.map((knowledgeBase) => (
                          <Field
                            key={knowledgeBase.id}
                            orientation="horizontal"
                            className="rounded-lg border p-3"
                          >
                            <Checkbox
                              id={`agent-knowledge-${knowledgeBase.id}`}
                              checked={form.knowledgeBaseIds.includes(
                                knowledgeBase.id
                              )}
                              onCheckedChange={(checked) =>
                                toggleKnowledgeBase(knowledgeBase.id, checked)
                              }
                            />
                            <FieldLabel
                              htmlFor={`agent-knowledge-${knowledgeBase.id}`}
                              className="font-normal"
                            >
                              <span>{knowledgeBase.name}</span>
                              <FieldDescription>
                                {knowledgeBase.description}
                              </FieldDescription>
                            </FieldLabel>
                          </Field>
                        ))}
                      </div>
                    )}
                  </FieldSet>
                </FieldGroup>
              )}
            </TabsContent>

            <TabsContent value="permissions" className="pt-4">
              {agent !== "new" && agent.internal ? (
                <Card size="sm">
                  <CardHeader>
                    <CardTitle>固定权限边界</CardTitle>
                    <CardDescription>
                      禁止网络、Shell 与写入，审批策略固定为
                      none；检索进程没有可调用工具。
                    </CardDescription>
                  </CardHeader>
                  <CardContent className="flex flex-wrap gap-2">
                    <BoundaryBadge allowed={false} label="写入" />
                    <BoundaryBadge allowed={false} label="Shell" />
                    <BoundaryBadge allowed={false} label="网络" />
                  </CardContent>
                </Card>
              ) : (
                <FieldGroup>
                  <Field>
                    <FieldTitle>工作区范围</FieldTitle>
                    <Input value="任务 Docker 容器内全部路径" disabled />
                    <FieldDescription>
                      任务工作区仅作为 Pi 默认目录，不限制容器内其他文件路径。
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
                      工具调用审批
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
                      <SelectTrigger
                        id="permission-approval"
                        className="w-full"
                      >
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
                  <Field>
                    <FieldLabel htmlFor="permission-rework-approval">
                      Issue 返工审批
                    </FieldLabel>
                    <Select
                      value={form.permissions.reworkApprovalMode || "inherit"}
                      onValueChange={(value) =>
                        updatePermission(
                          "reworkApprovalMode",
                          value === "inherit"
                            ? ""
                            : (String(
                                value
                              ) as PermissionBoundary["reworkApprovalMode"])
                        )
                      }
                    >
                      <SelectTrigger
                        id="permission-rework-approval"
                        className="w-full"
                      >
                        <SelectValue />
                      </SelectTrigger>
                      <SelectContent>
                        <SelectGroup>
                          <SelectItem value="inherit">继承全局策略</SelectItem>
                          <SelectItem value="all">始终需要人工批准</SelectItem>
                          <SelectItem value="none">
                            自动批准并重新执行
                          </SelectItem>
                        </SelectGroup>
                      </SelectContent>
                    </Select>
                  </Field>
                </FieldGroup>
              )}
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
  if (category === "design") return <Palette className={className} />
  if (category === "security") return <Shield className={className} />
  if (category === "knowledge") return <Database className={className} />
  if (category === "concierge") return <Sparkles className={className} />
  if (category === "data") return <Database className={className} />
  return <Wrench className={className} />
}

function blankAgent(): SaveAgentInput {
  return {
    id: "",
    templateId: "",
    name: "",
    englishName: "",
    description: "",
    avatar: "bot",
    category: "general",
    enabled: true,
    internal: false,
    model: {
      provider: "",
      model: "",
      baseUrl: "",
      thinking: "",
      pricing: null,
    },
    systemPrompt: "",
    memo: "",
    tools: [...allTools],
    skillIds: [],
    knowledgeBaseIds: [],
    permissions: {
      workspaceScope: "run_workspace",
      allowNetwork: true,
      allowShell: true,
      allowWrite: true,
      approvalMode: "none",
      reworkApprovalMode: "none",
    },
    departmentId: "",
    positionId: "",
    managerAgentId: "",
  }
}

function agentInput(agent: AgentDefinition): SaveAgentInput {
  return {
    id: agent.id,
    templateId: agent.templateId,
    name: agent.name,
    englishName: agent.englishName,
    description: agent.description,
    avatar: agent.avatar,
    category: agent.category,
    enabled: agent.enabled,
    internal: agent.internal,
    model: { ...agent.model },
    systemPrompt: agent.systemPrompt,
    memo: agent.memo ?? "",
    tools: [...agent.tools],
    skillIds: [...agent.skillIds],
    knowledgeBaseIds: [...agent.knowledgeBaseIds],
    permissions: { ...agent.permissions },
    departmentId: agent.departmentId ?? "",
    positionId: agent.positionId ?? "",
    managerAgentId: agent.managerAgentId ?? "",
  }
}

function templateAgentInput(template: AgentTemplate): SaveAgentInput {
  const input = blankAgent()
  return {
    ...input,
    templateId: template.id,
    id: template.metadata.englishName,
    name: template.metadata.chineseName,
    englishName: template.metadata.englishName,
    description: template.metadata.introduction,
    category: template.metadata.positions[0] || "general",
    model: {
      ...input.model,
      provider: template.provider,
      model: template.model,
    },
    systemPrompt: template.systemPrompt,
  }
}
