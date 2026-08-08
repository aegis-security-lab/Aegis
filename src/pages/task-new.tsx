import * as React from "react"
import { Copy, Send } from "lucide-react"
import { useLocation, useNavigate } from "react-router-dom"
import { toast } from "sonner"

import { IssueBriefComposer } from "@/components/issue-brief-composer"
import {
  HumanValidationToggle,
  ObjectiveToggle,
  TimeBudgetControl,
} from "@/components/issue-option-controls"
import { useInputAttachments } from "@/hooks/use-input-attachments"
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
import { createTask } from "@/lib/api"
import { limitIssueTitle } from "@/lib/issue-title"
import { useAppState } from "@/lib/state"
import type { CreateIssueInput } from "@/types"

const priorities: CreateIssueInput["priority"][] = ["high", "middle", "low"]

export function TaskNewPage() {
  const { state } = useAppState()
  const navigate = useNavigate()
  const location = useLocation()
  const clone = (location.state as { clone?: Partial<CreateIssueInput> } | null)
    ?.clone
  const hasBackground = Boolean(
    (location.state as { backgroundLocation?: unknown } | null)
      ?.backgroundLocation
  )
  const [busy, setBusy] = React.useState(false)
  const [objectiveEnabled, setObjectiveEnabled] = React.useState(
    Boolean(clone?.objective)
  )
  const attachments = useInputAttachments()
  const [form, setForm] = React.useState<CreateIssueInput>({
    projectId: clone?.projectId ?? state?.projects[0]?.id,
    title: clone?.title ?? "",
    description: clone?.description ?? "",
    objective: clone?.objective ?? "",
    priority: clone?.priority ?? "high",
    assigneeAgentId: clone?.assigneeAgentId,
    containerProfileId: clone?.containerProfileId,
    workMode: "autonomous",
    workspace: state?.config.workspace ?? "",
    constraints: "",
    timeBudgetMinutes: clone?.timeBudgetMinutes,
    humanValidationFallback: clone?.humanValidationFallback,
  })

  const enabledAgentTypes = (state?.agents ?? []).filter(
    (agent) =>
      agent.enabled && !agent.internal && agent.category !== "concierge"
  )
  const enabledContainerProfiles = (state?.containerProfiles ?? []).filter(
    (profile) => profile.enabled
  )
  const defaultContainerProfile =
    enabledContainerProfiles.find(
      (profile) => profile.name.toLowerCase() === "default"
    ) ?? enabledContainerProfiles[0]
  const defaultAgent =
    enabledAgentTypes.find((agent) => agent.id === "aegis-orchestrator") ??
    enabledAgentTypes[0]
  const selectedAgentId = form.assigneeAgentId || defaultAgent?.id || ""
  const selectedAgent = enabledAgentTypes.find(
    (agent) => agent.id === selectedAgentId
  )
  const priorityItems = priorities.map((priority) => ({
    label: priority,
    value: priority,
  }))

  const update = <K extends keyof CreateIssueInput>(
    key: K,
    value: CreateIssueInput[K]
  ) => setForm((current) => ({ ...current, [key]: value }))

  const submit = async (event: React.FormEvent) => {
    event.preventDefault()
    const description = form.description.trim()
    if (!description) {
      toast.error("请填写任务说明")
      return
    }
    if (!selectedAgentId) {
      toast.error("请选择一个 Agent 类型")
      return
    }
    if (!defaultContainerProfile) {
      toast.error("没有可用的 Docker 容器配置，请先在容器管理中启用一个配置")
      return
    }
    setBusy(true)
    try {
      const title = form.title.trim() || limitIssueTitle(description)
      const { task, issue } = await createTask({
        ...form,
        title,
        description,
        objective: objectiveEnabled ? form.objective : "",
        constraints: "",
        assigneeAgentId: selectedAgentId,
        containerProfileId: defaultContainerProfile.id,
        attachmentIds: attachments.attachmentIds,
      })
      attachments.clearBound()
      toast.success(`任务已交给 ${selectedAgent?.name ?? selectedAgentId}`, {
        description: issue.identifier,
      })
      navigate(`/tasks/${task.id}`)
    } catch (reason) {
      toast.error(reason instanceof Error ? reason.message : "发布失败")
    } finally {
      setBusy(false)
    }
  }

  return (
    <Dialog
      open
      onOpenChange={(open) => {
        if (!open) {
          if (hasBackground) navigate(-1)
          else navigate("/tasks", { replace: true })
        }
      }}
    >
      <DialogContent className="sm:max-w-4xl">
        <DialogHeader>
          <DialogTitle>
            {clone ? (
              <span className="flex items-center gap-2">
                <Copy className="size-4" />
                复制为新任务
              </span>
            ) : (
              "新建任务"
            )}
          </DialogTitle>
        </DialogHeader>
        <form onSubmit={submit}>
          <IssueBriefComposer
            idPrefix="task"
            title={form.title}
            description={form.description}
            onTitleChange={(value) => update("title", value)}
            onDescriptionChange={(value) => update("description", value)}
            agents={enabledAgentTypes}
            selectedAgentId={selectedAgentId}
            onAgentChange={(value) =>
              update("assigneeAgentId", value || undefined)
            }
            attachments={attachments}
            objective={form.objective}
            objectiveEnabled={objectiveEnabled}
            onObjectiveChange={(value) => update("objective", value)}
            footerControls={
              <>
                <Select
                  items={priorityItems}
                  value={form.priority}
                  onValueChange={(value) =>
                    update("priority", value as CreateIssueInput["priority"])
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
                  onValueChange={(value) => update("timeBudgetMinutes", value)}
                />
                <HumanValidationToggle
                  checked={form.humanValidationFallback ?? false}
                  onCheckedChange={(checked) =>
                    update("humanValidationFallback", checked)
                  }
                />
              </>
            }
          />
          <DialogFooter className="mt-5">
            <Button
              type="submit"
              size="lg"
              disabled={
                busy ||
                attachments.uploading ||
                attachments.hasErrors ||
                !form.description.trim() ||
                !selectedAgentId ||
                !defaultContainerProfile
              }
            >
              {busy ? (
                <Spinner data-icon="inline-start" />
              ) : (
                <Send data-icon="inline-start" />
              )}
              发布并执行
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}
