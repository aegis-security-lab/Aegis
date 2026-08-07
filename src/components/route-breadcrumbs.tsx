import * as React from "react"
import { ChevronRight } from "lucide-react"
import { Link, useLocation } from "react-router-dom"

import { useAppState } from "@/lib/state"
import type { Issue, Task } from "@/types"

type Crumb = {
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

export function RouteBreadcrumbs() {
  const { state } = useAppState()
  const location = useLocation()
  const fromBoard = new URLSearchParams(location.search).get("view") === "board"
  const crumbs = buildCrumbs(
    location.pathname,
    state?.issues ?? [],
    state?.tasks ?? [],
    fromBoard
  )
  if (crumbs.length === 0) return null

  return (
    <nav
      aria-label="面包屑"
      className="flex h-9 shrink-0 items-center gap-0.5 overflow-x-auto border-b border-border/70 px-3"
    >
      {crumbs.map((crumb, index) => (
        <React.Fragment key={`${crumb.label}-${index}`}>
          {index > 0 ? (
            <ChevronRight className="size-3.5 shrink-0 text-muted-foreground/60" />
          ) : null}
          {crumb.to && !crumb.current ? (
            <Link
              to={crumb.to}
              className="max-w-56 truncate rounded-md px-1.5 py-0.5 text-[0.8rem] text-muted-foreground transition-colors hover:bg-muted hover:text-foreground"
            >
              {crumb.label}
            </Link>
          ) : (
            <span
              aria-current="page"
              className="max-w-56 truncate rounded-md px-1.5 py-0.5 text-[0.8rem] font-medium text-foreground"
            >
              {crumb.label}
            </span>
          )}
        </React.Fragment>
      ))}
    </nav>
  )
}

function buildCrumbs(
  pathname: string,
  issues: Issue[],
  tasks: Task[],
  fromBoard: boolean
): Crumb[] {
  if (pathname === "/") return [{ label: "概览", current: true }]
  if (pathname === "/tasks/new") {
    return [{ label: "任务", to: "/tasks" }, { label: "新建任务", current: true }]
  }

  const parts = pathname.split("/").filter(Boolean)
  if (parts[0] === "tasks" && parts.length === 2) {
    const root = rootIssue(parts[1], issues)
    return [
      {
        label: taskTitle(root, tasks) ?? "任务",
        to: `/tasks/${parts[1]}`,
      },
      { label: "Home", current: true },
    ]
  }
  if (parts[0] === "tasks" && parts.length === 3) {
    const root = rootIssue(parts[1], issues)
    const taskCrumb = {
      label: taskTitle(root, tasks) ?? "任务",
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
    return [
      {
        label: taskTitle(root, tasks) ?? "任务",
        to: root ? `/tasks/${root.id}` : "/tasks",
      },
      fromBoard
        ? {
            label: "Board",
            to: root ? `/tasks/${root.id}/board` : "/tasks",
          }
        : {
            label: "Issues",
            to: root ? `/tasks/${root.id}/issues` : "/issues",
          },
      { label: issue?.title ?? "Issue", current: true },
    ]
  }
  if (parts[0] === "sessions" && parts.length === 2) {
    return [{ label: "执行会话", to: "/sessions" }, { label: "Session", current: true }]
  }
  if (parts[0] === "knowledge-bases" && parts.length === 2) {
    return [{ label: "知识库", to: "/knowledge-bases" }, { label: "详情", current: true }]
  }

  const simple = simpleCrumbs.find((item) => pathname.startsWith(item.prefix))
  return simple ? [{ label: simple.label, current: true }] : []
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

function taskTitle(root: Issue | undefined, tasks: Task[]) {
  if (!root) return null
  const task = tasks.find((candidate) => candidate.id === root.taskSourceId)
  return task?.title ?? root.identifier ?? root.title
}
