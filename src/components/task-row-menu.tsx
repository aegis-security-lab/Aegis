import * as React from "react"
import {
  CircleStop,
  Clock3,
  Copy,
  FileSearch,
  MoreHorizontal,
} from "lucide-react"
import { useNavigate } from "react-router-dom"

import { CancelTaskDialog } from "@/components/cancel-task-dialog"
import { TaskAuditDrawer } from "@/components/task-audit-drawer"
import { TaskBudgetDialog } from "@/components/task-budget-dialog"
import { Button } from "@/components/ui/button"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuGroup,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import { useAppState } from "@/lib/state"
import type { Issue, Task } from "@/types"

interface TaskRowMenuProps {
  task: Task
  taskIssues: Issue[]
  align?: "start" | "end"
  triggerRender?: React.ReactElement
}

export function TaskRowMenu({
  task,
  taskIssues,
  align = "end",
  triggerRender,
}: TaskRowMenuProps) {
  const { state, refresh } = useAppState()
  const navigate = useNavigate()
  const [auditOpen, setAuditOpen] = React.useState(false)
  const [cancelTarget, setCancelTarget] = React.useState<{
    taskId: string
    title: string
    totalIssues: number
    activeExecutions: number
  } | null>(null)
  const [budgetOpen, setBudgetOpen] = React.useState(false)
  const [now, setNow] = React.useState(() => Date.now())

  React.useEffect(() => {
    const timer = window.setInterval(() => setNow(Date.now()), 30000)
    return () => window.clearInterval(timer)
  }, [])

  const executions = state?.executions ?? []
  const issueIDs = new Set(taskIssues.map((issue) => issue.id))
  const activeExecutions = executions.filter(
    (execution) =>
      issueIDs.has(execution.issueId) &&
      ["queued", "starting", "running", "waiting_approval"].includes(
        execution.status
      )
  ).length

  const budgetMinutes = task.timeBudgetMinutes ?? null
  const startedAt = Date.parse(task.createdAt)
  const elapsedMinutes = Number.isFinite(startedAt)
    ? Math.max(0, Math.floor((now - startedAt) / 60000))
    : 0
  const budgetLabel = budgetMinutes
    ? `${elapsedMinutes}/${budgetMinutes}`
    : `${elapsedMinutes}/∞`

  const clone = () => {
    navigate("/tasks/new", {
      state: {
        clone: {
          projectId: task.projectId,
          title: task.title,
          description: task.description,
          objective: task.objective,
          priority: task.priority,
          workMode: task.workMode,
          assigneeAgentId: task.assigneeAgentId,
          containerProfileId:
            state?.containerProfiles.some(
              (profile) =>
                profile.id === task.containerProfileId && profile.enabled
            ) && task.containerProfileId
              ? task.containerProfileId
              : undefined,
          constraints: task.constraints,
          timeBudgetMinutes: task.timeBudgetMinutes,
          humanValidationFallback: task.humanValidationFallback,
        },
      },
    })
  }

  return (
    <>
      <DropdownMenu>
        <DropdownMenuTrigger
          render={
            triggerRender ?? (
              <Button
                variant="ghost"
                size="icon-sm"
                aria-label={`任务操作：${task.title}`}
              />
            )
          }
        >
          <MoreHorizontal />
        </DropdownMenuTrigger>
        <DropdownMenuContent align={align} className="w-60">
          <DropdownMenuGroup>
            <DropdownMenuItem onClick={() => setAuditOpen(true)}>
              <FileSearch data-icon="inline-start" />
              分析任务
            </DropdownMenuItem>
            <DropdownMenuItem onClick={clone}>
              <Copy data-icon="inline-start" />
              复制为新任务
            </DropdownMenuItem>
            <DropdownMenuItem onClick={() => setBudgetOpen(true)}>
              <Clock3 data-icon="inline-start" />
              时间预算
              <span className="ml-auto text-xs text-muted-foreground tabular-nums">
                {budgetLabel}
              </span>
            </DropdownMenuItem>
            {taskIssues.some(
              (issue) => !["done", "cancelled"].includes(issue.status)
            ) ? (
              <DropdownMenuItem
                variant="destructive"
                onClick={() =>
                  setCancelTarget({
                    taskId: task.id,
                    title: task.title,
                    totalIssues: issueIDs.size,
                    activeExecutions,
                  })
                }
              >
                <CircleStop data-icon="inline-start" />
                取消任务
              </DropdownMenuItem>
            ) : null}
          </DropdownMenuGroup>
        </DropdownMenuContent>
      </DropdownMenu>

      {auditOpen ? (
        <TaskAuditDrawer
          open
          onOpenChange={setAuditOpen}
          taskId={task.id}
          taskTitle={task.title}
        />
      ) : null}
      {cancelTarget ? (
        <CancelTaskDialog
          open
          onOpenChange={(open) => {
            if (!open) setCancelTarget(null)
          }}
          taskId={cancelTarget.taskId}
          taskTitle={cancelTarget.title}
          totalIssues={cancelTarget.totalIssues}
          activeExecutions={cancelTarget.activeExecutions}
          onCancelled={() => refresh()}
        />
      ) : null}
      <TaskBudgetDialog
        key={task.id}
        open={budgetOpen}
        onOpenChange={setBudgetOpen}
        taskId={task.id}
        taskTitle={task.title}
        currentMinutes={budgetMinutes}
        onSaved={() => refresh()}
      />
    </>
  )
}
