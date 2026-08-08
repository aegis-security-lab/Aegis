import * as React from "react"
import { toast } from "sonner"

import { Button } from "@/components/ui/button"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import { Spinner } from "@/components/ui/spinner"
import { Textarea } from "@/components/ui/textarea"
import { createIssue } from "@/lib/api"
import { workflowStatuses, workflowStatusLabels } from "@/lib/issue-workflow"
import { useAppState } from "@/lib/state"
import type { CreateIssueInput, Issue, IssueStatus } from "@/types"

const priorities: CreateIssueInput["priority"][] = ["high", "middle", "low"]

const statuses: IssueStatus[] = workflowStatuses

const statusLabels = workflowStatusLabels

type CreateIssueForm = {
  title: string
  objective: string
  description: string
  priority: CreateIssueInput["priority"]
  status: IssueStatus
  assigneeAgentId: string
}

export function CreateIssueDialog({
  open,
  onOpenChange,
  defaultStatus = "todo",
  workspace,
  taskSourceId,
  title = "新建 Issue",
  description = "在当前任务下创建新的根 Issue，创建后立即加入看板。",
  onCreated,
}: {
  open: boolean
  onOpenChange: (open: boolean) => void
  defaultStatus?: IssueStatus
  workspace?: string
  taskSourceId: string
  title?: string
  description?: string
  onCreated?: (issue: Issue) => void
}) {
  const { state, refresh } = useAppState()
  const [busy, setBusy] = React.useState(false)
  const [form, setForm] = React.useState<CreateIssueForm>(() =>
    emptyForm(defaultStatus)
  )

  const handleOpenChange = (next: boolean) => {
    if (next) setForm(emptyForm(defaultStatus))
    onOpenChange(next)
  }

  const submit = async () => {
    const issueTitle = form.title.trim()
    if (!issueTitle || busy) return
    setBusy(true)
    try {
      const issue = await createIssue({
        taskSourceId,
        title: issueTitle,
        objective: form.objective,
        description: form.description,
        priority: form.priority,
        status: form.status,
        assigneeAgentId: form.assigneeAgentId || undefined,
        workspace: workspace ?? state?.config.workspace ?? "",
        workMode: "autonomous",
        constraints: "",
      })
      toast.success(`已创建 ${issue.identifier}`)
      onCreated?.(issue)
      onOpenChange(false)
      void refresh()
    } catch (error) {
      toast.error(error instanceof Error ? error.message : "创建 Issue 失败")
    } finally {
      setBusy(false)
    }
  }

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogContent className="sm:max-w-2xl">
        <DialogHeader>
          <DialogTitle>{title}</DialogTitle>
          <DialogDescription>{description}</DialogDescription>
        </DialogHeader>
        <div className="grid gap-4 py-2">
          <div className="grid gap-2">
            <Label htmlFor="create-issue-title">标题</Label>
            <Input
              id="create-issue-title"
              autoFocus
              value={form.title}
              onChange={(event) =>
                setForm((current) => ({
                  ...current,
                  title: event.target.value,
                }))
              }
              onKeyDown={(event) => {
                if (event.key === "Enter" && !event.shiftKey) {
                  event.preventDefault()
                  void submit()
                }
              }}
            />
          </div>
          <div className="grid gap-2">
            <Label htmlFor="create-issue-objective">目标（可选）</Label>
            <Textarea
              id="create-issue-objective"
              rows={3}
              value={form.objective}
              onChange={(event) =>
                setForm((current) => ({
                  ...current,
                  objective: event.target.value,
                }))
              }
            />
          </div>
          <div className="grid gap-2">
            <Label htmlFor="create-issue-description">描述（可选）</Label>
            <Textarea
              id="create-issue-description"
              rows={3}
              value={form.description}
              onChange={(event) =>
                setForm((current) => ({
                  ...current,
                  description: event.target.value,
                }))
              }
            />
          </div>
          <div className="grid gap-4 sm:grid-cols-3">
            <div className="grid gap-2">
              <Label>状态</Label>
              <Select
                value={form.status}
                onValueChange={(value) =>
                  value &&
                  setForm((current) => ({
                    ...current,
                    status: value as IssueStatus,
                  }))
                }
              >
                <SelectTrigger>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectGroup>
                    {statuses.map((status) => (
                      <SelectItem key={status} value={status}>
                        {statusLabels[status]}
                      </SelectItem>
                    ))}
                  </SelectGroup>
                </SelectContent>
              </Select>
            </div>
            <div className="grid gap-2">
              <Label>优先级</Label>
              <Select
                value={form.priority}
                onValueChange={(value) =>
                  value &&
                  setForm((current) => ({
                    ...current,
                    priority: value as CreateIssueInput["priority"],
                  }))
                }
              >
                <SelectTrigger>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectGroup>
                    {priorities.map((priority) => (
                      <SelectItem key={priority} value={priority}>
                        {priority}
                      </SelectItem>
                    ))}
                  </SelectGroup>
                </SelectContent>
              </Select>
            </div>
            <div className="grid gap-2">
              <Label>负责人（可选）</Label>
              <Select
                value={form.assigneeAgentId || "unassigned"}
                onValueChange={(value) =>
                  setForm((current) => ({
                    ...current,
                    assigneeAgentId:
                      !value || value === "unassigned" ? "" : value,
                  }))
                }
              >
                <SelectTrigger>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectGroup>
                    <SelectItem value="unassigned">未委派</SelectItem>
                    {(state?.agents ?? [])
                      .filter(
                        (agent) =>
                          agent.enabled &&
                          !agent.internal &&
                          agent.category !== "concierge"
                      )
                      .map((agent) => (
                        <SelectItem key={agent.id} value={agent.id}>
                          {agent.name} · {agent.category}
                        </SelectItem>
                      ))}
                  </SelectGroup>
                </SelectContent>
              </Select>
            </div>
          </div>
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            取消
          </Button>
          <Button
            disabled={busy || !form.title.trim()}
            onClick={() => void submit()}
          >
            {busy ? <Spinner /> : null}
            创建 Issue
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

function emptyForm(defaultStatus: IssueStatus): CreateIssueForm {
  return {
    title: "",
    objective: "",
    description: "",
    priority: "middle",
    status: defaultStatus,
    assigneeAgentId: "",
  }
}
