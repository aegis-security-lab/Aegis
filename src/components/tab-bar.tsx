import * as React from "react"
import { Plus, X } from "lucide-react"

import { Button } from "@/components/ui/button"
import { cn } from "@/lib/utils"

export type TabItem = {
  id: string
  path: string
  title: string
}

type TabBarProps = {
  tabs: TabItem[]
  activeId: string
  onActivate: (id: string) => void
  onAdd: () => void
  onRemove: (id: string) => void
  onReorder: (fromId: string, toId: string) => void
}

export function TabBar({
  tabs,
  activeId,
  onActivate,
  onAdd,
  onRemove,
  onReorder,
}: TabBarProps) {
  const [dragId, setDragId] = React.useState<string | null>(null)
  const tabRefs = React.useRef(new Map<string, HTMLButtonElement>())

  const focusTab = (index: number) => {
    const tab = tabs[(index + tabs.length) % tabs.length]
    if (!tab) return
    onActivate(tab.id)
    window.requestAnimationFrame(() => tabRefs.current.get(tab.id)?.focus())
  }

  return (
    <div
      role="tablist"
      aria-label="页面标签"
      className="flex h-10 shrink-0 items-center gap-1.5 overflow-x-auto px-1.5 sm:px-2"
    >
      {tabs.map((tab) => {
        const active = tab.id === activeId
        const isDropTarget = dragId !== null && dragId !== tab.id
        return (
          <div
            key={tab.id}
            draggable
            onDragStart={(event) => {
              setDragId(tab.id)
              event.dataTransfer.effectAllowed = "move"
              event.dataTransfer.setData("text/plain", tab.id)
            }}
            onDragEnd={() => setDragId(null)}
            onDragOver={(event) => {
              if (!isDropTarget) return
              event.preventDefault()
              event.dataTransfer.dropEffect = "move"
            }}
            onDrop={(event) => {
              event.preventDefault()
              const fromId = event.dataTransfer.getData("text/plain")
              if (fromId) onReorder(fromId, tab.id)
              setDragId(null)
            }}
            className={cn(
              "group relative flex h-7 w-40 shrink-0 items-center rounded-2xl text-sm transition-[background-color,color,box-shadow,opacity,transform] duration-150 select-none",
              active
                ? "bg-card text-foreground shadow-sm ring-1 ring-foreground/5"
                : "bg-muted-foreground/10 text-muted-foreground hover:bg-muted-foreground/20 hover:text-foreground",
              dragId === tab.id && "opacity-55",
              isDropTarget && "ring-2 ring-primary/50"
            )}
          >
            <button
              ref={(element) => {
                if (element) tabRefs.current.set(tab.id, element)
                else tabRefs.current.delete(tab.id)
              }}
              role="tab"
              type="button"
              aria-selected={active}
              tabIndex={active ? 0 : -1}
              onClick={() => onActivate(tab.id)}
              onKeyDown={(event) => {
                const index = tabs.findIndex((item) => item.id === tab.id)
                if (event.key === "ArrowRight") {
                  event.preventDefault()
                  focusTab(index + 1)
                } else if (event.key === "ArrowLeft") {
                  event.preventDefault()
                  focusTab(index - 1)
                } else if (event.key === "Home") {
                  event.preventDefault()
                  focusTab(0)
                } else if (event.key === "End") {
                  event.preventDefault()
                  focusTab(tabs.length - 1)
                }
              }}
              className="min-w-0 flex-1 truncate rounded-2xl py-1 pr-7 pl-3 text-left outline-none focus-visible:ring-2 focus-visible:ring-ring/50"
            >
              {tab.title}
            </button>
            <Button
              aria-label={`关闭 ${tab.title}`}
              variant="ghost"
              size="icon-xs"
              onClick={(event) => {
                event.stopPropagation()
                onRemove(tab.id)
              }}
              className={cn(
                "absolute right-1.5 rounded-full text-muted-foreground/60 transition-[opacity,color,background-color] hover:bg-muted hover:text-foreground",
                active
                  ? "opacity-100"
                  : "pointer-events-none opacity-0 group-focus-within:pointer-events-auto group-focus-within:opacity-100 group-hover:pointer-events-auto group-hover:opacity-100"
              )}
            >
              <X />
            </Button>
          </div>
        )
      })}
      <Button
        variant="ghost"
        size="icon"
        aria-label="新建标签页"
        onClick={onAdd}
        className="shrink-0 rounded-full text-muted-foreground"
      >
        <Plus />
      </Button>
    </div>
  )
}
