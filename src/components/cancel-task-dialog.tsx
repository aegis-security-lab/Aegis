import * as React from "react"
import { CircleStop } from "lucide-react"
import { toast } from "sonner"

import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogMedia,
  AlertDialogTitle,
} from "@/components/ui/alert-dialog"
import {
  Field,
  FieldDescription,
  FieldError,
  FieldGroup,
  FieldLabel,
} from "@/components/ui/field"
import { Spinner } from "@/components/ui/spinner"
import { Textarea } from "@/components/ui/textarea"
import { cancelTask } from "@/lib/api"
import type { TaskCancellationResult } from "@/types"

const DEFAULT_CANCELLATION_REASON = "不想继续执行，取消任务，无其他原因。"

interface CancelTaskDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  taskId: string
  taskTitle: string
  totalIssues: number
  activeExecutions: number
  onCancelled?: (result: TaskCancellationResult) => void | Promise<void>
}

export function CancelTaskDialog({
  open,
  onOpenChange,
  taskId,
  taskTitle,
  totalIssues,
  activeExecutions,
  onCancelled,
}: CancelTaskDialogProps) {
  const formId = React.useId()
  const reasonId = React.useId()
  const [reason, setReason] = React.useState(DEFAULT_CANCELLATION_REASON)
  const [error, setError] = React.useState("")
  const [submitting, setSubmitting] = React.useState(false)

  const handleOpenChange = (nextOpen: boolean) => {
    if (submitting) return
    if (!nextOpen) {
      setReason(DEFAULT_CANCELLATION_REASON)
      setError("")
    }
    onOpenChange(nextOpen)
  }

  const submit = async (event: React.FormEvent<HTMLFormElement>) => {
    event.preventDefault()
    const normalizedReason = reason.trim()
    if (!normalizedReason) {
      setError("请填写取消原因，便于后续审计和恢复判断。")
      return
    }
    setSubmitting(true)
    setError("")
    try {
      const result = await cancelTask(taskId, normalizedReason)
      let refreshFailed = false
      try {
        await onCancelled?.(result)
      } catch {
        refreshFailed = true
      }
      toast.success("任务已取消", {
        description: refreshFailed
          ? "取消已生效，但页面状态刷新失败，请手动刷新。"
          : `已停止 ${result.cancelledIssues} 个未完成 Issue 和 ${result.cancelledExecutions} 个活跃执行。`,
      })
      onOpenChange(false)
      setReason(DEFAULT_CANCELLATION_REASON)
    } catch (caught) {
      setError(caught instanceof Error ? caught.message : "取消任务失败")
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <AlertDialog open={open} onOpenChange={handleOpenChange}>
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogMedia>
            <CircleStop />
          </AlertDialogMedia>
          <AlertDialogTitle>取消整个任务？</AlertDialogTitle>
          <AlertDialogDescription>
            “{taskTitle}”及其任务树中的未完成工作会立即停止。当前共涉及{" "}
            {totalIssues} 个 Issue
            {activeExecutions > 0
              ? `，其中 ${activeExecutions} 个执行仍在运行`
              : ""}
            。历史对话、日志和附件会保留，可在之后重新启动任务。
          </AlertDialogDescription>
        </AlertDialogHeader>

        <form id={formId} onSubmit={submit}>
          <FieldGroup>
            <Field data-invalid={Boolean(error)}>
              <FieldLabel htmlFor={reasonId}>取消原因</FieldLabel>
              <Textarea
                id={reasonId}
                value={reason}
                onChange={(event) => {
                  setReason(event.target.value)
                  if (error) setError("")
                }}
                placeholder="例如：目标已变更，停止当前方向"
                maxLength={500}
                disabled={submitting}
                aria-invalid={Boolean(error)}
                autoFocus
              />
              <FieldDescription>原因会写入任务审计记录。</FieldDescription>
              <FieldError>{error}</FieldError>
            </Field>
          </FieldGroup>
        </form>

        <AlertDialogFooter>
          <AlertDialogCancel disabled={submitting}>返回</AlertDialogCancel>
          <AlertDialogAction
            type="submit"
            form={formId}
            variant="destructive"
            disabled={submitting || !reason.trim()}
          >
            {submitting ? (
              <Spinner data-icon="inline-start" />
            ) : (
              <CircleStop data-icon="inline-start" />
            )}
            {submitting ? "正在取消…" : "确认取消任务"}
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  )
}
