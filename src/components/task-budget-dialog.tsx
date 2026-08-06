import * as React from "react"
import { Save } from "lucide-react"
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
import {
  Field,
  FieldDescription,
  FieldGroup,
  FieldLabel,
} from "@/components/ui/field"
import { Input } from "@/components/ui/input"
import { Spinner } from "@/components/ui/spinner"
import { updateTaskBudget } from "@/lib/api"

interface TaskBudgetDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  taskId: string
  taskTitle: string
  currentMinutes?: number | null
  onSaved?: () => void | Promise<void>
}

export function TaskBudgetDialog({
  open,
  onOpenChange,
  taskId,
  taskTitle,
  currentMinutes,
  onSaved,
}: TaskBudgetDialogProps) {
  const formId = React.useId()
  const inputId = React.useId()
  const [minutes, setMinutes] = React.useState(
    currentMinutes ? String(currentMinutes) : ""
  )
  const [saving, setSaving] = React.useState(false)

  const save = async (event: React.FormEvent<HTMLFormElement>) => {
    event.preventDefault()
    const value = Number.parseInt(minutes, 10)
    if (!Number.isFinite(value) || value <= 0) {
      toast.error("请输入大于 0 的分钟数")
      return
    }
    setSaving(true)
    try {
      await updateTaskBudget(taskId, value)
      await onSaved?.()
      toast.success("任务时间预算已更新")
      onOpenChange(false)
    } catch (error) {
      toast.error(error instanceof Error ? error.message : "更新时间预算失败")
    } finally {
      setSaving(false)
    }
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>调整时间预算</DialogTitle>
          <DialogDescription>
            为「{taskTitle}」设置总时钟墙预算；已用时间从任务创建时开始累计，
            等待、休眠和重新执行都不会重置。
          </DialogDescription>
        </DialogHeader>

        <form id={formId} onSubmit={save}>
          <FieldGroup>
            <Field>
              <FieldLabel htmlFor={inputId}>时间预算（分钟）</FieldLabel>
              <Input
                id={inputId}
                type="number"
                min={1}
                value={minutes}
                onChange={(event) => setMinutes(event.target.value)}
                placeholder="例如 60"
                disabled={saving}
                autoFocus
              />
              <FieldDescription>
                预算只对任务总运行时长做硬性约束，不影响子 Issue
                的单次 Execution 预算。
              </FieldDescription>
            </Field>
          </FieldGroup>
        </form>

        <DialogFooter>
          <Button
            variant="outline"
            onClick={() => onOpenChange(false)}
            disabled={saving}
          >
            取消
          </Button>
          <Button
            type="submit"
            form={formId}
            disabled={saving || !minutes.trim()}
          >
            {saving ? (
              <Spinner data-icon="inline-start" />
            ) : (
              <Save data-icon="inline-start" />
            )}
            保存预算
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
