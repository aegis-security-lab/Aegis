import * as React from "react"
import { MessageSquareWarning } from "lucide-react"
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
import { Textarea } from "@/components/ui/textarea"
import { Spinner } from "@/components/ui/spinner"
import { manuallyRejectIssueValidation } from "@/lib/api"
import type { Issue } from "@/types"

interface ManualRejectValidationDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  issueId: string
  onRejected?: (issue: Issue) => void | Promise<void>
}

export function ManualRejectValidationDialog({
  open,
  onOpenChange,
  issueId,
  onRejected,
}: ManualRejectValidationDialogProps) {
  const formId = React.useId()
  const reasonId = React.useId()
  const [reason, setReason] = React.useState("")
  const [error, setError] = React.useState("")
  const [submitting, setSubmitting] = React.useState(false)

  const handleOpenChange = (nextOpen: boolean) => {
    if (submitting) return
    if (!nextOpen) {
      setReason("")
      setError("")
    }
    onOpenChange(nextOpen)
  }

  const submit = async (event: React.FormEvent<HTMLFormElement>) => {
    event.preventDefault()
    const normalizedReason = reason.trim()
    if (!normalizedReason) {
      setError("请填写改判原因，验收 Agent 会以此作为后续验收基准。")
      return
    }
    setSubmitting(true)
    setError("")
    try {
      const issue = await manuallyRejectIssueValidation(issueId, normalizedReason)
      await onRejected?.(issue)
      toast.success("已改判验收不通过", {
        description: "验收 Agent 将以改判原因作为基准继续核验。",
      })
      onOpenChange(false)
      setReason("")
    } catch (caught) {
      setError(caught instanceof Error ? caught.message : "改判验收失败")
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <AlertDialog open={open} onOpenChange={handleOpenChange}>
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogMedia>
            <MessageSquareWarning />
          </AlertDialogMedia>
          <AlertDialogTitle>手动改判验收不通过？</AlertDialogTitle>
          <AlertDialogDescription>
            验收通过的 Issue 会回到执行中，固定验收 Agent 会以你填写的原因作为后续验收基准继续核验。
          </AlertDialogDescription>
        </AlertDialogHeader>

        <form id={formId} onSubmit={submit}>
          <FieldGroup>
            <Field data-invalid={Boolean(error)}>
              <FieldLabel htmlFor={reasonId}>改判原因</FieldLabel>
              <Textarea
                id={reasonId}
                value={reason}
                onChange={(event) => {
                  setReason(event.target.value)
                  if (error) setError("")
                }}
                placeholder="例如：交付物存在未修复的安全问题，验收结论不成立"
                maxLength={10000}
                disabled={submitting}
                aria-invalid={Boolean(error)}
                autoFocus
                rows={4}
              />
              <FieldDescription>
                原因会写入验收记录，并作为后续验收的基准依据。
              </FieldDescription>
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
              <MessageSquareWarning data-icon="inline-start" />
            )}
            {submitting ? "正在改判…" : "确认改判不通过"}
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  )
}
