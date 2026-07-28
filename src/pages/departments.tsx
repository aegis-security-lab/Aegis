import * as React from "react"
import {
  BriefcaseBusiness,
  Building2,
  ChevronRight,
  Network,
  Pencil,
  Plus,
  Trash2,
  UserPlus,
  UserRound,
  UsersRound,
} from "lucide-react"
import { Link, useNavigate } from "react-router-dom"
import { toast } from "sonner"

import { PageHeader } from "@/components/page-header"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from "@/components/ui/collapsible"
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
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import { Textarea } from "@/components/ui/textarea"
import {
  createDepartment,
  createPosition,
  deleteDepartment,
  deletePosition,
  fetchAgentTemplates,
  hireEmployee,
  updateDepartment,
  updatePosition,
  type SavePositionInput,
} from "@/lib/api"
import { useAppState } from "@/lib/state"
import type {
  AgentDefinition,
  AgentTemplate,
  Department,
  Position,
} from "@/types"

const defaultTools = [
  "read",
  "grep",
  "find",
  "ls",
  "bash",
  "edit",
  "write",
  "aegis_board",
  "aegis_relay",
  "aegis_create_subissues",
  "aegis_list_child_issues",
  "aegis_wait_for_child_issues",
  "aegis_cancel_issue",
  "aegis_publish_attachment",
  "aegis_submit_final_result",
  "aegis_report_progress",
  "aegis_get_issue_progress",
  "aegis_get_memo",
  "aegis_update_memo",
  "aegis_request_rework",
]

type DepartmentEditor = {
  id?: string
  name: string
  code: string
  description: string
  parentId: string
  leaderAgentId: string
  enabled: boolean
}

const blankDepartment = (): DepartmentEditor => ({
  name: "",
  code: "",
  description: "",
  parentId: "",
  leaderAgentId: "",
  enabled: true,
})

export function DepartmentsPage() {
  const { state, refresh } = useAppState()
  const navigate = useNavigate()
  const departments = state?.departments ?? []
  const positions = state?.positions ?? []
  const employees = (state?.agents ?? []).filter(
    (agent) => !agent.internal && agent.category !== "concierge"
  )
  const [templates, setTemplates] = React.useState<AgentTemplate[]>([])
  const [departmentEditor, setDepartmentEditor] =
    React.useState<DepartmentEditor | null>(null)
  const [positionEditor, setPositionEditor] = React.useState<
    Position | "new" | null
  >(null)
  const [positionDepartmentID, setPositionDepartmentID] = React.useState("")
  const [hiringPosition, setHiringPosition] = React.useState<Position | null>(
    null
  )

  React.useEffect(() => {
    void fetchAgentTemplates()
      .then(setTemplates)
      .catch(() => undefined)
  }, [])

  const childrenOf = (parentID: string) =>
    departments.filter((item) => (item.parentId || "") === parentID)

  const openDepartment = (department?: Department) =>
    setDepartmentEditor(
      department
        ? {
            id: department.id,
            name: department.name,
            code: department.code,
            description: department.description,
            parentId: department.parentId ?? "",
            leaderAgentId: department.leaderAgentId ?? "",
            enabled: department.enabled,
          }
        : blankDepartment()
    )

  const openPosition = (departmentID: string, position?: Position) => {
    setPositionDepartmentID(departmentID)
    setPositionEditor(position ?? "new")
  }

  const renderDepartment = (
    department: Department,
    depth = 0
  ): React.ReactNode => {
    const departmentPositions = positions.filter(
      (position) => position.departmentId === department.id
    )
    const departmentEmployees = employees.filter(
      (employee) => employee.departmentId === department.id
    )
    const leader = employees.find(
      (employee) => employee.id === department.leaderAgentId
    )
    const children = childrenOf(department.id)
    return (
      <Collapsible key={department.id} defaultOpen className="group/department">
        <Card style={{ marginLeft: `${Math.min(depth, 4) * 18}px` }}>
          <CardHeader className="gap-3 border-b">
            <div className="flex min-w-0 items-start gap-3">
              <span className="flex size-10 shrink-0 items-center justify-center rounded-xl bg-primary/10 text-primary">
                <Building2 className="size-5" />
              </span>
              <div className="min-w-0 flex-1">
                <div className="flex flex-wrap items-center gap-2">
                  <CardTitle>{department.name}</CardTitle>
                  <Badge variant="outline">{department.code}</Badge>
                  <Badge variant={department.enabled ? "secondary" : "outline"}>
                    {department.enabled ? "启用" : "停用"}
                  </Badge>
                </div>
                <p className="mt-1 text-sm text-muted-foreground">
                  {department.description || "暂无部门说明"}
                </p>
                <div className="mt-2 flex flex-wrap gap-x-4 gap-y-1 text-xs text-muted-foreground">
                  <span>负责人：{leader?.name ?? "未设置"}</span>
                  <span>{departmentPositions.length} 个岗位</span>
                  <span>{departmentEmployees.length} 名员工</span>
                  <span>{children.length} 个下级部门</span>
                </div>
              </div>
              <Button
                variant="outline"
                size="sm"
                onClick={() => openPosition(department.id)}
              >
                <Plus />
                新增岗位
              </Button>
              <Button
                variant="ghost"
                size="icon-sm"
                aria-label="编辑部门"
                onClick={() => openDepartment(department)}
              >
                <Pencil />
              </Button>
              <Button
                variant="ghost"
                size="icon-sm"
                aria-label="删除部门"
                onClick={() => void removeDepartment(department)}
              >
                <Trash2 />
              </Button>
              <CollapsibleTrigger
                render={
                  <Button
                    variant="ghost"
                    size="icon-sm"
                    aria-label="展开部门"
                  />
                }
              >
                <ChevronRight className="transition-transform group-data-open/department:rotate-90" />
              </CollapsibleTrigger>
            </div>
          </CardHeader>
          <CollapsibleContent>
            <CardContent className="space-y-3 pt-4">
              {departmentPositions.map((position) => (
                <PositionRow
                  key={position.id}
                  position={position}
                  employees={employees.filter(
                    (employee) => employee.positionId === position.id
                  )}
                  allEmployees={employees}
                  onEdit={() => openPosition(department.id, position)}
                  onHire={() => setHiringPosition(position)}
                  onDelete={() => void removePosition(position)}
                />
              ))}
              {departmentPositions.length === 0 ? (
                <div className="rounded-xl border border-dashed px-4 py-8 text-center text-sm text-muted-foreground">
                  这个部门还没有岗位，先新增岗位再招聘员工。
                </div>
              ) : null}
            </CardContent>
          </CollapsibleContent>
        </Card>
        {children.length > 0 ? (
          <div className="mt-3 space-y-3">
            {children.map((child) => renderDepartment(child, depth + 1))}
          </div>
        ) : null}
      </Collapsible>
    )
  }

  const removeDepartment = async (department: Department) => {
    if (!window.confirm(`删除部门「${department.name}」？`)) return
    try {
      await deleteDepartment(department.id)
      await refresh()
      toast.success("部门已删除")
    } catch (reason) {
      toast.error(reason instanceof Error ? reason.message : "删除部门失败")
    }
  }

  const removePosition = async (position: Position) => {
    if (!window.confirm(`删除岗位「${position.name}」？`)) return
    try {
      await deletePosition(position.id)
      await refresh()
      toast.success("岗位已删除")
    } catch (reason) {
      toast.error(reason instanceof Error ? reason.message : "删除岗位失败")
    }
  }

  return (
    <div className="flex flex-col gap-6">
      <PageHeader
        eyebrow="Organization"
        title="组织架构"
        description="按部门、岗位和汇报关系管理员工；岗位是能力模板，员工是真实、独立的长期 Agent。"
        actions={
          <Button onClick={() => openDepartment()}>
            <Plus />
            新增部门
          </Button>
        }
      />
      <div className="grid gap-4 sm:grid-cols-3">
        <SummaryCard icon={Building2} label="部门" value={departments.length} />
        <SummaryCard
          icon={BriefcaseBusiness}
          label="岗位"
          value={positions.length}
        />
        <SummaryCard
          icon={UsersRound}
          label="在册员工"
          value={employees.length}
        />
      </div>
      <div className="space-y-3">
        {childrenOf("").map((department) => renderDepartment(department))}
      </div>

      <DepartmentDialog
        editor={departmentEditor}
        departments={departments}
        employees={employees}
        onClose={() => setDepartmentEditor(null)}
        onSaved={refresh}
        setEditor={setDepartmentEditor}
      />
      <PositionDialog
        key={
          positionEditor === "new"
            ? `new-${positionDepartmentID}-${templates.length}`
            : (positionEditor?.id ?? "closed-position")
        }
        position={positionEditor}
        departmentID={positionDepartmentID}
        templates={templates}
        onClose={() => setPositionEditor(null)}
        onSaved={refresh}
      />
      <HireDialog
        key={hiringPosition?.id ?? "closed-hire"}
        position={hiringPosition}
        employees={employees}
        defaultManagerID={
          departments.find(
            (department) => department.id === hiringPosition?.departmentId
          )?.leaderAgentId ?? ""
        }
        onClose={() => setHiringPosition(null)}
        onSaved={refresh}
        onCreated={(employee) =>
          navigate(`/agents?agent=${encodeURIComponent(employee.id)}`)
        }
      />
    </div>
  )
}

function PositionRow({
  position,
  employees,
  allEmployees,
  onEdit,
  onHire,
  onDelete,
}: {
  position: Position
  employees: AgentDefinition[]
  allEmployees: AgentDefinition[]
  onEdit: () => void
  onHire: () => void
  onDelete: () => void
}) {
  return (
    <Collapsible defaultOpen className="group/position rounded-xl border">
      <div className="flex items-center gap-3 px-4 py-3">
        <span className="flex size-9 items-center justify-center rounded-lg bg-muted">
          <BriefcaseBusiness className="size-4" />
        </span>
        <div className="min-w-0 flex-1">
          <div className="flex flex-wrap items-center gap-2">
            <span className="font-medium">{position.name}</span>
            <span className="text-sm text-muted-foreground">
              {position.englishName}
            </span>
            <Badge variant="secondary">{employees.length} 人</Badge>
          </div>
          <p className="mt-0.5 truncate text-xs text-muted-foreground">
            {position.description || position.code}
          </p>
        </div>
        <Button size="sm" variant="outline" onClick={onHire}>
          <UserPlus />
          招聘
        </Button>
        <Button size="icon-sm" variant="ghost" onClick={onEdit}>
          <Pencil />
        </Button>
        <Button size="icon-sm" variant="ghost" onClick={onDelete}>
          <Trash2 />
        </Button>
        <CollapsibleTrigger
          render={
            <Button size="icon-sm" variant="ghost" aria-label="展开岗位" />
          }
        >
          <ChevronRight className="transition-transform group-data-open/position:rotate-90" />
        </CollapsibleTrigger>
      </div>
      <CollapsibleContent className="border-t">
        {employees.length > 0 ? (
          <div className="divide-y">
            {employees.map((employee) => {
              const manager = allEmployees.find(
                (candidate) => candidate.id === employee.managerAgentId
              )
              const subordinateCount = allEmployees.filter(
                (candidate) => candidate.managerAgentId === employee.id
              ).length
              return (
                <Link
                  key={employee.id}
                  to={`/employees/${encodeURIComponent(employee.id)}`}
                  className="flex items-center gap-3 px-4 py-3 transition-colors hover:bg-muted/60"
                >
                  <UserRound className="size-4 text-muted-foreground" />
                  <span className="min-w-0 flex-1">
                    <span className="block truncate text-sm font-medium">
                      {employee.name}
                    </span>
                    <span className="block truncate text-xs text-muted-foreground">
                      {employee.englishName} · {employee.id}
                    </span>
                  </span>
                  <span className="hidden text-xs text-muted-foreground md:block">
                    汇报给 {manager?.name ?? "—"}
                  </span>
                  <Badge variant="outline">{subordinateCount} 名直属下属</Badge>
                  <Badge variant={employee.enabled ? "secondary" : "outline"}>
                    {employee.enabled ? "在职" : "停用"}
                  </Badge>
                </Link>
              )
            })}
          </div>
        ) : (
          <div className="px-4 py-6 text-center text-sm text-muted-foreground">
            岗位暂无员工
          </div>
        )}
      </CollapsibleContent>
    </Collapsible>
  )
}

function DepartmentDialog({
  editor,
  departments,
  employees,
  onClose,
  onSaved,
  setEditor,
}: {
  editor: DepartmentEditor | null
  departments: Department[]
  employees: AgentDefinition[]
  onClose: () => void
  onSaved: () => Promise<void>
  setEditor: React.Dispatch<React.SetStateAction<DepartmentEditor | null>>
}) {
  const [saving, setSaving] = React.useState(false)
  if (!editor) return null
  const set = <K extends keyof DepartmentEditor>(
    key: K,
    value: DepartmentEditor[K]
  ) =>
    setEditor((current) => (current ? { ...current, [key]: value } : current))
  const submit = async (event: React.FormEvent) => {
    event.preventDefault()
    setSaving(true)
    try {
      const input = {
        name: editor.name,
        code: editor.code,
        description: editor.description,
        parentId: editor.parentId,
        leaderAgentId: editor.leaderAgentId,
        enabled: editor.enabled,
      }
      if (editor.id) await updateDepartment(editor.id, input)
      else await createDepartment(input)
      await onSaved()
      onClose()
      toast.success("部门已保存")
    } catch (reason) {
      toast.error(reason instanceof Error ? reason.message : "保存部门失败")
    } finally {
      setSaving(false)
    }
  }
  return (
    <Dialog open onOpenChange={(open) => !open && onClose()}>
      <DialogContent>
        <form onSubmit={submit} className="contents">
          <DialogHeader>
            <DialogTitle>{editor.id ? "编辑部门" : "新增部门"}</DialogTitle>
            <DialogDescription>
              维护部门层级、负责人和职责范围。
            </DialogDescription>
          </DialogHeader>
          <FieldGroup>
            <Field>
              <FieldLabel>部门名称</FieldLabel>
              <Input
                value={editor.name}
                onChange={(e) => set("name", e.target.value)}
              />
            </Field>
            <Field>
              <FieldLabel>部门编码</FieldLabel>
              <Input
                value={editor.code}
                onChange={(e) => set("code", e.target.value)}
              />
            </Field>
            <Field>
              <FieldLabel>上级部门</FieldLabel>
              <Select
                value={editor.parentId || "none"}
                onValueChange={(value) =>
                  set("parentId", value === "none" ? "" : String(value))
                }
              >
                <SelectTrigger className="w-full">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="none">顶级部门</SelectItem>
                  {departments
                    .filter((item) => item.id !== editor.id)
                    .map((item) => (
                      <SelectItem key={item.id} value={item.id}>
                        {item.name}
                      </SelectItem>
                    ))}
                </SelectContent>
              </Select>
            </Field>
            <Field>
              <FieldLabel>部门负责人</FieldLabel>
              <Select
                value={editor.leaderAgentId || "none"}
                onValueChange={(value) =>
                  set("leaderAgentId", value === "none" ? "" : String(value))
                }
              >
                <SelectTrigger className="w-full">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="none">暂不设置</SelectItem>
                  {employees.map((employee) => (
                    <SelectItem key={employee.id} value={employee.id}>
                      {employee.name} · {employee.englishName}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </Field>
            <Field>
              <FieldLabel>部门职责</FieldLabel>
              <Textarea
                rows={4}
                value={editor.description}
                onChange={(e) => set("description", e.target.value)}
              />
            </Field>
          </FieldGroup>
          <DialogFooter>
            <Button type="button" variant="outline" onClick={onClose}>
              取消
            </Button>
            <Button
              type="submit"
              disabled={saving || !editor.name.trim() || !editor.code.trim()}
            >
              {saving ? "保存中…" : "保存部门"}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}

function PositionDialog({
  position,
  departmentID,
  templates,
  onClose,
  onSaved,
}: {
  position: Position | "new" | null
  departmentID: string
  templates: AgentTemplate[]
  onClose: () => void
  onSaved: () => Promise<void>
}) {
  const [form, setForm] = React.useState<SavePositionInput | null>(() => {
    if (!position) return null
    if (position === "new") {
      const template = templates.find((item) => !item.hidden)
      return {
        departmentId: departmentID,
        templateId: template?.id ?? "",
        name: "",
        englishName: "",
        code: "",
        description: "",
        avatar: "briefcase-business",
        category: "general",
        model: {
          provider: template?.provider ?? "",
          model: template?.model ?? "",
          baseUrl: "",
          thinking: "",
          pricing: null,
        },
        systemPrompt: template?.systemPrompt ?? "",
        tools: [...defaultTools],
        skillIds: [],
        knowledgeBaseIds: [],
        permissions: {
          workspaceScope: "run_workspace",
          allowNetwork: true,
          allowShell: true,
          allowWrite: true,
          approvalMode: "all",
          reworkApprovalMode: "all",
        },
        enabled: true,
      }
    }
    const {
      id: _id,
      createdAt: _createdAt,
      updatedAt: _updatedAt,
      ...input
    } = position
    void _id
    void _createdAt
    void _updatedAt
    return {
      ...input,
      model: { ...input.model },
      tools: [...input.tools],
      skillIds: [...input.skillIds],
      knowledgeBaseIds: [...input.knowledgeBaseIds],
      permissions: { ...input.permissions },
    }
  })
  const [saving, setSaving] = React.useState(false)
  if (!position || !form) return null
  const set = <K extends keyof SavePositionInput>(
    key: K,
    value: SavePositionInput[K]
  ) => setForm((current) => (current ? { ...current, [key]: value } : current))
  const selectTemplate = (templateID: string) => {
    const template = templates.find((item) => item.id === templateID)
    if (!template) return
    setForm((current) =>
      current
        ? {
            ...current,
            templateId: template.id,
            model: {
              ...current.model,
              provider: template.provider,
              model: template.model,
            },
            systemPrompt: template.systemPrompt,
          }
        : current
    )
  }
  const submit = async (event: React.FormEvent) => {
    event.preventDefault()
    setSaving(true)
    try {
      if (position === "new") await createPosition(form)
      else await updatePosition(position.id, form)
      await onSaved()
      onClose()
      toast.success("岗位已保存")
    } catch (reason) {
      toast.error(reason instanceof Error ? reason.message : "保存岗位失败")
    } finally {
      setSaving(false)
    }
  }
  return (
    <Dialog open onOpenChange={(open) => !open && onClose()}>
      <DialogContent className="sm:max-w-2xl">
        <form onSubmit={submit} className="contents">
          <DialogHeader>
            <DialogTitle>
              {position === "new" ? "新增岗位" : "编辑岗位"}
            </DialogTitle>
            <DialogDescription>
              岗位保存招聘时继承的默认身份、模型、能力和权限。
            </DialogDescription>
          </DialogHeader>
          <FieldGroup>
            <div className="grid gap-4 sm:grid-cols-2">
              <Field>
                <FieldLabel>岗位中文名</FieldLabel>
                <Input
                  value={form.name}
                  onChange={(e) => set("name", e.target.value)}
                />
              </Field>
              <Field>
                <FieldLabel>岗位英文名</FieldLabel>
                <Input
                  value={form.englishName}
                  onChange={(e) => set("englishName", e.target.value)}
                />
              </Field>
              <Field>
                <FieldLabel>岗位编码</FieldLabel>
                <Input
                  value={form.code}
                  onChange={(e) => set("code", e.target.value)}
                  placeholder="backend-engineer"
                />
              </Field>
              <Field>
                <FieldLabel>能力类别</FieldLabel>
                <Input
                  value={form.category}
                  onChange={(e) => set("category", e.target.value)}
                />
              </Field>
            </div>
            <Field>
              <FieldLabel>人才能力模板</FieldLabel>
              <Select
                value={form.templateId}
                onValueChange={(value) => selectTemplate(String(value))}
              >
                <SelectTrigger className="w-full">
                  <SelectValue placeholder="选择模板" />
                </SelectTrigger>
                <SelectContent>
                  <SelectGroup>
                    {templates
                      .filter((item) => !item.hidden)
                      .map((template) => (
                        <SelectItem key={template.id} value={template.id}>
                          {template.metadata.chineseName} ·{" "}
                          {template.metadata.englishName}
                        </SelectItem>
                      ))}
                  </SelectGroup>
                </SelectContent>
              </Select>
            </Field>
            <Field>
              <FieldLabel>岗位职责</FieldLabel>
              <Textarea
                rows={4}
                value={form.description}
                onChange={(e) => set("description", e.target.value)}
              />
            </Field>
          </FieldGroup>
          <DialogFooter>
            <Button type="button" variant="outline" onClick={onClose}>
              取消
            </Button>
            <Button
              type="submit"
              disabled={
                saving ||
                !form.name.trim() ||
                !form.englishName.trim() ||
                !form.code.trim() ||
                !form.templateId
              }
            >
              {saving ? "保存中…" : "保存岗位"}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}

function HireDialog({
  position,
  employees,
  defaultManagerID,
  onClose,
  onSaved,
  onCreated,
}: {
  position: Position | null
  employees: AgentDefinition[]
  defaultManagerID: string
  onClose: () => void
  onSaved: () => Promise<void>
  onCreated: (employee: AgentDefinition) => void
}) {
  const [name, setName] = React.useState("")
  const [englishName, setEnglishName] = React.useState("")
  const [managerID, setManagerID] = React.useState(defaultManagerID)
  const [saving, setSaving] = React.useState(false)
  if (!position) return null
  const submit = async (event: React.FormEvent) => {
    event.preventDefault()
    setSaving(true)
    try {
      const employee = await hireEmployee(position.id, {
        name,
        englishName,
        managerAgentId: managerID,
      })
      await onSaved()
      onClose()
      toast.success(`${name} 已入职，可继续调整个人配置`)
      onCreated(employee)
    } catch (reason) {
      toast.error(reason instanceof Error ? reason.message : "招聘员工失败")
    } finally {
      setSaving(false)
    }
  }
  return (
    <Dialog open onOpenChange={(open) => !open && onClose()}>
      <DialogContent>
        <form onSubmit={submit} className="contents">
          <DialogHeader>
            <DialogTitle>招聘{position.name}</DialogTitle>
            <DialogDescription>
              员工会继承岗位配置，并获得独立 Agent ID 与长期会话。
            </DialogDescription>
          </DialogHeader>
          <FieldGroup>
            <Field>
              <FieldLabel>中文姓名</FieldLabel>
              <Input
                autoFocus
                value={name}
                onChange={(e) => setName(e.target.value)}
                placeholder="例如：张三"
              />
            </Field>
            <Field>
              <FieldLabel>英文姓名</FieldLabel>
              <Input
                value={englishName}
                onChange={(e) => setEnglishName(e.target.value)}
                placeholder="例如：Michael Zhang"
              />
            </Field>
            <Field>
              <FieldLabel>直属上级</FieldLabel>
              <Select
                value={managerID || "none"}
                onValueChange={(value) =>
                  setManagerID(value === "none" ? "" : String(value))
                }
              >
                <SelectTrigger className="w-full">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="none">暂不设置</SelectItem>
                  {employees.map((employee) => (
                    <SelectItem key={employee.id} value={employee.id}>
                      {employee.name} · {employee.englishName}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </Field>
          </FieldGroup>
          <DialogFooter>
            <Button type="button" variant="outline" onClick={onClose}>
              取消
            </Button>
            <Button
              type="submit"
              disabled={saving || !name.trim() || !englishName.trim()}
            >
              <UserPlus />
              {saving ? "创建中…" : "确认入职"}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}

function SummaryCard({
  icon: Icon,
  label,
  value,
}: {
  icon: typeof Network
  label: string
  value: number
}) {
  return (
    <Card>
      <CardContent className="flex items-center gap-3 py-4">
        <span className="flex size-9 items-center justify-center rounded-lg bg-muted">
          <Icon className="size-4" />
        </span>
        <span>
          <span className="block text-2xl font-semibold tabular-nums">
            {value}
          </span>
          <span className="text-xs text-muted-foreground">{label}</span>
        </span>
      </CardContent>
    </Card>
  )
}
