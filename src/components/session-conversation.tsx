import * as React from "react"
import { Bot, MessageSquareText, UserRound } from "lucide-react"
import { useVirtualizer } from "@tanstack/react-virtual"

import { MarkdownContent } from "@/components/markdown-content"
import { Avatar, AvatarFallback } from "@/components/ui/avatar"
import { Bubble, BubbleContent } from "@/components/ui/bubble"
import { Button } from "@/components/ui/button"
import {
  Empty,
  EmptyDescription,
  EmptyHeader,
  EmptyMedia,
  EmptyTitle,
} from "@/components/ui/empty"
import { Marker, MarkerContent, MarkerIcon } from "@/components/ui/marker"
import {
  Message,
  MessageAvatar,
  MessageContent,
  MessageFooter,
  MessageHeader,
} from "@/components/ui/message"
import {
  MessageScroller,
  MessageScrollerButton,
  MessageScrollerContent,
  MessageScrollerItem,
  MessageScrollerProvider,
  MessageScrollerViewport,
} from "@/components/ui/message-scroller"
import { Spinner } from "@/components/ui/spinner"
import { formatTime } from "@/lib/format"
import type { Message as SessionMessage } from "@/types"

export function SessionConversation({
  messages,
  agentName,
  issueIdentifier,
  initialPrompt,
  hasMore = false,
  loadingMore = false,
  onLoadMore,
}: {
  messages: SessionMessage[]
  agentName: string
  issueIdentifier: string
  initialPrompt: string
  hasMore?: boolean
  loadingMore?: boolean
  onLoadMore?: () => void
}) {
  const initialPromptMessageId = initialPrompt
    ? messages.find(
        (message) =>
          message.role === "user" && message.content === initialPrompt
      )?.id
    : undefined
  const isStreaming = messages.some((message) => message.streaming)
  const viewportRef = React.useRef<HTMLDivElement>(null)
  const count = messages.length + 1 + (hasMore ? 1 : 0)
  // eslint-disable-next-line react-hooks/incompatible-library
  const virtualizer = useVirtualizer({
    count,
    getScrollElement: () => viewportRef.current,
    estimateSize: (index) => {
      if (hasMore && index === 0) return 52
      const messageIndex = index - (hasMore ? 1 : 0) - 1
      if (messageIndex < 0) return 48
      const contentLength = messages[messageIndex]?.content.length ?? 0
      return Math.min(900, 120 + Math.ceil(contentLength / 5))
    },
    getItemKey: (index) => {
      if (hasMore && index === 0) return "load-more-messages"
      const messageIndex = index - (hasMore ? 1 : 0) - 1
      return messageIndex < 0
        ? "session-start"
        : (messages[messageIndex]?.id ?? index)
    },
    overscan: 4,
    initialOffset: 1_000_000_000,
  })

  if (messages.length === 0) {
    return (
      <Empty className="h-full border-0">
        <EmptyHeader>
          <EmptyMedia variant="icon">
            <MessageSquareText />
          </EmptyMedia>
          <EmptyTitle>这个 Session 还没有消息</EmptyTitle>
          <EmptyDescription>
            Pi 产生回复或收到用户补充要求后，对话会显示在这里。
          </EmptyDescription>
        </EmptyHeader>
      </Empty>
    )
  }

  return (
    <MessageScrollerProvider
      defaultScrollPosition="last-anchor"
      autoScroll={isStreaming}
    >
      <MessageScroller>
        <MessageScrollerViewport
          ref={viewportRef}
          aria-label={`${agentName} Session 对话记录`}
        >
          <MessageScrollerContent
            className="relative mx-auto block w-full max-w-4xl px-4 py-6 sm:px-8"
            style={{ height: virtualizer.getTotalSize() + 48 }}
          >
            {virtualizer.getVirtualItems().map((virtualItem) => {
              const startOffset = hasMore ? 1 : 0
              const messageIndex = virtualItem.index - startOffset - 1
              const message = messages[messageIndex]
              return (
                <MessageScrollerItem
                  key={virtualItem.key}
                  ref={virtualizer.measureElement}
                  data-index={virtualItem.index}
                  messageId={message?.id ?? String(virtualItem.key)}
                  scrollAnchor={message?.role === "user"}
                  className="absolute top-0 left-4 w-[calc(100%-2rem)] pb-6 sm:left-8 sm:w-[calc(100%-4rem)]"
                  style={{
                    transform: `translateY(${virtualItem.start + 24}px)`,
                  }}
                >
                  {hasMore && virtualItem.index === 0 ? (
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
                  ) : messageIndex < 0 ? (
                    <Marker variant="separator">
                      <MarkerIcon>
                        <MessageSquareText />
                      </MarkerIcon>
                      <MarkerContent>
                        {issueIdentifier} · Session 开始
                      </MarkerContent>
                    </Marker>
                  ) : message?.role === "system" ? (
                    <Marker variant="border">
                      <MarkerIcon>
                        <Bot />
                      </MarkerIcon>
                      <MarkerContent>{message.content}</MarkerContent>
                    </Marker>
                  ) : message ? (
                    <ConversationMessage
                      message={message}
                      agentName={agentName}
                      isInitialPrompt={message.id === initialPromptMessageId}
                    />
                  ) : null}
                </MessageScrollerItem>
              )
            })}
          </MessageScrollerContent>
        </MessageScrollerViewport>
        <MessageScrollerButton />
      </MessageScroller>
    </MessageScrollerProvider>
  )
}

function ConversationMessage({
  message,
  agentName,
  isInitialPrompt,
}: {
  message: SessionMessage
  agentName: string
  isInitialPrompt: boolean
}) {
  const isUser = message.role === "user"
  return (
    <Message align={isUser ? "end" : "start"}>
      <MessageAvatar>
        <Avatar>
          <AvatarFallback>{isUser ? <UserRound /> : <Bot />}</AvatarFallback>
        </Avatar>
      </MessageAvatar>
      <MessageContent>
        <MessageHeader>
          {isInitialPrompt ? "启动提示词" : isUser ? "你" : agentName}
        </MessageHeader>
        <Bubble
          align={isUser ? "end" : "start"}
          variant={isUser ? "default" : "ghost"}
        >
          <BubbleContent>
            <MarkdownContent>{message.content}</MarkdownContent>
          </BubbleContent>
        </Bubble>
        <MessageFooter>
          {formatTime(message.createdAt)}
          {message.streaming ? (
            <span className="ml-2 shimmer">正在生成…</span>
          ) : null}
        </MessageFooter>
      </MessageContent>
    </Message>
  )
}
