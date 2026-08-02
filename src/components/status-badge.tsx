import { Badge } from "@/components/ui/badge"
import { cn } from "@/lib/utils"

const labels: Record<string, string> = {
  planning: "规划中",
  running: "运行中",
  paused: "已暂停",
  awaiting_review: "待复核",
  completed: "已完成",
  failed: "失败",
  budget_exceeded: "超出预算",
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
  planning: "border-info/25 bg-info/10 text-info",
  running: "border-success/25 bg-success/10 text-success",
  in_progress: "border-success/25 bg-success/10 text-success",
  todo: "border-info/25 bg-info/10 text-info",
  completed: "border-border bg-muted text-foreground",
  done: "border-border bg-muted text-foreground",
  paused: "border-warning/25 bg-warning/10 text-warning",
  awaiting_review: "border-info/25 bg-info/10 text-info",
  review: "border-info/25 bg-info/10 text-info",
  in_review: "border-info/25 bg-info/10 text-info",
  pending: "border-warning/25 bg-warning/10 text-warning",
  waiting_approval: "border-warning/25 bg-warning/10 text-warning",
  failed: "border-destructive/25 bg-destructive/10 text-destructive",
  budget_exceeded: "border-warning/25 bg-warning/10 text-warning",
  blocked: "border-destructive/25 bg-destructive/10 text-destructive",
  disconnected: "border-destructive/25 bg-destructive/10 text-destructive",
  stopped: "border-border bg-muted text-muted-foreground",
  interrupted: "border-warning/25 bg-warning/10 text-warning",
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
