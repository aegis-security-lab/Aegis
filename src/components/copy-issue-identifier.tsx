import { toast } from "sonner"

import { cn } from "@/lib/utils"

export function CopyIssueIdentifier({ identifier, className }: { identifier: string; className?: string }) {
  const copy = async () => {
    try {
      if (navigator.clipboard?.writeText) {
        await navigator.clipboard.writeText(identifier)
      } else {
        const input = document.createElement("textarea")
        input.value = identifier
        input.style.position = "fixed"
        input.style.opacity = "0"
        document.body.appendChild(input)
        input.select()
        const copied = document.execCommand("copy")
        input.remove()
        if (!copied) throw new Error("copy command failed")
      }
      toast.success(`已复制 ${identifier}`)
    } catch {
      toast.error("复制失败，请检查浏览器剪贴板权限")
    }
  }

  return (
    <button
      type="button"
      draggable={false}
      className={cn(
        "-mx-1 inline-flex min-h-6 shrink-0 items-center rounded px-1 font-mono text-[11px] text-muted-foreground tabular-nums outline-none transition-colors hover:bg-muted hover:text-foreground focus-visible:ring-2 focus-visible:ring-ring/60",
        className
      )}
      aria-label={`复制 Issue ID ${identifier}`}
      title={`点击复制 ${identifier}`}
      onPointerDown={(event) => event.stopPropagation()}
      onClick={(event) => {
        event.stopPropagation()
        void copy()
      }}
    >
      {identifier}
    </button>
  )
}
