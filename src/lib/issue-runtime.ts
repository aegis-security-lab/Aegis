import type { Issue, IssueRuntimeKind, IssueRuntimeView } from "@/types"

const labels: Record<string, string> = {
  completed: "任务结束",
  abandoned: "目标已放弃",
  cancelled: "任务已取消",
  budget_exceeded: "执行预算已耗尽",
  failed: "执行失败",
  blocked: "任务阻塞",
  recovering: "重启恢复中",
  recovery_pending: "等待恢复调度",
  recovery_stalled: "恢复已停滞",
  data_inconsistent: "状态数据异常",
  waiting_children: "等待子任务",
  wait_stalled: "子任务等待已停滞",
  sleeping: "休眠等待唤醒",
  sleep_stalled: "休眠状态异常",
  scheduled: "等待调度",
  queued: "执行排队中",
  starting: "Agent 启动中",
  running: "Agent 执行中",
  validating: "验收中",
  validation_stalled: "验收已停滞",
  resuming: "汇总结果",
  summarizing: "取消后总结中",
  budget_summarizing: "预算耗尽后总结中",
  waiting_approval: "等待审批",
  execution_failed: "执行失败",
  execution_disconnected: "执行断开",
  execution_stopped: "执行已停止",
  execution_missing: "执行已停滞",
  in_review: "等待人工复核",
  pending: "等待调度",
  unavailable: "运行状态不可用",
  runtime_unavailable: "等待运行状态同步",
}

export function issueRuntimeLabel(runtime: IssueRuntimeView) {
  return labels[runtime.state] ?? runtime.state
}

export function issueRuntimeMap(runtimes: IssueRuntimeView[]) {
  return new Map(runtimes.map((runtime) => [runtime.issueId, runtime]))
}

export function issueRuntimeOrUnavailable(
  runtimes: Map<string, IssueRuntimeView>,
  issueId: string
): IssueRuntimeView {
  return (
    runtimes.get(issueId) ?? {
      issueId,
      state: "unavailable",
      kind: "failed",
      health: "error",
      detail: "服务端没有返回该 Issue 的统一运行状态",
      updatedAt: "",
    }
  )
}

const taskRuntimePriority: Record<IssueRuntimeKind, number> = {
  running: 0,
  waiting: 1,
  failed: 2,
  pending: 3,
  completed: 4,
  cancelled: 5,
}

export function taskRuntimeView(
  taskId: string,
  issues: Issue[],
  runtimes: Map<string, IssueRuntimeView>
): IssueRuntimeView | undefined {
  if (issues.length === 0) return undefined
  const selected = issues
    .map((issue) => issueRuntimeOrUnavailable(runtimes, issue.id))
    .sort(
      (left, right) =>
        taskRuntimePriority[left.kind] - taskRuntimePriority[right.kind] ||
        right.updatedAt.localeCompare(left.updatedAt)
    )[0]
  return { ...selected, issueId: taskId }
}
