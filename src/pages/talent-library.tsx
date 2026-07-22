import * as React from "react"
import {
  Copy,
  Eye,
  EyeOff,
  FileText,
  Plus,
  Save,
  Search,
  UserCheck,
  UserPlus,
  Users,
} from "lucide-react"
import { Link } from "react-router-dom"
import { toast } from "sonner"

import { PageHeader } from "@/components/page-header"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import {
  Card,
  CardContent,
  CardFooter,
  CardHeader,
  CardTitle,
} from "@/components/ui/card"
import { Input } from "@/components/ui/input"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import {
  Field,
  FieldDescription,
  FieldGroup,
  FieldLabel,
} from "@/components/ui/field"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import { Separator } from "@/components/ui/separator"
import { Spinner } from "@/components/ui/spinner"
import { Textarea } from "@/components/ui/textarea"
import {
  createAgentTemplate,
  fetchAgentTemplates,
  setAgentTemplateHidden,
  updateAgentTemplateNote,
} from "@/lib/api"
import type { SaveAgentTemplateInput } from "@/lib/api"
import { formatTime } from "@/lib/format"
import { useAppState } from "@/lib/state"
import { cn } from "@/lib/utils"
import type { AgentTemplate } from "@/types"

type TalentEditorState = {
  mode: "create" | "copy"
  sourceName?: string
  provider: string
  model: string
  systemPrompt: string
  note: string
  englishName: string
  chineseName: string
  introduction: string
  positions: string
}

const blankTalentEditor = (): TalentEditorState => ({
  mode: "create",
  provider: "",
  model: "",
  systemPrompt: "",
  note: "",
  englishName: "",
  chineseName: "",
  introduction: "",
  positions: "",
})

const copyTalentEditor = (template: AgentTemplate): TalentEditorState => ({
  mode: "copy",
  sourceName: template.metadata.chineseName,
  provider: template.provider,
  model: template.model,
  systemPrompt: template.systemPrompt,
  note: template.note ?? "",
  englishName: template.metadata.englishName,
  chineseName: template.metadata.chineseName,
  introduction: template.metadata.introduction,
  positions: template.metadata.positions.join(", "),
})

export function TalentLibraryPage() {
  const { state } = useAppState()
  const [templates, setTemplates] = React.useState<AgentTemplate[]>([])
  const [detailTemplate, setDetailTemplate] =
    React.useState<AgentTemplate | null>(null)
  const [editor, setEditor] = React.useState<TalentEditorState | null>(null)
  const [savingTalent, setSavingTalent] = React.useState(false)
  const [noteDraft, setNoteDraft] = React.useState("")
  const [savingNote, setSavingNote] = React.useState(false)
  const [query, setQuery] = React.useState("")
  const [provider, setProvider] = React.useState("all")
  const [position, setPosition] = React.useState("all")
  const [showHidden, setShowHidden] = React.useState(false)
  const load = React.useCallback(
    async () => setTemplates(await fetchAgentTemplates()),
    []
  )
  React.useEffect(() => {
    let active = true
    void fetchAgentTemplates().then((items) => {
      if (active) setTemplates(items)
    })
    return () => {
      active = false
    }
  }, [])
  const providers = [...new Set(templates.map((item) => item.provider))].sort()
  const positions = [
    ...new Set(templates.flatMap((item) => item.metadata.positions)),
  ].sort()
  const normalized = query.trim().toLowerCase()
  const hiredTemplateIDs = new Set(
    (state?.agents ?? []).map((agent) => agent.templateId)
  )
  const hiredAgents = (state?.agents ?? []).filter(
    (agent) => agent.templateId === detailTemplate?.id
  )
  const visible = templates.filter(
    (item) =>
      (showHidden || !item.hidden) &&
      (provider === "all" || item.provider === provider) &&
      (position === "all" || item.metadata.positions.includes(position)) &&
      (!normalized ||
        [
          item.id,
          item.provider,
          item.model,
          item.metadata.englishName,
          item.metadata.chineseName,
          item.metadata.introduction,
          item.note ?? "",
          ...item.metadata.positions,
        ].some((value) => value.toLowerCase().includes(normalized)))
  )
  const toggleHidden = async (item: AgentTemplate) => {
    try {
      await setAgentTemplateHidden(item.id, !item.hidden)
      await load()
      toast.success(item.hidden ? "已恢复显示" : "已标记为不看")
    } catch (reason) {
      toast.error(reason instanceof Error ? reason.message : "更新失败")
    }
  }
  const openDetail = (item: AgentTemplate) => {
    setDetailTemplate(item)
    setNoteDraft(item.note ?? "")
  }
  const saveTalent = async (event: React.FormEvent) => {
    event.preventDefault()
    if (!editor || savingTalent) return
    const positions = [
      ...new Set(
        editor.positions
          .split(/[,，\n]/)
          .map((value) => value.trim())
          .filter(Boolean)
      ),
    ]
    if (positions.length === 0) {
      toast.error("请至少填写一个职位")
      return
    }
    const input: SaveAgentTemplateInput = {
      provider: editor.provider,
      model: editor.model,
      systemPrompt: editor.systemPrompt,
      note: editor.note,
      metadata: {
        englishName: editor.englishName,
        chineseName: editor.chineseName,
        introduction: editor.introduction,
        positions,
      },
    }
    setSavingTalent(true)
    try {
      await createAgentTemplate(input)
      await load()
      toast.success(editor.mode === "copy" ? "新人才已生成" : "人才已添加")
      setEditor(null)
    } catch (reason) {
      toast.error(reason instanceof Error ? reason.message : "保存人才失败")
    } finally {
      setSavingTalent(false)
    }
  }
  const saveNote = async () => {
    if (!detailTemplate || savingNote) return
    setSavingNote(true)
    try {
      const updated = await updateAgentTemplateNote(
        detailTemplate.id,
        noteDraft
      )
      setTemplates((current) =>
        current.map((item) => (item.id === updated.id ? updated : item))
      )
      setDetailTemplate(updated)
      setNoteDraft(updated.note ?? "")
      toast.success("人才备注已保存")
    } catch (reason) {
      toast.error(reason instanceof Error ? reason.message : "保存备注失败")
    } finally {
      setSavingNote(false)
    }
  }
  return (
    <div className="flex flex-col gap-7">
      <PageHeader
        eyebrow="Agent marketplace"
        title="人才库"
        description="浏览可复用的人才模板；厂商、模型和系统提示词共同确定模板身份。"
        actions={
          <Button type="button" onClick={() => setEditor(blankTalentEditor())}>
            <Plus data-icon="inline-start" />
            添加人才
          </Button>
        }
      />
      <div className="grid gap-3 rounded-xl border bg-card p-4 md:grid-cols-[minmax(0,1fr)_180px_180px_auto]">
        <div className="relative">
          <Search className="absolute top-1/2 left-3 size-4 -translate-y-1/2 text-muted-foreground" />
          <Input
            className="pl-9"
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            placeholder="搜索名称、介绍、职位、模型或 ID"
          />
        </div>
        <Select value={provider} onValueChange={(v) => setProvider(String(v))}>
          <SelectTrigger className="w-full">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="all">全部厂商</SelectItem>
            {providers.map((v) => (
              <SelectItem key={v} value={v}>
                {v}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
        <Select value={position} onValueChange={(v) => setPosition(String(v))}>
          <SelectTrigger className="w-full">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="all">全部职位</SelectItem>
            {positions.map((v) => (
              <SelectItem key={v} value={v}>
                {v}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
        <Button
          type="button"
          variant={showHidden ? "secondary" : "outline"}
          onClick={() => setShowHidden((v) => !v)}
        >
          {showHidden ? (
            <Eye data-icon="inline-start" />
          ) : (
            <EyeOff data-icon="inline-start" />
          )}
          {showHidden ? "隐藏已显示" : "查看不看的"}
        </Button>
      </div>
      {visible.length ? (
        <div className="grid gap-5 lg:grid-cols-2 xl:grid-cols-3">
          {visible.map((item) => (
            <Card
              key={item.id}
              className={item.hidden ? "opacity-60" : undefined}
            >
              <CardHeader>
                <div className="flex items-start gap-3">
                  <span className="flex size-10 items-center justify-center rounded-xl bg-muted">
                    <Users className="size-5" />
                  </span>
                  <div className="min-w-0">
                    <CardTitle>{item.metadata.chineseName}</CardTitle>
                    <p className="mt-1 text-xs text-muted-foreground">
                      {item.metadata.englishName}
                    </p>
                  </div>
                </div>
              </CardHeader>
              <CardContent className="flex flex-col gap-4">
                <p className="line-clamp-3 text-sm text-muted-foreground">
                  {item.metadata.introduction}
                </p>
                <div className="flex flex-wrap gap-2">
                  {item.metadata.positions.map((v) => (
                    <Badge key={v} variant="secondary">
                      {v}
                    </Badge>
                  ))}
                </div>
                <div className="rounded-lg bg-muted/50 p-3 text-xs">
                  <div>
                    {item.provider} / {item.model}
                  </div>
                  <div className="mt-1 truncate font-mono text-muted-foreground">
                    {item.id}
                  </div>
                </div>
              </CardContent>
              <CardFooter className="flex-wrap justify-between gap-2">
                <Button
                  variant="ghost"
                  size="sm"
                  onClick={() => void toggleHidden(item)}
                >
                  {item.hidden ? (
                    <Eye data-icon="inline-start" />
                  ) : (
                    <EyeOff data-icon="inline-start" />
                  )}
                  {item.hidden ? "恢复显示" : "不看"}
                </Button>
                <div className="flex flex-wrap items-center justify-end gap-2">
                  <Button
                    type="button"
                    variant="outline"
                    size="sm"
                    onClick={() => setEditor(copyTalentEditor(item))}
                  >
                    <Copy data-icon="inline-start" />
                    复制
                  </Button>
                  <Button
                    type="button"
                    variant="outline"
                    size="sm"
                    onClick={() => openDetail(item)}
                  >
                    <FileText data-icon="inline-start" />
                    查看详情
                  </Button>
                  {hiredTemplateIDs.has(item.id) ? (
                    <Badge variant="secondary">
                      <UserCheck data-icon="inline-start" />
                      已聘用
                    </Badge>
                  ) : (
                    <Button
                      size="sm"
                      render={
                        <Link
                          to={`/agents?template=${encodeURIComponent(item.id)}`}
                        />
                      }
                    >
                      <UserPlus data-icon="inline-start" />
                      聘用
                    </Button>
                  )}
                </div>
              </CardFooter>
            </Card>
          ))}
        </div>
      ) : (
        <Card>
          <CardContent className="py-16 text-center text-sm text-muted-foreground">
            没有符合条件的人才模板。
          </CardContent>
        </Card>
      )}
      <Dialog
        open={detailTemplate !== null}
        onOpenChange={(open) => {
          if (!open) setDetailTemplate(null)
        }}
      >
        <DialogContent className="flex max-h-[calc(100dvh-2rem)] flex-col overflow-hidden sm:max-h-[90dvh] sm:max-w-4xl">
          <DialogHeader className="shrink-0 pr-8">
            <DialogTitle>
              {detailTemplate?.metadata.chineseName ?? "人才详情"}
            </DialogTitle>
            <DialogDescription>
              {detailTemplate
                ? `${detailTemplate.metadata.englishName} · ${detailTemplate.metadata.introduction}`
                : "人才模板的完整身份和能力信息"}
            </DialogDescription>
          </DialogHeader>
          <div className="flex min-h-0 flex-1 flex-col gap-5 overflow-y-auto overscroll-contain pr-2">
            <section className="flex flex-col gap-3">
              <div className="flex flex-wrap items-center gap-2">
                {detailTemplate?.metadata.positions.map((item) => (
                  <Badge key={item} variant="secondary">
                    {item}
                  </Badge>
                ))}
                <Badge
                  variant={detailTemplate?.builtin ? "secondary" : "outline"}
                >
                  {detailTemplate?.builtin ? "系统内置" : "自定义人才"}
                </Badge>
                {detailTemplate?.hidden ? (
                  <Badge variant="outline">已隐藏</Badge>
                ) : null}
                <Badge
                  variant={hiredAgents.length > 0 ? "secondary" : "outline"}
                >
                  {hiredAgents.length > 0
                    ? `已聘用 ${hiredAgents.length} 人`
                    : "尚未聘用"}
                </Badge>
              </div>
              <dl className="grid gap-3 rounded-lg bg-muted/50 p-4 text-sm sm:grid-cols-2">
                <TalentDetail
                  label="中文名"
                  value={detailTemplate?.metadata.chineseName}
                />
                <TalentDetail
                  label="英文名"
                  value={detailTemplate?.metadata.englishName}
                />
                <TalentDetail label="厂商" value={detailTemplate?.provider} />
                <TalentDetail label="模型" value={detailTemplate?.model} />
                <TalentDetail
                  label="创建时间"
                  value={formatTime(detailTemplate?.createdAt)}
                />
                <TalentDetail
                  label="更新时间"
                  value={formatTime(detailTemplate?.updatedAt)}
                />
                <TalentDetail
                  className="sm:col-span-2"
                  label="人才 ID"
                  value={detailTemplate?.id}
                  mono
                />
              </dl>
            </section>

            <section className="flex flex-col gap-2">
              <h3 className="text-sm font-medium">个人介绍</h3>
              <p className="text-sm leading-relaxed text-muted-foreground">
                {detailTemplate?.metadata.introduction}
              </p>
            </section>

            {hiredAgents.length > 0 ? (
              <section className="flex flex-col gap-2">
                <h3 className="text-sm font-medium">已聘员工</h3>
                <div className="flex flex-col gap-2">
                  {hiredAgents.map((agent) => (
                    <div
                      key={agent.id}
                      className="flex flex-wrap items-center justify-between gap-2 rounded-lg bg-muted/50 px-4 py-3"
                    >
                      <div className="min-w-0">
                        <div className="text-sm font-medium">{agent.name}</div>
                        <div className="truncate font-mono text-xs text-muted-foreground">
                          {agent.id}
                        </div>
                      </div>
                      <Badge variant={agent.enabled ? "secondary" : "outline"}>
                        {agent.enabled ? "已启用" : "已停用"}
                      </Badge>
                    </div>
                  ))}
                </div>
              </section>
            ) : null}

            <section className="flex flex-col gap-2">
              <div>
                <h3 className="text-sm font-medium">备注</h3>
                <p className="mt-1 text-xs text-muted-foreground">
                  备注由用户维护，不参与人才 ID 计算。
                </p>
              </div>
              <Textarea
                value={noteDraft}
                onChange={(event) => setNoteDraft(event.target.value)}
                placeholder="记录适用场景、使用偏好、注意事项等"
                maxLength={10000}
                className="min-h-28 resize-y"
              />
              <div className="flex flex-wrap items-center justify-between gap-2">
                <span className="text-xs text-muted-foreground">
                  {noteDraft.length} / 10000
                </span>
                <Button
                  type="button"
                  size="sm"
                  disabled={
                    savingNote ||
                    noteDraft.trim() === (detailTemplate?.note ?? "")
                  }
                  onClick={() => void saveNote()}
                >
                  {savingNote ? (
                    <Spinner data-icon="inline-start" />
                  ) : (
                    <Save data-icon="inline-start" />
                  )}
                  保存备注
                </Button>
              </div>
            </section>

            <Separator />

            <section className="flex flex-col gap-2">
              <div>
                <h3 className="text-sm font-medium">系统提示词</h3>
                <p className="mt-1 text-xs text-muted-foreground">
                  该提示词与厂商、模型共同确定人才身份。
                </p>
              </div>
              <div className="max-h-[42vh] overflow-auto rounded-lg bg-muted p-4">
                <pre className="font-mono text-xs leading-relaxed break-words whitespace-pre-wrap">
                  {detailTemplate?.systemPrompt}
                </pre>
              </div>
            </section>
          </div>
        </DialogContent>
      </Dialog>
      <TalentEditorDialog
        editor={editor}
        saving={savingTalent}
        onEditorChange={setEditor}
        onSubmit={saveTalent}
      />
    </div>
  )
}

function TalentEditorDialog({
  editor,
  saving,
  onEditorChange,
  onSubmit,
}: {
  editor: TalentEditorState | null
  saving: boolean
  onEditorChange: React.Dispatch<React.SetStateAction<TalentEditorState | null>>
  onSubmit: (event: React.FormEvent) => Promise<void>
}) {
  const set = <K extends keyof TalentEditorState>(
    key: K,
    value: TalentEditorState[K]
  ) =>
    onEditorChange((current) =>
      current ? { ...current, [key]: value } : current
    )

  return (
    <Dialog
      open={editor !== null}
      onOpenChange={(open) => {
        if (!open && !saving) onEditorChange(null)
      }}
    >
      <DialogContent className="flex max-h-[calc(100dvh-2rem)] flex-col overflow-hidden sm:max-h-[92dvh] sm:max-w-3xl">
        <form
          className="flex min-h-0 flex-1 flex-col gap-4"
          onSubmit={(event) => void onSubmit(event)}
        >
          <DialogHeader className="shrink-0 pr-8">
            <DialogTitle>
              {editor?.mode === "copy" ? "复制人才" : "添加人才"}
            </DialogTitle>
            <DialogDescription>
              {editor?.mode === "copy"
                ? `以“${editor.sourceName ?? "现有人才"}”为基础编辑并生成新人才。厂商、模型和提示词必须形成一个尚不存在的 ID。`
                : "手动填写人才身份、metadata 和备注。保存时会根据厂商、模型和提示词生成人才 ID。"}
            </DialogDescription>
          </DialogHeader>
          <div className="min-h-0 flex-1 overflow-y-auto overscroll-contain pr-2">
            <FieldGroup>
              <FieldGroup className="@md/field-group:grid @md/field-group:grid-cols-2">
                <Field>
                  <FieldLabel htmlFor="talent-provider">厂商名</FieldLabel>
                  <Input
                    id="talent-provider"
                    value={editor?.provider ?? ""}
                    onChange={(event) => set("provider", event.target.value)}
                    placeholder="例如 opencode-go"
                    required
                    autoFocus
                  />
                </Field>
                <Field>
                  <FieldLabel htmlFor="talent-model">模型名</FieldLabel>
                  <Input
                    id="talent-model"
                    value={editor?.model ?? ""}
                    onChange={(event) => set("model", event.target.value)}
                    placeholder="例如 deepseek-v4-flash"
                    required
                  />
                </Field>
              </FieldGroup>

              <FieldGroup className="@md/field-group:grid @md/field-group:grid-cols-2">
                <Field>
                  <FieldLabel htmlFor="talent-chinese-name">中文名</FieldLabel>
                  <Input
                    id="talent-chinese-name"
                    value={editor?.chineseName ?? ""}
                    onChange={(event) => set("chineseName", event.target.value)}
                    placeholder="例如 高级后端工程师"
                    required
                  />
                </Field>
                <Field>
                  <FieldLabel htmlFor="talent-english-name">英文名</FieldLabel>
                  <Input
                    id="talent-english-name"
                    value={editor?.englishName ?? ""}
                    onChange={(event) => set("englishName", event.target.value)}
                    placeholder="例如 senior-backend-engineer"
                    required
                  />
                </Field>
              </FieldGroup>

              <Field>
                <FieldLabel htmlFor="talent-positions">职位</FieldLabel>
                <Input
                  id="talent-positions"
                  value={editor?.positions ?? ""}
                  onChange={(event) => set("positions", event.target.value)}
                  placeholder="backend, architecture, golang"
                  required
                />
                <FieldDescription>
                  多个职位使用中文逗号、英文逗号或换行分隔。
                </FieldDescription>
              </Field>

              <Field>
                <FieldLabel htmlFor="talent-introduction">个人介绍</FieldLabel>
                <Textarea
                  id="talent-introduction"
                  value={editor?.introduction ?? ""}
                  onChange={(event) => set("introduction", event.target.value)}
                  placeholder="描述该人才擅长的工作、职责范围和适用场景"
                  className="min-h-24 resize-y"
                  required
                />
              </Field>

              <Field>
                <FieldLabel htmlFor="talent-system-prompt">
                  系统提示词
                </FieldLabel>
                <Textarea
                  id="talent-system-prompt"
                  value={editor?.systemPrompt ?? ""}
                  onChange={(event) => set("systemPrompt", event.target.value)}
                  placeholder="输入完整系统提示词"
                  className="min-h-56 resize-y font-mono text-xs leading-relaxed"
                  required
                />
                <FieldDescription>
                  系统提示词与厂商、模型共同决定人才
                  ID；复制后不修改这三项会产生 ID 冲突。
                </FieldDescription>
              </Field>

              <Field>
                <FieldLabel htmlFor="talent-note">备注</FieldLabel>
                <Textarea
                  id="talent-note"
                  value={editor?.note ?? ""}
                  onChange={(event) => set("note", event.target.value)}
                  placeholder="可选：记录使用偏好、适用场景和注意事项"
                  maxLength={10000}
                  className="min-h-24 resize-y"
                />
                <FieldDescription>
                  备注不会参与人才 ID 计算，最多 10000 个字符。
                </FieldDescription>
              </Field>
            </FieldGroup>
          </div>
          <DialogFooter className="shrink-0">
            <Button
              type="button"
              variant="outline"
              disabled={saving}
              onClick={() => onEditorChange(null)}
            >
              取消
            </Button>
            <Button type="submit" disabled={saving}>
              {saving ? (
                <Spinner data-icon="inline-start" />
              ) : editor?.mode === "copy" ? (
                <Copy data-icon="inline-start" />
              ) : (
                <Plus data-icon="inline-start" />
              )}
              {editor?.mode === "copy" ? "保存为新人才" : "添加人才"}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}

function TalentDetail({
  label,
  value,
  mono = false,
  className,
}: {
  label: string
  value?: string
  mono?: boolean
  className?: string
}) {
  return (
    <div className={className}>
      <dt className="text-xs text-muted-foreground">{label}</dt>
      <dd
        className={cn(
          "mt-1 break-words",
          mono ? "font-mono text-xs leading-relaxed break-all" : "font-medium"
        )}
      >
        {value || "—"}
      </dd>
    </div>
  )
}
