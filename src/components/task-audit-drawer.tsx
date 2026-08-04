import * as React from "react"
/* eslint-disable react-hooks/set-state-in-effect */
import {
  AlertTriangle,
  CheckCircle2,
  Download,
  FileSearch,
  Plus,
} from "lucide-react"
import { toast } from "sonner"

import { MarkdownContent } from "@/components/markdown-content"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
} from "@/components/ui/sheet"
import { Spinner } from "@/components/ui/spinner"
import {
  createTaskAudit,
  fetchTaskAudit,
  fetchTaskAudits,
  taskAuditReportURL,
} from "@/lib/api"
import { formatTime } from "@/lib/format"
import { cn } from "@/lib/utils"
import type { TaskAudit } from "@/types"

interface TaskAuditDrawerProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  taskId: string
  taskTitle: string
}

export function TaskAuditDrawer({
  open,
  onOpenChange,
  taskId,
  taskTitle,
}: TaskAuditDrawerProps) {
  const [audits, setAudits] = React.useState<TaskAudit[]>([])
  const [selectedId, setSelectedId] = React.useState("")
  const [loading, setLoading] = React.useState(false)
  const [creating, setCreating] = React.useState(false)

  const load = React.useCallback(async () => {
    setLoading(true)
    try {
      const result = await fetchTaskAudits(taskId)
      setAudits(result.audits)
      setSelectedId((current) =>
        current && result.audits.some((audit) => audit.id === current)
          ? current
          : (result.audits[0]?.id ?? "")
      )
    } catch (error) {
      toast.error(error instanceof Error ? error.message : "读取审计记录失败")
    } finally {
      setLoading(false)
    }
  }, [taskId])

  React.useEffect(() => {
    if (open) void load()
  }, [open, load])

  const selected = audits.find((audit) => audit.id === selectedId)

  React.useEffect(() => {
    if (!open || !selected || !["preparing", "running"].includes(selected.status))
      return
    const timer = window.setInterval(() => {
      void fetchTaskAudit(selected.id)
        .then((next) =>
          setAudits((current) =>
            current.map((audit) => (audit.id === next.id ? next : audit))
          )
        )
        .catch(() => undefined)
    }, 700)
    return () => window.clearInterval(timer)
  }, [open, selected])

  const create = async () => {
    setCreating(true)
    try {
      const audit = await createTaskAudit(taskId)
      setAudits((current) => [audit, ...current])
      setSelectedId(audit.id)
      toast.success("任务审计已启动")
    } catch (error) {
      toast.error(error instanceof Error ? error.message : "启动任务审计失败")
    } finally {
      setCreating(false)
    }
  }

  return (
    <Sheet open={open} onOpenChange={onOpenChange}>
      <SheetContent className="w-[min(96vw,960px)]! gap-0 p-0 sm:max-w-[960px]!">
        <SheetHeader className="border-b px-5 py-4 pr-14">
          <div className="flex flex-wrap items-start justify-between gap-3">
            <div className="min-w-0">
              <SheetTitle className="flex items-center gap-2">
                <FileSearch className="size-4 text-primary" />
                任务审计
              </SheetTitle>
              <SheetDescription className="mt-1 line-clamp-1">
                {taskTitle} · 基于冻结证据快照评估任务质量与平台健康度
              </SheetDescription>
            </div>
            <Button size="sm" onClick={() => void create()} disabled={creating}>
              {creating ? <Spinner /> : <Plus data-icon="inline-start" />}
              新建审计
            </Button>
          </div>
        </SheetHeader>

        <div className="grid min-h-0 flex-1 grid-cols-1 md:grid-cols-[224px_minmax(0,1fr)]">
          <aside className="max-h-48 overflow-y-auto border-b bg-muted/20 p-3 md:max-h-none md:border-r md:border-b-0">
            <p className="px-2 pb-2 text-[11px] font-medium tracking-wide text-muted-foreground uppercase">
              审计记录 · {audits.length}
            </p>
            {loading && audits.length === 0 ? (
              <div className="flex h-20 items-center justify-center"><Spinner /></div>
            ) : audits.length === 0 ? (
              <p className="rounded-md border border-dashed p-3 text-xs leading-5 text-muted-foreground">
                暂无记录。新建审计后会冻结当前任务证据并实时生成报告。
              </p>
            ) : (
              <div className="space-y-1">
                {audits.map((audit, index) => (
                  <button
                    key={audit.id}
                    type="button"
                    onClick={() => setSelectedId(audit.id)}
                    className={cn(
                      "w-full rounded-md border px-3 py-2.5 text-left transition-colors",
                      selectedId === audit.id
                        ? "border-primary/35 bg-background"
                        : "border-transparent hover:bg-background/70"
                    )}
                  >
                    <div className="flex items-center justify-between gap-2">
                      <span className="text-xs font-medium">第 {audits.length - index} 次</span>
                      <AuditStatus audit={audit} compact />
                    </div>
                    <p className="mt-1 font-mono text-[10px] text-muted-foreground">
                      {formatTime(audit.createdAt)}
                    </p>
                  </button>
                ))}
              </div>
            )}
          </aside>

          <main className="flex min-h-0 min-w-0 flex-col">
            {selected ? (
              <>
                <div className="flex flex-wrap items-center justify-between gap-3 border-b px-5 py-3">
                  <div className="flex items-center gap-2">
                    <AuditStatus audit={selected} />
                    <span className="text-xs text-muted-foreground">
                      {selected.frameworkVersion}
                    </span>
                    {!selected.evidenceComplete ? (
                      <Badge variant="outline" className="text-warning">证据快照有警告</Badge>
                    ) : null}
                  </div>
                  {selected.reportMarkdown ? (
                    <Button
                      variant="outline"
                      size="sm"
                      render={<a href={taskAuditReportURL(selected.id)} download />}
                      nativeButton={false}
                    >
                      <Download data-icon="inline-start" />
                      下载报告
                    </Button>
                  ) : null}
                </div>
                <div className="min-h-0 flex-1 overflow-y-auto bg-background px-5 py-5 md:px-8 md:py-7">
                  {selected.reportMarkdown ? (
                    <article className="mx-auto max-w-3xl border-l-2 border-primary/20 pl-5 md:pl-7">
                      <MarkdownContent className="text-sm !leading-7">
                        {selected.reportMarkdown}
                      </MarkdownContent>
                      {selected.status === "running" ? (
                        <span className="mt-3 inline-block h-4 w-0.5 animate-pulse bg-primary" aria-label="AI 正在生成" />
                      ) : null}
                    </article>
                  ) : selected.status === "failed" ? (
                    <div className="mx-auto mt-12 max-w-lg rounded-lg border border-destructive/30 bg-destructive/5 p-5">
                      <p className="flex items-center gap-2 font-medium text-destructive"><AlertTriangle className="size-4" />审计失败</p>
                      <p className="mt-2 text-sm leading-6 text-muted-foreground">{selected.error || "分析 Agent 未能生成报告。"}</p>
                    </div>
                  ) : (
                    <div className="flex h-full min-h-64 flex-col items-center justify-center text-center">
                      <Spinner />
                      <p className="mt-4 text-sm font-medium">{selected.status === "preparing" ? "正在冻结任务证据" : "分析 Agent 正在读取证据"}</p>
                      <p className="mt-1 max-w-sm text-xs leading-5 text-muted-foreground">报告开始输出后会在这里逐字显示。关闭抽屉不会中断审计。</p>
                    </div>
                  )}
                </div>
              </>
            ) : (
              <div className="flex min-h-72 flex-1 flex-col items-center justify-center p-8 text-center">
                <FileSearch className="size-8 text-muted-foreground/60" />
                <p className="mt-3 text-sm font-medium">创建一次独立任务审计</p>
                <p className="mt-1 max-w-sm text-xs leading-5 text-muted-foreground">分析 Agent 会读取任务、Issue 树、全部会话、工具事件、协作、验收、日志、Phone 与附件证据。</p>
              </div>
            )}
          </main>
        </div>
      </SheetContent>
    </Sheet>
  )
}

function AuditStatus({ audit, compact = false }: { audit: TaskAudit; compact?: boolean }) {
  if (audit.status === "completed") return <Badge variant="outline" className="text-success"><CheckCircle2 />{compact ? "完成" : "审计完成"}</Badge>
  if (audit.status === "failed") return <Badge variant="outline" className="text-destructive"><AlertTriangle />{compact ? "失败" : "审计失败"}</Badge>
  return <Badge variant="outline"><Spinner />{audit.status === "preparing" ? "准备证据" : "分析中"}</Badge>
}
