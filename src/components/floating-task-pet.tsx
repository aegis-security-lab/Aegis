import * as React from "react"
import { Bot, ChevronRight, Grip, ListTodo } from "lucide-react"
import { useNavigate } from "react-router-dom"

import { StatusBadge } from "@/components/status-badge"
import { Badge } from "@/components/ui/badge"
import { useAppState } from "@/lib/state"
import { cn } from "@/lib/utils"

type Position = { x: number; y: number }

const storageKey = "aegis-floating-task-pet-position"
const activeStatuses = new Set([
  "queued",
  "starting",
  "running",
  "waiting_approval",
])

function storedPosition(): Position | null {
  try {
    const value = JSON.parse(localStorage.getItem(storageKey) ?? "null")
    return typeof value?.x === "number" && typeof value?.y === "number"
      ? value
      : null
  } catch {
    return null
  }
}

function clampPosition(position: Position): Position {
  const petSize = 68
  const edge = 12
  const minimumY = Math.min(340, window.innerHeight - petSize - edge)
  return {
    x: Math.max(edge, Math.min(position.x, window.innerWidth - petSize - edge)),
    y: Math.max(
      Math.max(edge, minimumY),
      Math.min(position.y, window.innerHeight - petSize - edge)
    ),
  }
}

export function FloatingTaskPet() {
  const { state } = useAppState()
  const navigate = useNavigate()
  const [open, setOpen] = React.useState(false)
  const [position, setPosition] = React.useState<Position | null>(() =>
    storedPosition()
  )
  const drag = React.useRef<{
    pointerId: number
    startX: number
    startY: number
    origin: Position
    moved: boolean
  } | null>(null)

  const activeExecutions = (state?.executions ?? [])
    .filter((execution) => activeStatuses.has(execution.status))
    .sort((left, right) => right.updatedAt.localeCompare(left.updatedAt))
  const visibleExecutions = activeExecutions.slice(0, 8)

  React.useEffect(() => {
    const onResize = () =>
      setPosition((current) => (current ? clampPosition(current) : current))
    window.addEventListener("resize", onResize)
    return () => window.removeEventListener("resize", onResize)
  }, [])

  const currentPosition = () => {
    if (position) return clampPosition(position)
    return {
      x: window.innerWidth - 88,
      y: window.innerHeight - 88,
    }
  }

  const pointerDown = (event: React.PointerEvent<HTMLButtonElement>) => {
    const origin = currentPosition()
    drag.current = {
      pointerId: event.pointerId,
      startX: event.clientX,
      startY: event.clientY,
      origin,
      moved: false,
    }
    event.currentTarget.setPointerCapture(event.pointerId)
  }

  const pointerMove = (event: React.PointerEvent<HTMLButtonElement>) => {
    const current = drag.current
    if (!current || current.pointerId !== event.pointerId) return
    const dx = event.clientX - current.startX
    const dy = event.clientY - current.startY
    if (Math.abs(dx) + Math.abs(dy) > 5) current.moved = true
    setPosition(
      clampPosition({ x: current.origin.x + dx, y: current.origin.y + dy })
    )
  }

  const pointerUp = (event: React.PointerEvent<HTMLButtonElement>) => {
    const current = drag.current
    if (!current || current.pointerId !== event.pointerId) return
    event.currentTarget.releasePointerCapture(event.pointerId)
    drag.current = null
    if (position) localStorage.setItem(storageKey, JSON.stringify(position))
    if (!current.moved) setOpen((value) => !value)
  }

  return (
    <div
      className={cn(
        "pointer-events-none fixed z-50",
        position === null && "right-5 bottom-5"
      )}
      style={position ? { left: position.x, top: position.y } : undefined}
    >
      {open ? (
        <div className="pointer-events-auto absolute right-0 bottom-[76px] w-[min(340px,calc(100vw-24px))] overflow-hidden rounded-2xl border bg-popover text-popover-foreground shadow-2xl">
          <div className="flex items-center justify-between border-b bg-muted/40 px-4 py-3">
            <div>
              <p className="text-sm font-semibold">正在执行的任务</p>
              <p className="text-xs text-muted-foreground">点击任务查看详情</p>
            </div>
            <Badge variant="secondary">{activeExecutions.length}</Badge>
          </div>
          <div className="max-h-72 overflow-y-auto p-2">
            {visibleExecutions.length === 0 ? (
              <div className="flex flex-col items-center gap-2 px-4 py-8 text-center text-muted-foreground">
                <ListTodo className="size-6" />
                <p className="text-sm">当前没有正在执行的任务</p>
              </div>
            ) : (
              visibleExecutions.map((execution) => {
                const issue = state?.issues.find(
                  (item) => item.id === execution.issueId
                )
                const agent = state?.agents.find(
                  (item) => item.id === execution.agentId
                )
                return (
                  <button
                    key={execution.id}
                    type="button"
                    className="flex w-full items-center gap-3 rounded-xl px-3 py-3 text-left transition-colors hover:bg-muted"
                    onClick={() => {
                      setOpen(false)
                      navigate(`/issues/${execution.issueId}`)
                    }}
                  >
                    <span className="flex size-9 shrink-0 items-center justify-center rounded-full bg-primary/10 text-primary">
                      <Bot className="size-4" />
                    </span>
                    <span className="min-w-0 flex-1">
                      <span className="flex items-center gap-2">
                        <span className="truncate text-sm font-medium">
                          {issue?.identifier ?? execution.issueId}
                        </span>
                        <StatusBadge
                          status={execution.status}
                          className="shrink-0 text-[10px]"
                        />
                      </span>
                      <span className="mt-1 block truncate text-xs text-muted-foreground">
                        {execution.checkpoint ||
                          execution.currentTool ||
                          issue?.title ||
                          "等待 Agent 更新"}
                      </span>
                      <span className="mt-0.5 block truncate text-[11px] text-muted-foreground/80">
                        {agent?.name ?? execution.agentId}
                      </span>
                    </span>
                    <ChevronRight className="size-4 shrink-0 text-muted-foreground" />
                  </button>
                )
              })
            )}
          </div>
          {activeExecutions.length > visibleExecutions.length ? (
            <div className="border-t px-4 py-2 text-center text-xs text-muted-foreground">
              还有 {activeExecutions.length - visibleExecutions.length} 个执行
            </div>
          ) : null}
        </div>
      ) : null}

      <button
        type="button"
        aria-label={open ? "收起正在执行的任务" : "展开正在执行的任务"}
        className="group pointer-events-auto relative flex size-[68px] touch-none items-center justify-center rounded-[24px] border-2 border-white/70 bg-gradient-to-br from-cyan-400 via-blue-500 to-violet-600 text-3xl shadow-xl shadow-blue-500/25 transition-transform select-none hover:scale-105 active:scale-95 dark:border-white/20"
        onPointerDown={pointerDown}
        onPointerMove={pointerMove}
        onPointerUp={pointerUp}
        onPointerCancel={() => {
          drag.current = null
        }}
      >
        <span
          aria-hidden="true"
          className={cn(
            "drop-shadow-sm transition-transform group-hover:scale-110 group-hover:-rotate-6",
            activeExecutions.length > 0 && "animate-bounce"
          )}
        >
          🦊
        </span>
        <span className="absolute -top-1 -left-1 flex size-5 items-center justify-center rounded-full bg-background/90 text-muted-foreground shadow-sm">
          <Grip className="size-3" />
        </span>
        {activeExecutions.length > 0 ? (
          <span className="absolute -top-2 -right-2 flex min-w-6 items-center justify-center rounded-full bg-emerald-500 px-1.5 py-1 text-[11px] font-bold text-white shadow">
            {activeExecutions.length > 99 ? "99+" : activeExecutions.length}
          </span>
        ) : null}
      </button>
    </div>
  )
}
