import { Paperclip } from "lucide-react"

import { MarkdownContent } from "@/components/markdown-content"
import { Bubble, BubbleContent } from "@/components/ui/bubble"
import { Message, MessageContent, MessageHeader } from "@/components/ui/message"
import { formatBytes } from "@/lib/format"
import { cn } from "@/lib/utils"
import type { Message as ChatMessage } from "@/types"

export function ChatMessage({
  message,
  label = "Agent",
  time,
  onSelect,
  compact = false,
  className,
}: {
  message: ChatMessage
  label?: string
  time?: string
  onSelect?: () => void
  compact?: boolean
  className?: string
}) {
  if (message.role === "user") {
    return (
      <Message align="end" className={className}>
        <MessageContent>
          <Bubble variant="secondary" align="end">
            <BubbleContent className="text-[13px] leading-5">
              <MarkdownContent className="!leading-5 [&>*+*]:!mt-2">
                {message.content}
              </MarkdownContent>
              {message.attachments?.length ? (
                <div className="mt-2 flex max-w-md flex-col gap-1.5 border-t border-border/60 pt-2">
                  {message.attachments.map((attachment) => (
                    <div
                      key={attachment.id}
                      className="flex min-w-0 items-center gap-2 text-xs text-muted-foreground"
                    >
                      <Paperclip className="size-3.5 shrink-0" />
                      <span className="min-w-0 flex-1 truncate text-foreground">
                        {attachment.name}
                      </span>
                      <span className="shrink-0 tabular-nums">
                        {formatBytes(attachment.size)}
                      </span>
                    </div>
                  ))}
                </div>
              ) : null}
            </BubbleContent>
          </Bubble>
        </MessageContent>
      </Message>
    )
  }

  const content = (
    <MarkdownContent
      className={cn(
        "text-[13px] !leading-5 [&>*+*]:!mt-2",
        compact && "line-clamp-5 text-xs !leading-5 [&>*+*]:!mt-1.5"
      )}
    >
      {message.content || "正在思考…"}
    </MarkdownContent>
  )
  return (
    <Message className={className}>
      <MessageContent>
        {label || time ? (
          <MessageHeader className="px-0">
            {label}
            {time ? (
              <span className="font-normal text-muted-foreground/80">
                {" "}
                · {time}
              </span>
            ) : null}
          </MessageHeader>
        ) : null}
        {onSelect ? (
          <button
            type="button"
            onClick={onSelect}
            className="w-full min-w-0 rounded-lg text-left outline-none hover:bg-muted/40 focus-visible:ring-2 focus-visible:ring-ring/50"
          >
            {content}
          </button>
        ) : (
          content
        )}
        {message.streaming ? (
          <span className="flex items-center gap-1.5 text-xs text-muted-foreground">
            <span className="inline-block size-1.5 animate-pulse rounded-full bg-current" />
            正在生成…
          </span>
        ) : null}
      </MessageContent>
    </Message>
  )
}
