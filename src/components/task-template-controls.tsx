import * as React from "react"
import { Bookmark, Check, Save, Search, Trash2 } from "lucide-react"
import { toast } from "sonner"

import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from "@/components/ui/popover"
import {
  deleteTaskTemplate,
  fetchTaskTemplates,
  saveTaskTemplate,
} from "@/lib/api"
import { limitIssueTitle } from "@/lib/issue-title"
import { cn } from "@/lib/utils"
import type { CreateIssueInput, TaskTemplate } from "@/types"

export function TaskTemplateControls({
  form,
  selectedAgentId,
  objectiveEnabled,
  onApply,
}: {
  form: CreateIssueInput
  selectedAgentId: string
  objectiveEnabled: boolean
  onApply: (template: TaskTemplate) => void
}) {
  const [templates, setTemplates] = React.useState<TaskTemplate[]>([])
  const [query, setQuery] = React.useState("")
  const [open, setOpen] = React.useState(false)
  const [busy, setBusy] = React.useState(false)
  const [selectedId, setSelectedId] = React.useState("")

  const load = React.useCallback(async () => {
    const result = await fetchTaskTemplates()
    setTemplates(result.templates)
  }, [])

  React.useEffect(() => {
    // Initial remote data belongs in this mount synchronization effect.
    // eslint-disable-next-line react-hooks/set-state-in-effect
    void load().catch((error) =>
      toast.error(error instanceof Error ? error.message : "读取任务模板失败")
    )
  }, [load])

  const visible = templates.filter((template) => {
    const needle = query.trim().toLocaleLowerCase()
    return (
      !needle ||
      template.title.toLocaleLowerCase().includes(needle) ||
      template.description.toLocaleLowerCase().includes(needle)
    )
  })
  const selected = templates.find((template) => template.id === selectedId)

  const save = async () => {
    const description = form.description.trim()
    const title = form.title.trim() || limitIssueTitle(description)
    if (!title || !description) {
      toast.error("请先填写模板标题和任务说明")
      return
    }
    const overwriting = templates.some((template) => template.title === title)
    setBusy(true)
    try {
      const saved = await saveTaskTemplate({
        title,
        description,
        objective: objectiveEnabled ? form.objective.trim() : "",
        priority: form.priority,
        assigneeAgentId: selectedAgentId || undefined,
        timeBudgetMinutes: form.timeBudgetMinutes,
        humanValidationFallback: form.humanValidationFallback ?? false,
      })
      await load()
      setSelectedId(saved.id)
      toast.success(overwriting ? "同名模板已更新" : "任务模板已保存")
    } catch (error) {
      toast.error(error instanceof Error ? error.message : "保存模板失败")
    } finally {
      setBusy(false)
    }
  }

  const remove = async (template: TaskTemplate) => {
    try {
      await deleteTaskTemplate(template.id)
      setTemplates((current) =>
        current.filter((item) => item.id !== template.id)
      )
      if (selectedId === template.id) setSelectedId("")
      toast.success(`已删除模板“${template.title}”`)
    } catch (error) {
      toast.error(error instanceof Error ? error.message : "删除模板失败")
    }
  }

  return (
    <div className="flex min-w-0 items-center gap-2">
      <Popover open={open} onOpenChange={setOpen}>
        <PopoverTrigger
          render={
            <Button
              type="button"
              variant="outline"
              size="sm"
              className="max-w-48"
            />
          }
        >
          <Bookmark />
          <span className="truncate">{selected?.title ?? "选择模板"}</span>
        </PopoverTrigger>
        <PopoverContent
          side="top"
          align="start"
          sideOffset={8}
          className="w-80 p-1.5"
        >
          <div className="relative mb-1.5">
            <Search className="pointer-events-none absolute top-1/2 left-2.5 size-3.5 -translate-y-1/2 text-muted-foreground" />
            <Input
              autoFocus
              value={query}
              onChange={(event) => setQuery(event.target.value)}
              placeholder="搜索任务模板…"
              className="h-8 pl-8 text-xs"
            />
          </div>
          <div className="max-h-72 overflow-y-auto">
            {visible.map((template) => (
              <div key={template.id} className="group relative">
                <button
                  type="button"
                  className={cn(
                    "flex w-full min-w-0 items-start gap-2 rounded-md px-2 py-2 pr-9 text-left hover:bg-muted",
                    selectedId === template.id && "bg-muted"
                  )}
                  onClick={() => {
                    setSelectedId(template.id)
                    onApply(template)
                    setOpen(false)
                  }}
                >
                  <span className="min-w-0 flex-1">
                    <span className="block truncate text-xs font-medium">
                      {template.title}
                    </span>
                    <span className="mt-0.5 block truncate text-[11px] text-muted-foreground">
                      {template.description}
                    </span>
                  </span>
                  {selectedId === template.id ? (
                    <Check className="mt-0.5 size-3.5" />
                  ) : null}
                </button>
                <Button
                  type="button"
                  variant="ghost"
                  size="icon-xs"
                  className="absolute top-2 right-1.5 opacity-0 group-hover:opacity-100 focus-visible:opacity-100"
                  aria-label={`删除模板 ${template.title}`}
                  title="删除模板"
                  onClick={(event) => {
                    event.stopPropagation()
                    void remove(template)
                  }}
                >
                  <Trash2 />
                </Button>
              </div>
            ))}
            {visible.length === 0 ? (
              <p className="px-3 py-6 text-center text-xs text-muted-foreground">
                没有匹配的模板
              </p>
            ) : null}
          </div>
        </PopoverContent>
      </Popover>
      <Button
        type="button"
        variant="ghost"
        size="sm"
        disabled={busy}
        onClick={() => void save()}
      >
        <Save />
        保存为模板
      </Button>
    </div>
  )
}
