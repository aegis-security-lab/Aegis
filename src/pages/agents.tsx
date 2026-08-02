import * as React from "react"
import {
  Bot,
  BrainCircuit,
  Database,
  Pencil,
  Plus,
  ShieldCheck,
  Trash2,
} from "lucide-react"
import { toast } from "sonner"

import { PageHeader } from "@/components/page-header"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import {
  Card,
  CardContent,
  CardDescription,
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
import { Field, FieldGroup, FieldLabel } from "@/components/ui/field"
import { Input } from "@/components/ui/input"
import { ScrollArea } from "@/components/ui/scroll-area"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
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

const toolCatalog = [
  "read",
  "grep",
  "find",
  "ls",
  "bash",
  "edit",
  "write",
  "aegis_board",
  "aegis_relay",
  "aegis_web_search",
  "aegis_publish_attachment",
  "aegis_submit_final_result",
  "aegis_report_progress",
  "aegis_get_issue_progress",
  "aegis_get_memo",
  "aegis_update_memo",
]

const openPermissions: PermissionBoundary = {
  workspaceScope: "run_workspace",
  allowNetwork: true,
  allowShell: true,
  allowWrite: true,
  approvalMode: "none",
  reworkApprovalMode: "none",
}

function blankAgent(): SaveAgentInput {
  return {
    id: "",
    name: "",
    description: "",
    avatar: "bot",
    category: "general",
    enabled: true,
    internal: false,
    model: { provider: "", model: "", baseUrl: "", thinking: "", pricing: null },
    systemPrompt: "",
    memo: "",
    tools: [...toolCatalog],
    skillIds: [],
    knowledgeBaseIds: [],
    permissions: { ...openPermissions },
  }
}

function editAgent(agent: AgentDefinition): SaveAgentInput {
  return {
    ...agent,
    model: { ...agent.model, pricing: agent.model.pricing ? { ...agent.model.pricing } : null },
    tools: [...agent.tools],
    skillIds: [...agent.skillIds],
    knowledgeBaseIds: [...agent.knowledgeBaseIds],
    permissions: { ...agent.permissions },
  }
}

export function AgentsPage() {
  const { state, refresh } = useAppState()
  const agents = (state?.agents ?? []).filter((agent) => !agent.internal)
  const [editing, setEditing] = React.useState<AgentDefinition | "new" | null>(null)

  return (
    <div className="flex flex-col gap-6">
      <PageHeader
        eyebrow="Agent type registry"
        title="Agent 类型"
        description="这里定义可复用的角色能力。它不是员工名册：任务发布后，系统才为每个 Issue 创建独立的临时身份、会话和手机。"
        actions={
          <Button onClick={() => setEditing("new")}>
            <Plus data-icon="inline-start" />
            新建 Agent 类型
          </Button>
        }
      />

      <div className="grid gap-3 md:grid-cols-3">
        <Metric icon={Bot} label="可调度类型" value={agents.filter((item) => item.enabled).length} />
        <Metric icon={BrainCircuit} label="Skill 绑定" value={agents.reduce((sum, item) => sum + item.skillIds.length, 0)} />
        <Metric icon={Database} label="知识库绑定" value={agents.reduce((sum, item) => sum + item.knowledgeBaseIds.length, 0)} />
      </div>

      <Card className="overflow-hidden">
        <CardHeader className="border-b bg-muted/25">
          <CardTitle>运行模型</CardTitle>
          <CardDescription>
            Agent 类型决定“会做什么”；任务成员实例决定“这次是谁、在哪个 Issue、使用哪部手机”。同一类型可以在一个任务中并行创建多个实例。
          </CardDescription>
        </CardHeader>
        <CardContent className="p-0">
          <div className="hidden grid-cols-[minmax(12rem,1.2fr)_minmax(14rem,2fr)_8rem_8rem_6rem] gap-4 border-b px-5 py-2 text-xs font-medium text-muted-foreground lg:grid">
            <span>Agent 类型</span><span>职责与能力</span><span>模型</span><span>权限</span><span className="text-right">操作</span>
          </div>
          {agents.map((agent) => (
            <AgentRow key={agent.id} agent={agent} onEdit={() => setEditing(agent)} onDelete={async () => {
              if (!window.confirm(`删除 Agent 类型「${agent.name}」？`)) return
              try { await deleteAgent(agent.id); await refresh(); toast.success("Agent 类型已删除") }
              catch (error) { toast.error(error instanceof Error ? error.message : "删除失败") }
            }} />
          ))}
          {agents.length === 0 ? <div className="px-6 py-16 text-center text-sm text-muted-foreground">暂无 Agent 类型。创建一个角色能力定义后即可发布任务。</div> : null}
        </CardContent>
      </Card>

      <AgentEditor
        key={editing === "new" ? "new" : editing?.id ?? "closed"}
        agent={editing}
        onClose={() => setEditing(null)}
        onSaved={async () => { setEditing(null); await refresh() }}
      />
    </div>
  )
}

function Metric({ icon: Icon, label, value }: { icon: typeof Bot; label: string; value: number }) {
  return <Card><CardContent className="flex items-center gap-3 py-4"><span className="flex size-9 items-center justify-center rounded-md border bg-background"><Icon className="size-4 text-primary" /></span><div><div className="text-2xl font-semibold tabular-nums">{value}</div><div className="text-xs text-muted-foreground">{label}</div></div></CardContent></Card>
}

function AgentRow({ agent, onEdit, onDelete }: { agent: AgentDefinition; onEdit: () => void; onDelete: () => void }) {
  const capabilityCount = 2 + agent.skillIds.length + agent.knowledgeBaseIds.length
  return (
    <div className="grid gap-4 border-b px-5 py-4 last:border-b-0 lg:grid-cols-[minmax(12rem,1.2fr)_minmax(14rem,2fr)_8rem_8rem_6rem] lg:items-center">
      <div className="min-w-0">
        <div className="flex items-center gap-2"><span className="truncate font-medium">{agent.name}</span><span className={`size-2 rounded-full ${agent.enabled ? "bg-emerald-500" : "bg-muted-foreground/40"}`} /></div>
        <code className="text-xs text-muted-foreground">{agent.id}</code>
      </div>
      <div className="min-w-0"><p className="line-clamp-2 text-sm text-muted-foreground">{agent.description || "未填写职责边界"}</p><div className="mt-2 flex flex-wrap gap-1.5"><Badge variant="outline">{agent.category}</Badge><Badge variant="secondary">{capabilityCount} 项能力</Badge>{agent.skillIds.slice(0, 2).map((id) => <Badge key={id} variant="outline">{id}</Badge>)}</div></div>
      <div className="text-sm"><div className="font-medium">{agent.model.model || "继承默认"}</div><div className="text-xs text-muted-foreground">{agent.model.provider || "default"}</div></div>
      <div className="flex items-center gap-2 text-sm"><ShieldCheck className="size-4 text-muted-foreground" /><span>{agent.permissions.approvalMode || "继承"}</span></div>
      <div className="flex justify-end gap-1"><Button variant="ghost" size="icon-sm" aria-label={`编辑 ${agent.name}`} onClick={onEdit}><Pencil /></Button><Button variant="ghost" size="icon-sm" aria-label={`删除 ${agent.name}`} disabled={agent.builtin} onClick={onDelete}><Trash2 /></Button></div>
    </div>
  )
}

function AgentEditor({ agent, onClose, onSaved }: { agent: AgentDefinition | "new" | null; onClose: () => void; onSaved: () => Promise<void> }) {
  const { state } = useAppState()
  const [form, setForm] = React.useState<SaveAgentInput>(() => agent === "new" || !agent ? blankAgent() : editAgent(agent))
  const [saving, setSaving] = React.useState(false)
  if (!agent) return null
  const set = <K extends keyof SaveAgentInput>(key: K, value: SaveAgentInput[K]) => setForm((current) => ({ ...current, [key]: value }))
  const toggleItem = (key: "tools" | "skillIds" | "knowledgeBaseIds", value: string, checked: boolean) => set(key, checked ? [...new Set([...form[key], value])] : form[key].filter((item) => item !== value))
  const submit = async (event: React.FormEvent) => {
    event.preventDefault()
    if (!form.name.trim() || !form.systemPrompt.trim()) { toast.error("请填写 Agent 类型名称和系统提示词"); return }
    setSaving(true)
    try {
      if (agent === "new") await createAgent(form)
      else await updateAgent(agent.id, form)
      toast.success(agent === "new" ? "Agent 类型已创建" : "Agent 类型已更新")
      await onSaved()
    } catch (error) { toast.error(error instanceof Error ? error.message : "保存失败") }
    finally { setSaving(false) }
  }
  return (
    <Dialog open onOpenChange={(open) => { if (!open) onClose() }}>
      <DialogContent className="max-h-[88vh] max-w-3xl overflow-hidden p-0">
        <form onSubmit={submit} className="flex max-h-[88vh] flex-col">
          <DialogHeader className="border-b px-6 py-5"><DialogTitle>{agent === "new" ? "新建 Agent 类型" : `编辑 ${agent.name}`}</DialogTitle><DialogDescription>定义角色职责、模型、Skills、知识和权限。Coordination 与任务 Phone 由运行时自动提供；个人名字和手机不在这里配置。</DialogDescription></DialogHeader>
          <ScrollArea className="min-h-0 flex-1">
            <div className="p-6">
              <Tabs defaultValue="definition">
                <TabsList><TabsTrigger value="definition">角色定义</TabsTrigger><TabsTrigger value="capabilities">能力</TabsTrigger><TabsTrigger value="runtime">运行权限</TabsTrigger></TabsList>
                <TabsContent value="definition" className="pt-5"><FieldGroup><div className="grid gap-4 sm:grid-cols-2"><Field><FieldLabel htmlFor="agent-name">类型名称</FieldLabel><Input id="agent-name" value={form.name} onChange={(event) => set("name", event.target.value)} placeholder="例如：红队工程师" /></Field><Field><FieldLabel htmlFor="agent-id">Agent ID</FieldLabel><Input id="agent-id" disabled={agent !== "new"} value={form.id} onChange={(event) => set("id", event.target.value)} placeholder="red-team-engineer" /></Field></div><Field><FieldLabel htmlFor="agent-description">职责边界</FieldLabel><Textarea id="agent-description" value={form.description} onChange={(event) => set("description", event.target.value)} placeholder="说明适合接收什么任务、产出什么证据。" /></Field><div className="grid gap-4 sm:grid-cols-2"><Field><FieldLabel htmlFor="agent-category">分类</FieldLabel><Input id="agent-category" value={form.category} onChange={(event) => set("category", event.target.value)} /></Field><Field className="flex-row items-center justify-between rounded-md border p-3"><div><FieldLabel>允许调度</FieldLabel><p className="text-xs text-muted-foreground">关闭后不会被分配新 Issue</p></div><Switch checked={form.enabled} onCheckedChange={(checked) => set("enabled", checked)} /></Field></div><Field><FieldLabel htmlFor="agent-prompt">系统提示词</FieldLabel><Textarea id="agent-prompt" className="min-h-64 font-mono text-xs" value={form.systemPrompt} onChange={(event) => set("systemPrompt", event.target.value)} /></Field></FieldGroup></TabsContent>
                <TabsContent value="capabilities" className="space-y-6 pt-5"><div className="rounded-md border bg-muted/25 p-4 text-sm"><p className="font-medium">运行时基础能力</p><p className="mt-1 text-xs leading-relaxed text-muted-foreground">每个 TaskAgent 自动获得自己的 Phone；Issue 委派、继续和休眠均通过 Phone Board 及其快捷指令完成。Web 与外部 MCP 由能力控制台和任务策略决定。</p><div className="mt-3 flex flex-wrap gap-1.5"><Badge variant="secondary">phone/default</Badge><Badge variant="secondary">phone_board_*</Badge><Badge variant="secondary">phone_relay_*</Badge></div></div><CapabilityGroup title="Skills" icon={BrainCircuit} items={(state?.skills ?? []).map((item) => item.id)} selected={form.skillIds} onToggle={(id, checked) => toggleItem("skillIds", id, checked)} /><CapabilityGroup title="知识库" icon={Database} items={(state?.knowledgeBases ?? []).map((item) => item.id)} selected={form.knowledgeBaseIds} onToggle={(id, checked) => toggleItem("knowledgeBaseIds", id, checked)} /></TabsContent>
                <TabsContent value="runtime" className="pt-5"><FieldGroup><div className="grid gap-4 sm:grid-cols-2"><Field><FieldLabel htmlFor="provider">Provider</FieldLabel><Input id="provider" value={form.model.provider} onChange={(event) => set("model", { ...form.model, provider: event.target.value })} placeholder="留空继承系统默认" /></Field><Field><FieldLabel htmlFor="model">Model</FieldLabel><Input id="model" value={form.model.model} onChange={(event) => set("model", { ...form.model, model: event.target.value })} placeholder="留空继承系统默认" /></Field></div><Field><FieldLabel htmlFor="base-url">Base URL</FieldLabel><Input id="base-url" value={form.model.baseUrl} onChange={(event) => set("model", { ...form.model, baseUrl: event.target.value })} /></Field><div className="grid gap-3 sm:grid-cols-3">{([['allowNetwork','网络'],['allowShell','Shell'],['allowWrite','写入']] as const).map(([key,label]) => <label key={key} className="flex items-center justify-between rounded-md border p-3 text-sm"><span>{label}</span><Switch checked={form.permissions[key]} onCheckedChange={(checked) => set("permissions", { ...form.permissions, [key]: checked })} /></label>)}</div><Field><FieldLabel>审批策略</FieldLabel><Select value={form.permissions.approvalMode || "inherit"} onValueChange={(value) => set("permissions", { ...form.permissions, approvalMode: value === "inherit" ? "" : value as PermissionBoundary["approvalMode"] })}><SelectTrigger className="w-full"><SelectValue /></SelectTrigger><SelectContent><SelectItem value="inherit">继承系统</SelectItem><SelectItem value="none">无需审批</SelectItem><SelectItem value="risky">仅高风险</SelectItem><SelectItem value="all">全部审批</SelectItem></SelectContent></Select></Field></FieldGroup></TabsContent>
              </Tabs>
            </div>
          </ScrollArea>
          <DialogFooter className="border-t px-6 py-4"><Button type="button" variant="outline" onClick={onClose}>取消</Button><Button type="submit" disabled={saving}>{saving ? <Spinner data-icon="inline-start" /> : null}保存类型</Button></DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}

function CapabilityGroup({ title, icon: Icon, items, selected, onToggle }: { title: string; icon: typeof Bot; items: string[]; selected: string[]; onToggle: (id: string, checked: boolean) => void }) {
  return <section><div className="mb-3 flex items-center gap-2 text-sm font-medium"><Icon className="size-4 text-primary" />{title}<Badge variant="secondary">{selected.length}</Badge></div><div className="grid gap-2 sm:grid-cols-2">{items.map((item) => <label key={item} className="flex items-center gap-3 rounded-md border p-3 text-sm hover:bg-muted/40"><Checkbox checked={selected.includes(item)} onCheckedChange={(checked) => onToggle(item, checked === true)} /><code className="truncate text-xs">{item}</code></label>)}</div>{items.length === 0 ? <p className="text-sm text-muted-foreground">暂无可绑定项</p> : null}</section>
}
