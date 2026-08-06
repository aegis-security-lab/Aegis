import * as React from "react"
import { Activity } from "lucide-react"

import { TimelinePanel } from "@/components/timeline-panel"
import { PageHeader } from "@/components/page-header"
import {
  Card,
  CardContent,
} from "@/components/ui/card"
import {
  Empty,
  EmptyDescription,
  EmptyHeader,
  EmptyMedia,
  EmptyTitle,
} from "@/components/ui/empty"
import { Separator } from "@/components/ui/separator"
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import { useAppState } from "@/lib/state"

export function TimelinePage() {
  const { state } = useAppState()
  const tasks = React.useMemo(
    () =>
      [...(state?.issues ?? [])]
        .filter((issue) => !issue.parentId)
        .sort((left, right) => right.updatedAt.localeCompare(left.updatedAt)),
    [state?.issues]
  )
  const [taskID, setTaskID] = React.useState("")
  const resolvedTaskID = tasks.some((task) => task.id === taskID)
    ? taskID
    : (tasks[0]?.id ?? "")
  const taskItems = tasks.map((task) => ({
    label: task.identifier + " · " + task.title,
    value: task.id,
  }))

  return (
    <div className="flex flex-col gap-5">
      <PageHeader
        eyebrow="Work"
        title="时间线"
        description="跨任务查看 Issue、执行、验收与审批的关键事件。"
      />
      <div className="flex min-w-0 flex-wrap items-center gap-3">
        <div className="w-72 shrink-0">
          <Select
            items={taskItems}
            value={resolvedTaskID || null}
            onValueChange={(value) => setTaskID(value ?? "")}
            disabled={tasks.length === 0}
          >
            <SelectTrigger id="timeline-task" className="w-full min-w-0">
              <SelectValue placeholder="选择一个顶层任务" />
            </SelectTrigger>
            <SelectContent alignItemWithTrigger={false}>
              <SelectGroup>
                {taskItems.map((item) => (
                  <SelectItem key={item.value} value={item.value}>
                    <span className="max-w-[420px] truncate">
                      {item.label}
                    </span>
                  </SelectItem>
                ))}
              </SelectGroup>
            </SelectContent>
          </Select>
        </div>
        <Separator orientation="vertical" className="h-7" />
        <p className="text-sm text-muted-foreground">
          关键事件按时间倒序展示，可进入对应 Issue 或 Session。
        </p>
      </div>

      {tasks.length === 0 ? (
        <Card>
          <CardContent className="py-16">
            <Empty>
              <EmptyHeader>
                <EmptyMedia variant="icon">
                  <Activity />
                </EmptyMedia>
                <EmptyTitle>还没有任务时间线</EmptyTitle>
                <EmptyDescription>
                  发布第一个任务后，关键事件会自动出现在这里。
                </EmptyDescription>
              </EmptyHeader>
            </Empty>
          </CardContent>
        </Card>
      ) : (
        <TimelinePanel taskId={resolvedTaskID} />
      )}
    </div>
  )
}
