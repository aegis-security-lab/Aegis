import * as React from "react"
import {
  Download,
  FileArchive,
  FileCode2,
  FolderDown,
  Pencil,
  Plus,
  Trash2,
  Upload,
  Wrench,
} from "lucide-react"
import { toast } from "sonner"

import { PageHeader } from "@/components/page-header"
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
  CardHeader,
  CardTitle,
} from "@/components/ui/card"
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
} from "@/components/ui/field"
import { Input } from "@/components/ui/input"
import { Spinner } from "@/components/ui/spinner"
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table"
import { Textarea } from "@/components/ui/textarea"
import {
  createSkill,
  deleteSkill,
  exportSkill,
  importSkill,
  installSkill,
  updateSkill,
  type SaveSkillInput,
} from "@/lib/api"
import { formatTime } from "@/lib/format"
import { useAppState } from "@/lib/state"
import type { SkillDefinition } from "@/types"

export function SkillsPage() {
  const { state, refresh } = useAppState()
  const skills = state?.skills ?? []
  const agents = state?.agents ?? []
  const [editing, setEditing] = React.useState<SkillDefinition | "new" | null>(
    null
  )
  const [installOpen, setInstallOpen] = React.useState(false)
  const [deleting, setDeleting] = React.useState<SkillDefinition | null>(null)
  const [importing, setImporting] = React.useState(false)
  const fileRef = React.useRef<HTMLInputElement>(null)
  const assignments = agents.reduce(
    (total, agent) => total + agent.skillIds.length,
    0
  )

  const handleImport = async (event: React.ChangeEvent<HTMLInputElement>) => {
    const file = event.target.files?.[0]
    event.target.value = ""
    if (!file) return
    setImporting(true)
    try {
      await importSkill(file)
      await refresh()
      toast.success("Skill 已导入")
    } catch (reason) {
      toast.error(reason instanceof Error ? reason.message : "导入 Skill 失败")
    } finally {
      setImporting(false)
    }
  }

  const download = async (skill: SkillDefinition) => {
    try {
      const blob = await exportSkill(skill.id)
      const url = URL.createObjectURL(blob)
      const link = document.createElement("a")
      link.href = url
      link.download = `${skill.name}.zip`
      link.click()
      URL.revokeObjectURL(url)
    } catch (reason) {
      toast.error(reason instanceof Error ? reason.message : "导出 Skill 失败")
    }
  }

  const remove = async () => {
    if (!deleting) return
    try {
      await deleteSkill(deleting.id)
      setDeleting(null)
      await refresh()
      toast.success("Skill 已删除")
    } catch (reason) {
      toast.error(reason instanceof Error ? reason.message : "删除 Skill 失败")
    }
  }

  return (
    <div className="flex flex-col gap-7">
      <PageHeader
        eyebrow="Capability library"
        title="Skills"
        actions={
          <>
            <input
              ref={fileRef}
              type="file"
              className="hidden"
              accept=".zip,.md,text/markdown,application/zip"
              onChange={(event) => void handleImport(event)}
            />
            <Button
              variant="outline"
              disabled={importing}
              onClick={() => fileRef.current?.click()}
            >
              {importing ? (
                <Spinner data-icon="inline-start" />
              ) : (
                <Upload data-icon="inline-start" />
              )}
              导入
            </Button>
            <Button variant="outline" onClick={() => setInstallOpen(true)}>
              <FolderDown data-icon="inline-start" />
              安装
            </Button>
            <Button onClick={() => setEditing("new")}>
              <Plus data-icon="inline-start" />
              新增 Skill
            </Button>
          </>
        }
      />

      <div className="grid gap-4 sm:grid-cols-3">
        <Summary icon={Wrench} label="Skill 总数" value={skills.length} />
        <Summary
          icon={FileArchive}
          label="内置 Skills"
          value={skills.filter((skill) => skill.builtin).length}
        />
        <Summary icon={FileCode2} label="Agent 引用" value={assignments} />
      </div>

      <Card>
        <CardHeader>
          <CardTitle>能力目录</CardTitle>
        </CardHeader>
        <CardContent>
          {skills.length === 0 ? (
            <Empty className="py-12">
              <EmptyHeader>
                <EmptyMedia variant="icon">
                  <Wrench />
                </EmptyMedia>
                <EmptyTitle>还没有 Skill</EmptyTitle>
                <EmptyDescription>
                  创建 SKILL.md，或导入一个现有 Skill 包。
                </EmptyDescription>
              </EmptyHeader>
            </Empty>
          ) : (
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>Skill</TableHead>
                  <TableHead>来源</TableHead>
                  <TableHead>版本</TableHead>
                  <TableHead>Agent 引用</TableHead>
                  <TableHead>更新</TableHead>
                  <TableHead className="text-right">操作</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {skills.map((skill) => {
                  const usedBy = agents.filter((agent) =>
                    agent.skillIds.includes(skill.id)
                  )
                  return (
                    <TableRow key={skill.id}>
                      <TableCell className="min-w-64 whitespace-normal">
                        <div className="flex flex-col gap-1">
                          <div className="flex items-center gap-2">
                            <span className="font-medium">
                              {skill.displayName}
                            </span>
                            {skill.builtin ? (
                              <Badge variant="outline">内置</Badge>
                            ) : null}
                          </div>
                          <span className="line-clamp-2 text-xs text-muted-foreground">
                            {skill.description}
                          </span>
                          <span className="font-mono text-xs text-muted-foreground">
                            {skill.name}
                          </span>
                        </div>
                      </TableCell>
                      <TableCell>
                        <Badge variant="secondary">
                          {sourceLabel(skill.source)}
                        </Badge>
                      </TableCell>
                      <TableCell className="font-mono text-xs">
                        {skill.version}
                      </TableCell>
                      <TableCell>
                        <span className="text-sm">{usedBy.length}</span>
                      </TableCell>
                      <TableCell className="text-xs text-muted-foreground">
                        {formatTime(skill.updatedAt)}
                      </TableCell>
                      <TableCell>
                        <div className="flex justify-end gap-1">
                          <Button
                            variant="ghost"
                            size="icon-sm"
                            aria-label={`导出 ${skill.displayName}`}
                            onClick={() => void download(skill)}
                          >
                            <Download />
                          </Button>
                          <Button
                            variant="ghost"
                            size="icon-sm"
                            aria-label={`编辑 ${skill.displayName}`}
                            onClick={() => setEditing(skill)}
                          >
                            <Pencil />
                          </Button>
                          {!skill.builtin ? (
                            <Button
                              variant="ghost"
                              size="icon-sm"
                              aria-label={`删除 ${skill.displayName}`}
                              disabled={usedBy.length > 0}
                              onClick={() => setDeleting(skill)}
                            >
                              <Trash2 />
                            </Button>
                          ) : null}
                        </div>
                      </TableCell>
                    </TableRow>
                  )
                })}
              </TableBody>
            </Table>
          )}
        </CardContent>
      </Card>

      {editing ? (
        <SkillDialog
          key={editing === "new" ? "new" : editing.id}
          skill={editing}
          onOpenChange={(open) => {
            if (!open) setEditing(null)
          }}
          onSaved={async () => {
            setEditing(null)
            await refresh()
          }}
        />
      ) : null}
      <InstallDialog
        open={installOpen}
        onOpenChange={setInstallOpen}
        onInstalled={async () => {
          setInstallOpen(false)
          await refresh()
        }}
      />
      <AlertDialog
        open={deleting !== null}
        onOpenChange={(open) => !open && setDeleting(null)}
      >
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogMedia>
              <Trash2 />
            </AlertDialogMedia>
            <AlertDialogTitle>删除 {deleting?.displayName}？</AlertDialogTitle>
            <AlertDialogDescription>
              SKILL.md 及本地物化目录都会删除。正在被 Agent 使用的 Skill
              无法删除。
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>取消</AlertDialogCancel>
            <AlertDialogAction
              variant="destructive"
              onClick={() => void remove()}
            >
              删除
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  )
}

function SkillDialog({
  skill,
  onOpenChange,
  onSaved,
}: {
  skill: SkillDefinition | "new"
  onOpenChange: (open: boolean) => void
  onSaved: () => Promise<void>
}) {
  const [form, setForm] = React.useState<SaveSkillInput>(() =>
    skill === "new" ? blankSkill() : skillInput(skill)
  )
  const [saving, setSaving] = React.useState(false)

  const set = <K extends keyof SaveSkillInput>(
    key: K,
    value: SaveSkillInput[K]
  ) => setForm((current) => ({ ...current, [key]: value }))

  const submit = async (event: React.FormEvent) => {
    event.preventDefault()
    if (!form.name.trim() || !form.description.trim() || saving) return
    setSaving(true)
    try {
      if (skill === "new") await createSkill(form)
      else await updateSkill(skill.id, form)
      toast.success(skill === "new" ? "Skill 已创建" : "Skill 已保存")
      await onSaved()
    } catch (reason) {
      toast.error(reason instanceof Error ? reason.message : "保存 Skill 失败")
    } finally {
      setSaving(false)
    }
  }

  return (
    <Dialog open onOpenChange={onOpenChange}>
      <DialogContent className="max-h-[92svh] overflow-y-auto sm:max-w-3xl">
        <form onSubmit={submit} className="contents">
          <DialogHeader>
            <DialogTitle>
              {skill === "new" ? "新增 Skill" : `编辑 ${form.displayName}`}
            </DialogTitle>
            <DialogDescription>
              使用标准 SKILL.md frontmatter：只包含 name 与
              description，正文保持简洁、可执行。
            </DialogDescription>
          </DialogHeader>
          <FieldGroup>
            <div className="grid gap-5 sm:grid-cols-2">
              <Field>
                <FieldLabel htmlFor="skill-name">name</FieldLabel>
                <Input
                  id="skill-name"
                  value={form.name}
                  disabled={skill !== "new"}
                  placeholder="review-release"
                  onChange={(event) => set("name", event.target.value)}
                />
              </Field>
              <Field>
                <FieldLabel htmlFor="skill-display-name">显示名称</FieldLabel>
                <Input
                  id="skill-display-name"
                  value={form.displayName}
                  onChange={(event) => set("displayName", event.target.value)}
                />
              </Field>
              <Field>
                <FieldLabel htmlFor="skill-version">版本</FieldLabel>
                <Input
                  id="skill-version"
                  value={form.version}
                  onChange={(event) => set("version", event.target.value)}
                />
              </Field>
              <Field>
                <FieldLabel htmlFor="skill-source">来源</FieldLabel>
                <Input id="skill-source" value={form.source} disabled />
              </Field>
            </div>
            <Field>
              <FieldLabel htmlFor="skill-description">description</FieldLabel>
              <Textarea
                id="skill-description"
                rows={3}
                value={form.description}
                onChange={(event) => set("description", event.target.value)}
              />
              <FieldDescription>
                用于 Agent 判断何时加载这个 Skill。
              </FieldDescription>
            </Field>
            <Field>
              <FieldLabel htmlFor="skill-content">SKILL.md</FieldLabel>
              <Textarea
                id="skill-content"
                className="min-h-80 font-mono text-xs leading-5"
                value={form.content}
                placeholder="创建后会自动生成 frontmatter 和工作流模板。"
                onChange={(event) => set("content", event.target.value)}
              />
            </Field>
          </FieldGroup>
          <DialogFooter>
            <Button
              type="button"
              variant="outline"
              onClick={() => onOpenChange(false)}
            >
              取消
            </Button>
            <Button
              type="submit"
              disabled={saving || !form.name.trim() || !form.description.trim()}
            >
              {saving ? <Spinner data-icon="inline-start" /> : null}
              {saving ? "保存中…" : "保存 Skill"}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}

function InstallDialog({
  open,
  onOpenChange,
  onInstalled,
}: {
  open: boolean
  onOpenChange: (open: boolean) => void
  onInstalled: () => Promise<void>
}) {
  const [source, setSource] = React.useState("")
  const [installing, setInstalling] = React.useState(false)
  const submit = async (event: React.FormEvent) => {
    event.preventDefault()
    if (!source.trim() || installing) return
    setInstalling(true)
    try {
      await installSkill(source.trim())
      setSource("")
      toast.success("Skill 已安装")
      await onInstalled()
    } catch (reason) {
      toast.error(reason instanceof Error ? reason.message : "安装 Skill 失败")
    } finally {
      setInstalling(false)
    }
  }
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <form onSubmit={submit} className="contents">
          <DialogHeader>
            <DialogTitle>从本地安装 Skill</DialogTitle>
            <DialogDescription>
              填写包含 SKILL.md 的目录，或一个 .md / .zip
              文件路径。安装时会复制到 Aegis 数据目录。
            </DialogDescription>
          </DialogHeader>
          <FieldGroup>
            <Field>
              <FieldLabel htmlFor="skill-install-source">本地来源</FieldLabel>
              <Input
                id="skill-install-source"
                value={source}
                placeholder="/path/to/my-skill"
                onChange={(event) => setSource(event.target.value)}
              />
            </Field>
          </FieldGroup>
          <DialogFooter>
            <Button
              type="button"
              variant="outline"
              onClick={() => onOpenChange(false)}
            >
              取消
            </Button>
            <Button type="submit" disabled={!source.trim() || installing}>
              {installing ? (
                <Spinner data-icon="inline-start" />
              ) : (
                <FolderDown data-icon="inline-start" />
              )}
              {installing ? "安装中…" : "安装"}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}

function Summary({
  icon: Icon,
  label,
  value,
}: {
  icon: typeof Wrench
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

function blankSkill(): SaveSkillInput {
  return {
    id: "",
    name: "",
    displayName: "",
    description: "",
    content: "",
    source: "local",
    version: "1.0.0",
  }
}

function skillInput(skill: SkillDefinition): SaveSkillInput {
  return {
    id: skill.id,
    name: skill.name,
    displayName: skill.displayName,
    description: skill.description,
    content: skill.content,
    source: skill.source,
    version: skill.version,
  }
}

function sourceLabel(source: string) {
  if (source === "builtin") return "内置"
  if (source.startsWith("import:")) return "导入"
  if (source.startsWith("local:")) return "本地安装"
  return "本地创建"
}
