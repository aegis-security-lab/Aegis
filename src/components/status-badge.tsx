import { Badge } from "@/components/ui/badge"
import { cn } from "@/lib/utils"

const labels: Record<string, string> = {
  planning: "规划中",
  running: "运行中",
  paused: "已暂停",
  awaiting_review: "待复核",
  completed: "已完成",
  failed: "失败",
  stopped: "已停止",
  interrupted: "待恢复",
  backlog: "待处理",
  todo: "待执行",
  in_progress: "处理中",
  in_review: "待复核",
  blocked: "阻塞",
  review: "待复核",
  done: "已完成",
  cancelled: "已取消",
  queued: "排队中",
  starting: "启动中",
  idle: "空闲",
  waiting_approval: "待审批",
  disconnected: "已断开",
  pending: "待审批",
  approved: "已批准",
  rejected: "已拒绝",
}

const styles: Record<string, string> = {
  planning:
    "border-sky-200 bg-sky-50 text-sky-700 dark:border-sky-900 dark:bg-sky-950 dark:text-sky-300",
  running:
    "border-emerald-200 bg-emerald-50 text-emerald-700 dark:border-emerald-900 dark:bg-emerald-950 dark:text-emerald-300",
  in_progress:
    "border-emerald-200 bg-emerald-50 text-emerald-700 dark:border-emerald-900 dark:bg-emerald-950 dark:text-emerald-300",
  todo: "border-sky-200 bg-sky-50 text-sky-700 dark:border-sky-900 dark:bg-sky-950 dark:text-sky-300",
  completed: "border-border bg-muted text-foreground",
  done: "border-border bg-muted text-foreground",
  paused:
    "border-amber-200 bg-amber-50 text-amber-700 dark:border-amber-900 dark:bg-amber-950 dark:text-amber-300",
  awaiting_review:
    "border-violet-200 bg-violet-50 text-violet-700 dark:border-violet-900 dark:bg-violet-950 dark:text-violet-300",
  review:
    "border-violet-200 bg-violet-50 text-violet-700 dark:border-violet-900 dark:bg-violet-950 dark:text-violet-300",
  in_review:
    "border-violet-200 bg-violet-50 text-violet-700 dark:border-violet-900 dark:bg-violet-950 dark:text-violet-300",
  pending:
    "border-amber-200 bg-amber-50 text-amber-700 dark:border-amber-900 dark:bg-amber-950 dark:text-amber-300",
  waiting_approval:
    "border-amber-200 bg-amber-50 text-amber-700 dark:border-amber-900 dark:bg-amber-950 dark:text-amber-300",
  failed: "border-destructive/25 bg-destructive/10 text-destructive",
  blocked: "border-destructive/25 bg-destructive/10 text-destructive",
  disconnected: "border-destructive/25 bg-destructive/10 text-destructive",
  stopped: "border-border bg-muted text-muted-foreground",
  interrupted:
    "border-orange-200 bg-orange-50 text-orange-700 dark:border-orange-900 dark:bg-orange-950 dark:text-orange-300",
}

export function StatusBadge({
  status,
  className,
}: {
  status: string
  className?: string
}) {
  return (
    <Badge
      variant="outline"
      className={cn("font-medium", styles[status], className)}
    >
      {labels[status] ?? status}
    </Badge>
  )
}
