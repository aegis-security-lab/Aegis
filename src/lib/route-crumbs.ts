import type { Issue, Task } from "@/types"

export type Crumb = {
  label: string
  to?: string
  current?: boolean
}

const simpleCrumbs: Array<{ prefix: string; label: string }> = [
  { prefix: "/workspace", label: "管家" },
  { prefix: "/tasks", label: "任务" },
  { prefix: "/issues", label: "Issues" },
  { prefix: "/sessions", label: "执行会话" },
  { prefix: "/approvals", label: "审批" },
  { prefix: "/containers", label: "环境与容器" },
  { prefix: "/security/findings", label: "安全发现" },
  { prefix: "/security/uncover", label: "网络空间检索" },
  { prefix: "/security", label: "安全态势" },
  { prefix: "/agents", label: "Agent 类型" },
  { prefix: "/capabilities", label: "能力控制台" },
  { prefix: "/skills", label: "Skills" },
  { prefix: "/knowledge-bases", label: "知识库" },
  { prefix: "/timeline", label: "时间线" },
  { prefix: "/settings", label: "设置" },
]

export function buildCrumbs(
  pathname: string,
  issues: Issue[],
  tasks: Task[],
  fromBoard = false
): Crumb[] {
  if (pathname === "/") return [{ label: "概览", current: true }]
  if (pathname === "/tasks/new") {
    return [
      { label: "任务", to: "/tasks" },
      { label: "新建任务", current: true },
    ]
  }

  const parts = pathname.split("/").filter(Boolean)
  if (parts[0] === "tasks" && parts.length === 2) {
    const task = tasks.find((candidate) => candidate.id === parts[1])
    return [
      {
        label: task?.title ?? "任务",
        to: `/tasks/${parts[1]}`,
      },
      { label: "Home", current: true },
    ]
  }
  if (parts[0] === "tasks" && parts.length === 3) {
    const task = tasks.find((candidate) => candidate.id === parts[1])
    const taskCrumb = {
      label: task?.title ?? "任务",
      to: `/tasks/${parts[1]}`,
    }
    if (parts[2] === "issues") {
      return [taskCrumb, { label: "Issues", current: true }]
    }
    if (parts[2] === "board") {
      return [taskCrumb, { label: "Board", current: true }]
    }
  }
  if (parts[0] === "issues" && parts.length === 2) {
    const issue = issues.find((candidate) => candidate.id === parts[1])
    const root = rootIssue(parts[1], issues)
    const taskID = root?.taskSourceId
    return [
      {
        label: taskTitle(taskID, tasks) ?? "任务",
        to: taskID ? `/tasks/${taskID}` : "/tasks",
      },
      fromBoard
        ? {
            label: "Board",
            to: taskID ? `/tasks/${taskID}/board` : "/tasks",
          }
        : {
            label: "Issues",
            to: taskID ? `/tasks/${taskID}/issues` : "/issues",
          },
      { label: issue?.title ?? "Issue", current: true },
    ]
  }
  if (parts[0] === "sessions" && parts.length === 2) {
    return [
      { label: "执行会话", to: "/sessions" },
      { label: "Session", current: true },
    ]
  }
  if (parts[0] === "knowledge-bases" && parts.length === 2) {
    return [
      { label: "知识库", to: "/knowledge-bases" },
      { label: "详情", current: true },
    ]
  }

  const simple = simpleCrumbs.find((item) => pathname.startsWith(item.prefix))
  return simple ? [{ label: simple.label, current: true }] : []
}

export function lastCrumbLabel(
  pathname: string,
  issues: Issue[],
  tasks: Task[]
): string | null {
  const crumbs = buildCrumbs(pathname, issues, tasks)
  return crumbs.length > 0 ? crumbs[crumbs.length - 1].label : null
}

function rootIssue(issueId: string, issues: Issue[]) {
  const byID = new Map(issues.map((issue) => [issue.id, issue]))
  let current = byID.get(issueId)
  const seen = new Set<string>()
  while (current?.parentId && !seen.has(current.id)) {
    seen.add(current.id)
    current = byID.get(current.parentId)
  }
  return current
}

function taskTitle(taskID: string | undefined, tasks: Task[]) {
  if (!taskID) return null
  return tasks.find((candidate) => candidate.id === taskID)?.title ?? null
}
