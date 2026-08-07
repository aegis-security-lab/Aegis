import * as React from "react"
import { MessageSquare, Plus, Trash2 } from "lucide-react"

import { Button } from "@/components/ui/button"
import { ScrollArea } from "@/components/ui/scroll-area"
import { Spinner } from "@/components/ui/spinner"
import { cn } from "@/lib/utils"
import type { ConciergeConversation } from "@/types"

function formatSessionTime(value: string) {
  return new Intl.DateTimeFormat("zh-CN", {
    month: "numeric",
    day: "numeric",
    hour: "2-digit",
    minute: "2-digit",
  }).format(new Date(value))
}

export function ConciergeSessionDrawer({
  conversations,
  selectedId,
  loading,
  creating,
  onSelect,
  onDelete,
  onCreate,
}: {
  conversations: ConciergeConversation[]
  selectedId?: string
  loading: boolean
  creating: boolean
  onSelect: (id: string) => void
  onDelete: (conversation: ConciergeConversation) => void
  onCreate: () => void
}) {
  const [open, setOpen] = React.useState(false)
  const closeTimerRef = React.useRef<ReturnType<typeof setTimeout> | undefined>(
    undefined
  )

  const openNow = React.useCallback(() => {
    if (closeTimerRef.current) clearTimeout(closeTimerRef.current)
    setOpen(true)
  }, [])
  const scheduleClose = React.useCallback(() => {
    if (closeTimerRef.current) clearTimeout(closeTimerRef.current)
    closeTimerRef.current = setTimeout(() => setOpen(false), 180)
  }, [])
  React.useEffect(
    () => () => {
      if (closeTimerRef.current) clearTimeout(closeTimerRef.current)
    },
    []
  )

  const select = (id: string) => {
    setOpen(false)
    onSelect(id)
  }

  return (
    <>
      <div
        className="absolute inset-y-0 left-0 z-20 w-2"
        onMouseEnter={openNow}
        aria-hidden="true"
      />
      <aside
        className={cn(
          "absolute inset-y-0 left-0 z-30 flex w-72 min-w-0 flex-col bg-card transition-transform duration-200 ease-out",
          open
            ? "translate-x-0 border-r shadow-xl shadow-foreground/10"
            : "-translate-x-full pointer-events-none"
        )}
        onMouseEnter={openNow}
        onMouseLeave={scheduleClose}
      >
        <ScrollArea className="min-h-0 flex-1">
          <div className="flex flex-col gap-1 p-2">
            {loading && conversations.length === 0 ? (
              <div className="flex justify-center py-10">
                <Spinner />
              </div>
            ) : conversations.length === 0 ? (
              <div className="flex flex-col items-center gap-2 px-4 py-12 text-center">
                <MessageSquare className="size-5 text-muted-foreground" />
                <p className="text-xs text-muted-foreground">
                  还没有会话，新建会话后开始对话。
                </p>
              </div>
            ) : (
              conversations.map((conversation) => (
                <div key={conversation.id} className="group/conversation relative">
                  <Button
                    variant={
                      conversation.id === selectedId ? "secondary" : "ghost"
                    }
                    className="h-auto w-full justify-start py-2.5 pl-3 pr-10 text-left"
                    onClick={() => select(conversation.id)}
                  >
                    <span className="flex min-w-0 flex-1 flex-col gap-0.5">
                      <span className="truncate text-sm font-medium">
                        {conversation.title}
                      </span>
                      <span className="truncate text-xs font-normal text-muted-foreground">
                        {conversation.lastMessage || "开始一段新对话"}
                      </span>
                    </span>
                    <span className="shrink-0 text-[11px] font-normal text-muted-foreground">
                      {formatSessionTime(conversation.updatedAt)}
                    </span>
                  </Button>
                  <Button
                    variant="ghost"
                    size="icon-sm"
                    className="absolute top-1/2 right-2 -translate-y-1/2 opacity-0 focus-visible:opacity-100 group-hover/conversation:opacity-100"
                    aria-label={`删除会话 ${conversation.title}`}
                    onClick={() => onDelete(conversation)}
                  >
                    <Trash2 />
                  </Button>
                </div>
              ))
            )}
          </div>
        </ScrollArea>
        <div className="border-t p-2">
          <Button
            type="button"
            variant="ghost"
            className="w-full justify-start"
            disabled={creating}
            onClick={onCreate}
          >
            {creating ? <Spinner /> : <Plus />}
            新建会话
          </Button>
        </div>
      </aside>
    </>
  )
}
