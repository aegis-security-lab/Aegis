import * as React from "react"
import { useVirtualizer } from "@tanstack/react-virtual"
import { Paperclip } from "lucide-react"
import { MarkdownContent } from "@/components/markdown-content"
import { Bubble, BubbleContent } from "@/components/ui/bubble"
import { Button } from "@/components/ui/button"
import { Message, MessageContent, MessageHeader } from "@/components/ui/message"
import {
  MessageScroller,
  MessageScrollerButton,
  MessageScrollerProvider,
  MessageScrollerViewport,
} from "@/components/ui/message-scroller"
import { Spinner } from "@/components/ui/spinner"
import { formatBytes, formatTime } from "@/lib/format"
import type { Message as ChatMessage } from "@/types"

export function ConciergeConversationView({
  messages,
  hasMore,
  loadingMore,
  onLoadMore,
}: {
  messages: ChatMessage[]
  hasMore: boolean
  loadingMore: boolean
  onLoadMore: () => void
}) {
  const viewportRef = React.useRef<HTMLDivElement>(null)
  const count = messages.length + (hasMore ? 1 : 0)
  // eslint-disable-next-line react-hooks/incompatible-library
  const virtualizer = useVirtualizer({
    count,
    getScrollElement: () => viewportRef.current,
    estimateSize: (index) => {
      if (hasMore && index === 0) return 56
      const message = messages[index - (hasMore ? 1 : 0)]
      return Math.min(720, 64 + Math.ceil((message?.content.length ?? 0) / 5))
    },
    getItemKey: (index) => {
      if (hasMore && index === 0) return "load-more-concierge-messages"
      return messages[index - (hasMore ? 1 : 0)]?.id ?? index
    },
    overscan: 6,
    initialOffset: 1_000_000_000,
  })

  return (
    <MessageScrollerProvider
      defaultScrollPosition="last-anchor"
      autoScroll={messages.some((message) => message.streaming)}
    >
      <MessageScroller className="h-full min-h-0">
        <MessageScrollerViewport ref={viewportRef} className="h-full">
          <div
            className="relative mx-auto w-full max-w-4xl"
            style={{ height: virtualizer.getTotalSize() + 48 }}
          >
            {virtualizer.getVirtualItems().map((virtualItem) => {
              const message = messages[virtualItem.index - (hasMore ? 1 : 0)]
              return (
                <div
                  key={virtualItem.key}
                  ref={virtualizer.measureElement}
                  data-index={virtualItem.index}
                  className="absolute top-0 left-0 w-full px-5 pb-3 sm:px-8"
                  style={{
                    transform: `translateY(${virtualItem.start + 24}px)`,
                  }}
                >
                  {message ? (
                    <ConversationMessage message={message} />
                  ) : (
                    <div className="flex justify-center">
                      <Button
                        type="button"
                        variant="outline"
                        size="sm"
                        disabled={loadingMore}
                        onClick={onLoadMore}
                      >
                        {loadingMore ? (
                          <Spinner data-icon="inline-start" />
                        ) : null}
                        加载更早消息
                      </Button>
                    </div>
                  )}
                </div>
              )
            })}
          </div>
        </MessageScrollerViewport>
        <MessageScrollerButton />
      </MessageScroller>
    </MessageScrollerProvider>
  )
}

function ConversationMessage({ message }: { message: ChatMessage }) {
  const assistant = message.role === "assistant"
  if (assistant) {
    return (
      <Message>
        <MessageContent>
          <MessageHeader className="px-0">
            管家 · {formatTime(message.createdAt)}
          </MessageHeader>
          <MarkdownContent className="text-[13px] !leading-5 [&>*+*]:!mt-2">
            {message.content || "正在思考…"}
          </MarkdownContent>
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
  return (
    <Message align="end">
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
