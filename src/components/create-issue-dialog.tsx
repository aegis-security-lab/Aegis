import * as React from "react"
import { toast } from "sonner"

import { IssueBriefComposer } from "@/components/issue-brief-composer"
import {
  HumanValidationToggle,
  ObjectiveToggle,
  TimeBudgetControl,
} from "@/components/issue-option-controls"
import { Button } from "@/components/ui/button"
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import { Spinner } from "@/components/ui/spinner"
import { useInputAttachments } from "@/hooks/use-input-attachments"
import { createIssue } from "@/lib/api"
import { limitIssueTitle } from "@/lib/issue-title"
import { issuesForTask } from "@/lib/collections"
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
  timeBudgetMinutes?: number
  humanValidationFallback: boolean
}

export function CreateIssueDialog({
  open,
  onOpenChange,
  defaultStatus = "todo",
  workspace,
  taskSourceId,
  title = "新建 Issue",
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
  const [objectiveEnabled, setObjectiveEnabled] = React.useState(false)
  const attachments = useInputAttachments({
    scopeKey: `create-issue:${taskSourceId}`,
  })
  const [form, setForm] = React.useState<CreateIssueForm>(() =>
    emptyForm(defaultStatus)
  )

  const enabledAgentTypes = (state?.agents ?? []).filter(
    (agent) =>
      agent.enabled && !agent.internal && agent.category !== "concierge"
  )
  const taskIssues = issuesForTask(taskSourceId, state?.issues ?? [])

  const handleOpenChange = (next: boolean) => {
    if (next) {
      setForm(emptyForm(defaultStatus))
      setObjectiveEnabled(false)
    } else attachments.discard()
    onOpenChange(next)
  }

  const submit = async () => {
    const issueDescription = form.description.trim()
    if (
      !issueDescription ||
      busy ||
      attachments.uploading ||
      attachments.hasErrors
    )
      return
    setBusy(true)
    try {
      const issueTitle = form.title.trim() || limitIssueTitle(issueDescription)
      const issue = await createIssue({
        taskSourceId,
        title: issueTitle,
        objective: objectiveEnabled ? form.objective : "",
        description: issueDescription,
        priority: form.priority,
        status: form.status,
        assigneeAgentId: form.assigneeAgentId || undefined,
        workspace: workspace ?? state?.config.workspace ?? "",
        workMode: "autonomous",
        attachmentIds: attachments.attachmentIds,
        timeBudgetMinutes: form.timeBudgetMinutes,
        humanValidationFallback: form.humanValidationFallback,
      })
      attachments.clearBound()
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
      <DialogContent className="sm:max-w-4xl">
        <DialogHeader>
          <DialogTitle>{title}</DialogTitle>
        </DialogHeader>
        <div className="py-2">
          <IssueBriefComposer
            idPrefix="create-issue"
            title={form.title}
            description={form.description}
            onTitleChange={(value) =>
              setForm((current) => ({ ...current, title: value }))
            }
            onDescriptionChange={(value) =>
              setForm((current) => ({ ...current, description: value }))
            }
            agents={enabledAgentTypes}
            selectedAgentId={form.assigneeAgentId}
            onAgentChange={(value) =>
              setForm((current) => ({ ...current, assigneeAgentId: value }))
            }
            attachments={attachments}
            allowUnassigned
            autoFocus
            mentionIssues={taskIssues}
            objective={form.objective}
            objectiveEnabled={objectiveEnabled}
            onObjectiveChange={(value) =>
              setForm((current) => ({ ...current, objective: value }))
            }
            footerControls={
              <>
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
                  <SelectTrigger size="sm" className="w-auto min-w-28">
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
                  <SelectTrigger size="sm" className="w-auto min-w-24">
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
                <ObjectiveToggle
                  checked={objectiveEnabled}
                  onCheckedChange={setObjectiveEnabled}
                />
                <TimeBudgetControl
                  value={form.timeBudgetMinutes}
                  onValueChange={(value) =>
                    setForm((current) => ({
                      ...current,
                      timeBudgetMinutes: value,
                    }))
                  }
                />
                <HumanValidationToggle
                  checked={form.humanValidationFallback}
                  onCheckedChange={(checked) =>
                    setForm((current) => ({
                      ...current,
                      humanValidationFallback: checked,
                    }))
                  }
                />
              </>
            }
          />
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={() => handleOpenChange(false)}>
            取消
          </Button>
          <Button
            disabled={
              busy ||
              attachments.uploading ||
              attachments.hasErrors ||
              !form.description.trim()
            }
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
    timeBudgetMinutes: undefined,
    humanValidationFallback: false,
  }
}
