import type { AppState, Execution, Issue, IssueRuntimeView } from "@/types"

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

// normalizeIssueRuntimeProjection is a rolling-upgrade adapter. The currently
// running pre-projection backend may omit issueRuntimes until it is restarted.
// New backends are authoritative; only missing entries are synthesized here,
// once, at the state ingestion boundary, using the exact currentExecutionId.
export function normalizeIssueRuntimeProjection(state: AppState): AppState {
  const provided = issueRuntimeMap(state.issueRuntimes ?? [])
  if (state.issues.every((issue) => provided.has(issue.id))) return state

  const executionByID = new Map(
    state.executions.map((execution) => [execution.id, execution])
  )
  const issueRuntimes = state.issues.map(
    (issue) =>
      provided.get(issue.id) ??
      legacyIssueRuntime(
        issue,
        executionByID.get(issue.currentExecutionId ?? "")
      )
  )
  return { ...state, issueRuntimes }
}

function legacyIssueRuntime(
  issue: Issue,
  execution: Execution | undefined
): IssueRuntimeView {
  const base = {
    issueId: issue.id,
    currentExecutionId: execution?.id,
    currentExecutionStatus: execution?.status,
    updatedAt: issue.updatedAt,
  }
  const view = (
    state: string,
    kind: IssueRuntimeView["kind"],
    health: IssueRuntimeView["health"],
    detail?: string
  ): IssueRuntimeView => ({ ...base, state, kind, health, detail })

  if (issue.status === "done" && issue.labels?.includes("budget_exceeded")) {
    return view("budget_exceeded", "failed", "error", issue.error)
  }
  if (issue.status === "done" && issue.labels?.includes("failed")) {
    return view("failed", "failed", "error", issue.error || execution?.error)
  }
  if (issue.status === "in_progress" && issue.labels?.includes("blocked")) {
    return view("blocked", "failed", "error", issue.error || execution?.error)
  }
  if (issue.status === "done") {
    return view("completed", "completed", "healthy")
  }
  if (issue.status === "cancelled") {
    return view(
      issue.objectiveAbandoned ? "abandoned" : "cancelled",
      "cancelled",
      "healthy",
      issue.abandonmentReason
    )
  }
  if (issue.executionPhase === "recovering") {
    if (
      !issue.recoveryExecutionId ||
      issue.recoveryExecutionId !== issue.currentExecutionId
    ) {
      return view(
        "data_inconsistent",
        "failed",
        "error",
        "恢复 Execution 不存在，或不是 Issue 当前执行"
      )
    }
    if (!execution) {
      return view(
        "recovery_pending",
        "waiting",
        "waiting",
        "旧后端未返回恢复 Execution；等待恢复调度器接管"
      )
    }
    if (
      ["queued", "starting", "running", "waiting_approval"].includes(
        execution.status
      )
    ) {
      return view("recovering", "running", "healthy", execution.currentTool)
    }
    if (["failed", "cancelled"].includes(execution.status)) {
      return view("recovery_stalled", "failed", "stalled", execution.error)
    }
    return view("recovery_pending", "waiting", "waiting", "等待恢复调度器接管")
  }
  if (issue.executionPhase === "waiting_children") {
    return view(
      "waiting_children",
      "waiting",
      "waiting",
      "等待直属子 Issue 完成"
    )
  }
  if (issue.executionPhase === "sleeping") {
    return view("sleeping", "waiting", "waiting", "等待唤醒")
  }
  if (issue.executionPhase === "scheduled") {
    return view("scheduled", "pending", "waiting", "等待调度器创建 Execution")
  }
  if (execution) {
    if (execution.status === "queued")
      return view("queued", "running", "healthy")
    if (execution.status === "starting")
      return view("starting", "running", "healthy")
    if (execution.status === "waiting_approval") {
      return view("waiting_approval", "waiting", "waiting")
    }
    if (execution.status === "running") {
      const stateByPhase: Record<string, string> = {
        validating: "validating",
        resuming: "resuming",
        summarizing: "summarizing",
        budget_summarizing: "budget_summarizing",
      }
      return view(
        stateByPhase[issue.executionPhase] ?? "running",
        "running",
        "healthy",
        execution.currentTool ? `正在调用 ${execution.currentTool}` : undefined
      )
    }
    if (["failed", "disconnected", "stopped"].includes(execution.status)) {
      return view(
        `execution_${execution.status}`,
        "failed",
        "stalled",
        issue.error || execution.error
      )
    }
  }
  if (issue.status === "in_progress") {
    if (issue.currentExecutionId) {
      return view(
        "runtime_unavailable",
        "waiting",
        "waiting",
        "旧后端的精简状态未包含当前 Execution"
      )
    }
    return view(
      "execution_missing",
      "failed",
      "stalled",
      "Issue 正在处理中，但没有活动的当前 Execution"
    )
  }
  if (issue.status === "in_review") {
    return view("in_review", "waiting", "waiting", "等待人工复核")
  }
  return view("pending", "pending", "healthy")
}
