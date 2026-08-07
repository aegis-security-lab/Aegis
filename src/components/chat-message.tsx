import { Paperclip } from "lucide-react"

import { MarkdownContent } from "@/components/markdown-content"
import { Bubble, BubbleContent } from "@/components/ui/bubble"
import { Message, MessageContent, MessageHeader } from "@/components/ui/message"
import { Spinner } from "@/components/ui/spinner"
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

  const streaming = message.streaming
  const content = message.content.trim()
  const body = content ? (
    <MarkdownContent
      className={cn(
        "text-[13px] !leading-5 [&>*+*]:!mt-2",
        compact && "line-clamp-5 text-xs !leading-5 [&>*+*]:!mt-1.5"
      )}
    >
      {content}
    </MarkdownContent>
  ) : (
    <span className="flex items-center gap-2 text-muted-foreground">
      <Spinner className="size-3.5" />
      正在思考…
    </span>
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
        <Bubble variant="ghost">
          <BubbleContent
            className={cn(
              "text-[13px] !leading-5 [&>*+*]:!mt-2",
              compact && "text-xs !leading-5 [&>*+*]:!mt-1.5"
            )}
          >
            {onSelect ? (
              <button
                type="button"
                onClick={onSelect}
                className="w-full min-w-0 rounded-lg text-left outline-none hover:bg-muted/40 focus-visible:ring-2 focus-visible:ring-ring/50"
              >
                {body}
              </button>
            ) : (
              body
            )}
            {streaming && content ? (
              <span className="ml-0.5 inline-block h-4 w-px animate-pulse bg-current align-middle" />
            ) : null}
          </BubbleContent>
        </Bubble>
      </MessageContent>
    </Message>
  )
}
