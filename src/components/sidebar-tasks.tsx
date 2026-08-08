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
import { Spinner } from "@/components/ui/spinner"
import {
  SidebarMenu,
  SidebarMenuAction,
  SidebarMenuButton,
  SidebarMenuItem,
  SidebarMenuSub,
  SidebarMenuSubButton,
  SidebarMenuSubItem,
} from "@/components/ui/sidebar"
import { issueRuntimeMap, issueRuntimeOrUnavailable } from "@/lib/issue-runtime"
import { issuesByTask, latestUpdatedAt } from "@/lib/collections"
import { useAppState } from "@/lib/state"
import { cn } from "@/lib/utils"
import type { Issue, Task } from "@/types"

export function SidebarTaskGroup() {
  const { state } = useAppState()
  const location = useLocation()
  const issues = React.useMemo(() => state?.issues ?? [], [state?.issues])
  const taskIssuesByID = React.useMemo(() => issuesByTask(issues), [issues])
  const tasks = React.useMemo(
    () =>
      [...(state?.tasks ?? [])].sort((left, right) =>
        latestUpdatedAt(
          right.updatedAt,
          taskIssuesByID.get(right.id) ?? []
        ).localeCompare(
          latestUpdatedAt(left.updatedAt, taskIssuesByID.get(left.id) ?? [])
        )
      ),
    [state?.tasks, taskIssuesByID]
  )
  const routeTaskId = matchTaskId(location.pathname)
  const routeIssueId = matchIssueId(location.pathname)
  const runtimeMap = React.useMemo(
    () => issueRuntimeMap(state?.issueRuntimes ?? []),
    [state?.issueRuntimes]
  )
  const runningTaskIds = React.useMemo(() => {
    const ids = new Set<string>()
    const issueById = new Map(issues.map((issue) => [issue.id, issue]))
    const rootByIssueId = new Map<string, Issue>()

    const findRoot = (issue: Issue) => {
      const cached = rootByIssueId.get(issue.id)
      if (cached) return cached

      const path: Issue[] = []
      const seen = new Set<string>()
      let current = issue
      while (current.parentId && !seen.has(current.id)) {
        seen.add(current.id)
        path.push(current)
        const parent = issueById.get(current.parentId)
        if (!parent) break
        current = parent
      }
      rootByIssueId.set(current.id, current)
      for (const candidate of path) rootByIssueId.set(candidate.id, current)
      return current
    }

    for (const issue of issues) {
      if (issueRuntimeOrUnavailable(runtimeMap, issue.id).kind !== "running") {
        continue
      }
      const root = findRoot(issue)
      const taskId = root.taskSourceId
      if (taskId) ids.add(taskId)
    }
    return ids
  }, [issues, runtimeMap])
  const activeTaskId = React.useMemo(() => {
    if (routeTaskId) return routeTaskId
    if (!routeIssueId) return null
    const issueById = new Map(issues.map((issue) => [issue.id, issue]))
    let current = issueById.get(routeIssueId)
    const seen = new Set<string>()
    while (current && current.parentId && !seen.has(current.id)) {
      seen.add(current.id)
      current = issueById.get(current.parentId)
    }
    return current?.taskSourceId ?? null
  }, [issues, routeIssueId, routeTaskId])

  return (
    <div className="mt-2 pt-2 group-data-[collapsible=icon]:hidden">
      <div className="flex items-center justify-between px-2 pb-1">
        <p className="px-1 text-xs font-medium text-sidebar-foreground/70">
          任务
        </p>
        <Link
          to="/tasks/new"
          state={{ backgroundLocation: location }}
          aria-label="新建任务"
          title="新建任务"
          className="flex size-6 items-center justify-center rounded-full text-sidebar-foreground/70 outline-none hover:bg-sidebar-accent hover:text-sidebar-accent-foreground focus-visible:ring-2 focus-visible:ring-ring"
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
          tasks={tasks}
          taskIssuesByID={taskIssuesByID}
          runningTaskIds={runningTaskIds}
          activeTaskId={activeTaskId}
          locationPathname={location.pathname}
          navigationKey={location.key}
          boardView={
            new URLSearchParams(location.search).get("view") === "board"
          }
        />
      </SidebarMenu>
    </div>
  )
}

function TaskList({
  tasks,
  taskIssuesByID,
  runningTaskIds,
  activeTaskId,
  locationPathname,
  navigationKey,
  boardView,
}: {
  tasks: Task[]
  taskIssuesByID: Map<string, Issue[]>
  runningTaskIds: Set<string>
  activeTaskId: string | null
  locationPathname: string
  navigationKey: string
  boardView: boolean
}) {
  const [expandedTaskId, setExpandedTaskId] = React.useState<string | null>(
    () => activeTaskId
  )
  const taskItemRefs = React.useRef(new Map<string, HTMLLIElement>())

  React.useEffect(() => {
    if (!activeTaskId) return
    // eslint-disable-next-line react-hooks/set-state-in-effect
    setExpandedTaskId(activeTaskId)
  }, [activeTaskId, navigationKey])

  // Route changes may come from the top navigation, a table, or an Issue link.
  // Wait until the matching task is expanded, then center the task navigation
  // group so its active Home / Issues / Board entry is immediately visible.
  React.useEffect(() => {
    if (!activeTaskId || expandedTaskId !== activeTaskId) return
    const frame = window.requestAnimationFrame(() => {
      const item = taskItemRefs.current.get(activeTaskId)
      const container = item?.closest<HTMLElement>(
        '[data-slot="sidebar-content"]'
      )
      if (!container || !item) return

      const containerRect = container.getBoundingClientRect()
      const itemRect = item.getBoundingClientRect()
      const top =
        container.scrollTop +
        itemRect.top -
        containerRect.top -
        (containerRect.height - itemRect.height) / 2
      const reducedMotion = window.matchMedia(
        "(prefers-reduced-motion: reduce)"
      ).matches
      container.scrollTo({
        top,
        behavior: reducedMotion ? "auto" : "smooth",
      })
    })
    return () => window.cancelAnimationFrame(frame)
  }, [activeTaskId, expandedTaskId, navigationKey, tasks])

  return (
    <>
      {tasks.map((task) => (
        <TaskSidebarItem
          key={task.id}
          task={task}
          taskIssues={taskIssuesByID.get(task.id) ?? []}
          running={runningTaskIds.has(task.id)}
          routeTaskId={activeTaskId}
          locationPathname={locationPathname}
          boardView={boardView}
          expanded={expandedTaskId === task.id}
          onToggle={() =>
            setExpandedTaskId((current) =>
              current === task.id ? null : task.id
            )
          }
          itemRef={(element) => {
            if (element) taskItemRefs.current.set(task.id, element)
            else taskItemRefs.current.delete(task.id)
          }}
        />
      ))}
    </>
  )
}

function TaskSidebarItem({
  task,
  taskIssues,
  running,
  routeTaskId,
  locationPathname,
  boardView,
  expanded,
  onToggle,
  itemRef,
}: {
  task: Task
  taskIssues: Issue[]
  running: boolean
  routeTaskId: string | null
  locationPathname: string
  boardView: boolean
  expanded: boolean
  onToggle: () => void
  itemRef: (element: HTMLLIElement | null) => void
}) {
  const homeTo = `/tasks/${task.id}`
  const issuesTo = `/tasks/${task.id}/issues`
  const boardTo = `/tasks/${task.id}/board`
  const label = task.title
  // Any issue detail page of this task (root or child) counts as its
  // Issues entry; board view counts as its Board entry.
  const routeIssueInTask =
    routeTaskId === task.id && /^\/issues\/[^/]+$/.test(locationPathname)

  return (
    <SidebarMenuItem ref={itemRef}>
      <div className="flex h-7 min-w-0 items-center gap-0.5 rounded-md text-[0.8rem] text-sidebar-foreground transition-colors hover:bg-sidebar-accent hover:text-sidebar-accent-foreground">
        <button
          type="button"
          onClick={onToggle}
          aria-expanded={expanded}
          title={task.title}
          className="flex h-6 min-w-0 flex-1 items-center gap-0.5 rounded-md px-1 py-0.5 text-left outline-none focus-visible:ring-2 focus-visible:ring-ring"
        >
          <ChevronRight
            className={cn(
              "size-3.5 shrink-0 transition-transform",
              expanded && "rotate-90"
            )}
          />
          <span className="flex min-w-0 flex-1 items-center gap-1">
            <span className="min-w-0 flex-1 truncate">{label}</span>
            {running ? (
              <span
                className="flex size-5 shrink-0 items-center justify-center rounded-full bg-sidebar-primary text-sidebar-primary-foreground"
                title="执行中"
              >
                <Spinner
                  className="size-3.5 stroke-[2.5]"
                  aria-label="执行中"
                />
              </span>
            ) : null}
          </span>
        </button>
        <TaskRowMenu
          task={task}
          taskIssues={taskIssues}
          triggerRender={
            <SidebarMenuAction
              showOnHover
              className="isolate before:pointer-events-none before:absolute before:inset-y-0 before:right-0 before:z-0 before:w-10 before:rounded-r-md before:bg-gradient-to-l before:from-sidebar-accent before:via-sidebar-accent before:to-transparent [&>svg]:relative [&>svg]:z-10"
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
              isActive={locationPathname === `/tasks/${task.id}`}
              render={<NavLink to={homeTo} end />}
            >
              <House />
              <span>Home</span>
            </SidebarMenuSubButton>
          </SidebarMenuSubItem>
          <SidebarMenuSubItem>
            <SidebarMenuSubButton
              isActive={
                locationPathname === `/tasks/${task.id}/issues` ||
                (routeIssueInTask && !boardView)
              }
              render={<NavLink to={issuesTo} end />}
            >
              <GitBranch />
              <span>Issues</span>
            </SidebarMenuSubButton>
          </SidebarMenuSubItem>
          <SidebarMenuSubItem>
            <SidebarMenuSubButton
              isActive={
                locationPathname === `/tasks/${task.id}/board` ||
                (routeIssueInTask && boardView)
              }
              render={<NavLink to={boardTo} end />}
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

function matchTaskId(pathname: string) {
  const match = pathname.match(/^\/tasks\/([^/]+)(?:\/(?:issues|board))?$/)
  return match?.[1] ?? null
}

function matchIssueId(pathname: string) {
  const issueMatch = pathname.match(/^\/issues\/([^/]+)$/)
  return issueMatch?.[1] ?? null
}
