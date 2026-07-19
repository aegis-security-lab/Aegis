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
import { Button } from "@/components/ui/button"
import { Spinner } from "@/components/ui/spinner"
import { interruptCurrentTool } from "@/lib/api"
import type { Execution } from "@/types"

interface InterruptToolButtonProps {
  execution: Execution
  onInterrupted?: () => void | Promise<void>
}

export function InterruptToolButton({
  execution,
  onInterrupted,
}: InterruptToolButtonProps) {
  const [open, setOpen] = React.useState(false)
  const [busy, setBusy] = React.useState(false)
  const tool = execution.currentTool?.trim()
  const supported =
    execution.status === "running" &&
    Boolean(tool) &&
    !["validation", "concierge"].includes(execution.kind)

  if (!supported || !tool) return null

  const interrupt = async () => {
    setBusy(true)
    try {
      const result = await interruptCurrentTool(execution.id)
      setOpen(false)
      toast.success(`正在中断 ${result.tool}`, {
        description:
          "Session 和 Issue 会保留，工具停止后可通过评论或对话继续。",
      })
      await onInterrupted?.()
    } catch (error) {
      toast.error(error instanceof Error ? error.message : "中断工具失败")
    } finally {
      setBusy(false)
    }
  }

  return (
    <AlertDialog open={open} onOpenChange={setOpen}>
      <Button variant="outline" disabled={busy} onClick={() => setOpen(true)}>
        <CircleStop data-icon="inline-start" />
        中断工具
      </Button>
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogMedia>
            <CircleStop />
          </AlertDialogMedia>
          <AlertDialogTitle>中断当前工具？</AlertDialogTitle>
          <AlertDialogDescription>
            正在执行的 {tool} 会收到取消信号，当前 Agent 轮次会停止，但不会取消
            Issue 或关闭
            Session。已经产生的文件、请求或其他副作用不会自动回滚；中断后可通过评论或对话发送新的继续指令。
          </AlertDialogDescription>
        </AlertDialogHeader>
        <AlertDialogFooter>
          <AlertDialogCancel disabled={busy}>返回</AlertDialogCancel>
          <AlertDialogAction
            variant="destructive"
            disabled={busy}
            onClick={() => void interrupt()}
          >
            {busy ? (
              <Spinner data-icon="inline-start" />
            ) : (
              <CircleStop data-icon="inline-start" />
            )}
            确认中断
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  )
}
