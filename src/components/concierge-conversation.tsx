import * as React from "react"
import { useVirtualizer } from "@tanstack/react-virtual"
import { Bot, UserRound } from "lucide-react"

import { MarkdownContent } from "@/components/markdown-content"
import { Avatar, AvatarFallback } from "@/components/ui/avatar"
import { Bubble, BubbleContent } from "@/components/ui/bubble"
import { Button } from "@/components/ui/button"
import { Message, MessageAvatar, MessageContent } from "@/components/ui/message"
import {
  MessageScroller,
  MessageScrollerButton,
  MessageScrollerProvider,
  MessageScrollerViewport,
} from "@/components/ui/message-scroller"
import { Spinner } from "@/components/ui/spinner"
import { formatTime } from "@/lib/format"
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
      return Math.min(720, 92 + Math.ceil((message?.content.length ?? 0) / 5))
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
            className="relative mx-auto w-full max-w-3xl"
            style={{ height: virtualizer.getTotalSize() + 48 }}
          >
            {virtualizer.getVirtualItems().map((virtualItem) => {
              const message = messages[virtualItem.index - (hasMore ? 1 : 0)]
              return (
                <div
                  key={virtualItem.key}
                  ref={virtualizer.measureElement}
                  data-index={virtualItem.index}
                  className="absolute top-0 left-0 w-full px-5 pb-6"
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
  return (
    <Message align={assistant ? "start" : "end"}>
      <MessageAvatar>
        <Avatar className="size-8">
          <AvatarFallback>{assistant ? <Bot /> : <UserRound />}</AvatarFallback>
        </Avatar>
      </MessageAvatar>
      <MessageContent>
        <Bubble variant={assistant ? "ghost" : "secondary"}>
          <BubbleContent>
            {assistant ? (
              <MarkdownContent>
                {message.content || "正在思考…"}
              </MarkdownContent>
            ) : (
              <p className="whitespace-pre-wrap">{message.content}</p>
            )}
          </BubbleContent>
        </Bubble>
        <span className="px-1 text-xs text-muted-foreground">
          {assistant ? "Aegis 管家" : "你"} · {formatTime(message.createdAt)}
          {message.streaming ? " · 回复中" : ""}
        </span>
      </MessageContent>
    </Message>
  )
}
