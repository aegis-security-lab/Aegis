import * as React from "react"
import {
  ChevronRight,
  GitBranch,
  House,
  LayoutGrid,
  ListTodo,
  Plus,
} from "lucide-react"
import { Link, NavLink, useLocation } from "react-router-dom"

import { TaskRowMenu } from "@/components/task-row-menu"
import {
  SidebarMenu,
  SidebarMenuAction,
  SidebarMenuButton,
  SidebarMenuItem,
  SidebarMenuSub,
  SidebarMenuSubButton,
  SidebarMenuSubItem,
} from "@/components/ui/sidebar"
import { useAppState } from "@/lib/state"
import { cn } from "@/lib/utils"
import type { Issue, Task } from "@/types"

export function SidebarTaskGroup() {
  const { state } = useAppState()
  const location = useLocation()
  const tasks = [...(state?.tasks ?? [])].sort((left, right) =>
    right.updatedAt.localeCompare(left.updatedAt)
  )
  const issues = React.useMemo(() => state?.issues ?? [], [state?.issues])
  const latestRunByTask = new Map<string, Issue>()
  for (const issue of issues) {
    if (issue.parentId || !issue.taskSourceId) continue
    const current = latestRunByTask.get(issue.taskSourceId)
    if (!current || issue.createdAt > current.createdAt) {
      latestRunByTask.set(issue.taskSourceId, issue)
    }
  }
  const routeIssueId = matchIssueId(location.pathname)
  const activeTaskId = React.useMemo(() => {
    if (!routeIssueId) return null
    const issueById = new Map(issues.map((issue) => [issue.id, issue]))
    let current = issueById.get(routeIssueId)
    const seen = new Set<string>()
    while (current && current.parentId && !seen.has(current.id)) {
      seen.add(current.id)
      current = issueById.get(current.parentId)
    }
    return current?.taskSourceId ?? null
  }, [issues, routeIssueId])

  return (
    <div className="mt-2 pt-2 group-data-[collapsible=icon]:hidden">
      <div className="flex items-center justify-between px-2 pb-1">
        <p className="px-1 text-xs font-medium text-sidebar-foreground/70">
          任务
        </p>
        <Link
          to="/tasks/new"
          aria-label="新建任务"
          title="新建任务"
          className="flex size-6 items-center justify-center rounded-full text-sidebar-foreground/70 hover:bg-sidebar-accent hover:text-sidebar-accent-foreground focus-visible:ring-2 focus-visible:ring-ring outline-none"
        >
          <Plus className="size-4" />
        </Link>
      </div>
      <SidebarMenu>
        <SidebarMenuItem>
          <SidebarMenuButton
            tooltip="全部任务"
            isActive={location.pathname === "/tasks"}
            render={<NavLink to="/tasks" />}
          >
            <ListTodo />
            <span>全部任务</span>
          </SidebarMenuButton>
        </SidebarMenuItem>
        <TaskList
          key={
            activeTaskId
              ? `${activeTaskId}:${location.pathname}`
              : "no-task"
          }
          tasks={tasks}
          latestRunByTask={latestRunByTask}
          activeTaskId={activeTaskId}
          locationPathname={location.pathname}
        />
      </SidebarMenu>
    </div>
  )
}

function TaskList({
  tasks,
  latestRunByTask,
  activeTaskId,
  locationPathname,
}: {
  tasks: Task[]
  latestRunByTask: Map<string, Issue>
  activeTaskId: string | null
  locationPathname: string
}) {
  const [expandedTaskId, setExpandedTaskId] = React.useState<string | null>(
    () => activeTaskId
  )
  const taskRowRefs = React.useRef(new Map<string, HTMLDivElement>())

  // Keep the active task visible: scroll it to the middle of the sidebar.
  // Only fires on navigation (task/path change), never on manual expand.
  React.useEffect(() => {
    if (!activeTaskId) return
    const frame = window.requestAnimationFrame(() => {
      const container = document.querySelector(
        '[data-slot="sidebar-content"]'
      )
      const row = taskRowRefs.current.get(activeTaskId)
      if (!container || !row) return
      const containerRect = container.getBoundingClientRect()
      const rowRect = row.getBoundingClientRect()
      const target =
        container.scrollTop +
        (rowRect.top - containerRect.top) -
        (containerRect.height - rowRect.height) / 2
      container.scrollTo({ top: target, behavior: "smooth" })
    })
    return () => window.cancelAnimationFrame(frame)
  }, [activeTaskId, locationPathname])

  return (
    <>
      {tasks.map((task) => (
        <TaskSidebarItem
          key={task.id}
          task={task}
          latest={latestRunByTask.get(task.id)}
          expanded={expandedTaskId === task.id}
          onToggle={() =>
            setExpandedTaskId((current) =>
              current === task.id ? null : task.id
            )
          }
          rowRef={(element) => {
            if (element) taskRowRefs.current.set(task.id, element)
            else taskRowRefs.current.delete(task.id)
          }}
        />
      ))}
    </>
  )
}

function TaskSidebarItem({
  task,
  latest,
  expanded,
  onToggle,
  rowRef,
}: {
  task: Task
  latest?: Issue
  expanded: boolean
  onToggle: () => void
  rowRef: (element: HTMLDivElement | null) => void
}) {
  const location = useLocation()
  const homeTo = latest ? `/tasks/${latest.id}` : "/tasks"
  const issuesTo = latest ? `/tasks/${latest.id}/issues` : "/issues"
  const boardTo = latest ? `/tasks/${latest.id}/board` : undefined
  const label = task.title

  return (
    <SidebarMenuItem>
      <div
        ref={rowRef}
        className="flex h-7 min-w-0 items-center gap-0.5 rounded-md pr-8 text-[0.8rem] text-sidebar-foreground transition-colors hover:bg-sidebar-accent hover:text-sidebar-accent-foreground"
      >
        <button
          type="button"
          onClick={onToggle}
          aria-expanded={expanded}
          title={task.title}
          className="flex h-6 min-w-0 flex-1 items-center gap-0.5 rounded-md px-1 py-0.5 text-left focus-visible:ring-2 focus-visible:ring-ring outline-none"
        >
          <ChevronRight
            className={cn(
              "size-3.5 shrink-0 transition-transform",
              expanded && "rotate-90"
            )}
          />
          <span className="min-w-0 flex-1 truncate">{label}</span>
        </button>
        <TaskRowMenu
          task={task}
          triggerRender={
            <SidebarMenuAction
              showOnHover
              aria-label={`任务操作：${task.title}`}
              title="任务操作"
            />
          }
        />
      </div>
      {expanded ? (
        <SidebarMenuSub>
          <SidebarMenuSubItem>
            <SidebarMenuSubButton
              isActive={latest ? location.pathname === `/tasks/${latest.id}` : false}
              render={<NavLink to={homeTo} end />}
            >
              <House />
              <span>Home</span>
            </SidebarMenuSubButton>
          </SidebarMenuSubItem>
          <SidebarMenuSubItem>
            <SidebarMenuSubButton
              isActive={
                latest
                  ? location.pathname === `/tasks/${latest.id}/issues` ||
                    (location.pathname === `/issues/${latest.id}` &&
                      new URLSearchParams(location.search).get("view") !==
                        "board")
                  : false
              }
              render={<NavLink to={issuesTo ?? "/tasks"} end />}
            >
              <GitBranch />
              <span>Issues</span>
            </SidebarMenuSubButton>
          </SidebarMenuSubItem>
          <SidebarMenuSubItem>
            <SidebarMenuSubButton
              isActive={
                latest
                  ? location.pathname === `/tasks/${latest.id}/board` ||
                    (location.pathname === `/issues/${latest.id}` &&
                      new URLSearchParams(location.search).get("view") ===
                        "board")
                  : false
              }
              render={<NavLink to={boardTo ?? "/tasks"} end />}
            >
              <LayoutGrid />
              <span>Board</span>
            </SidebarMenuSubButton>
          </SidebarMenuSubItem>
        </SidebarMenuSub>
      ) : null}
    </SidebarMenuItem>
  )
}

function matchIssueId(pathname: string) {
  const boardMatch = pathname.match(/^\/tasks\/([^/]+)\/board$/)
  if (boardMatch) return boardMatch[1]
  const issuesMatch = pathname.match(/^\/tasks\/([^/]+)\/issues$/)
  if (issuesMatch) return issuesMatch[1]
  const taskMatch = pathname.match(/^\/tasks\/([^/]+)$/)
  if (taskMatch) return taskMatch[1]
  const issueMatch = pathname.match(/^\/issues\/([^/]+)$/)
  if (issueMatch) return issueMatch[1]
  return null
}
