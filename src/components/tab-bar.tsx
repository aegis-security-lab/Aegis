import * as React from "react"
import { Plus, X } from "lucide-react"

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
            role="tab"
            aria-selected={active}
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
            onClick={() => onActivate(tab.id)}
            className={cn(
              "group flex h-7 w-40 shrink-0 cursor-pointer items-center gap-1.5 rounded-2xl px-3 text-sm select-none",
              active
                ? "bg-card text-foreground shadow-sm ring-1 ring-foreground/5"
                : "bg-muted-foreground/10 text-muted-foreground hover:bg-muted-foreground/20 hover:text-foreground",
              isDropTarget && "ring-2 ring-primary/50"
            )}
          >
            <span className="min-w-0 flex-1 truncate">{tab.title}</span>
            <button
              type="button"
              aria-label={`关闭 ${tab.title}`}
              onClick={(event) => {
                event.stopPropagation()
                onRemove(tab.id)
              }}
              className={cn(
                "ml-auto flex size-4 shrink-0 items-center justify-center rounded-full text-muted-foreground/60 transition-colors hover:bg-muted hover:text-foreground",
                active ? "flex" : "hidden group-hover:flex"
              )}
            >
              <X className="size-3" />
            </button>
          </div>
        )
      })}
      <button
        type="button"
        aria-label="新建标签页"
        onClick={onAdd}
        className="flex size-7 shrink-0 cursor-pointer items-center justify-center rounded-2xl text-muted-foreground transition-colors hover:bg-muted hover:text-foreground"
      >
        <Plus className="size-4" />
      </button>
    </div>
  )
}
