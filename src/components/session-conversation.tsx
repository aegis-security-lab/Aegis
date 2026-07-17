import { Bot, MessageSquareText, UserRound } from "lucide-react"

import { MarkdownContent } from "@/components/markdown-content"
import { Avatar, AvatarFallback } from "@/components/ui/avatar"
import { Bubble, BubbleContent } from "@/components/ui/bubble"
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
import { formatTime } from "@/lib/format"
import type { Message as SessionMessage } from "@/types"

export function SessionConversation({
  messages,
  agentName,
  issueIdentifier,
  initialPrompt,
}: {
  messages: SessionMessage[]
  agentName: string
  issueIdentifier: string
  initialPrompt: string
}) {
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

  const initialPromptMessageId = initialPrompt
    ? messages.find(
        (message) =>
          message.role === "user" && message.content === initialPrompt
      )?.id
    : undefined
  const isStreaming = messages.some((message) => message.streaming)

  return (
    <MessageScrollerProvider
      defaultScrollPosition="last-anchor"
      autoScroll={isStreaming}
    >
      <MessageScroller>
        <MessageScrollerViewport aria-label={`${agentName} Session 对话记录`}>
          <MessageScrollerContent className="mx-auto w-full max-w-4xl px-4 py-6 sm:px-8">
            <MessageScrollerItem messageId="session-start">
              <Marker variant="separator">
                <MarkerIcon>
                  <MessageSquareText />
                </MarkerIcon>
                <MarkerContent>{issueIdentifier} · Session 开始</MarkerContent>
              </Marker>
            </MessageScrollerItem>
            {messages.map((message) => (
              <MessageScrollerItem
                key={message.id}
                messageId={message.id}
                scrollAnchor={message.role === "user"}
              >
                {message.role === "system" ? (
                  <Marker variant="border">
                    <MarkerIcon>
                      <Bot />
                    </MarkerIcon>
                    <MarkerContent>{message.content}</MarkerContent>
                  </Marker>
                ) : (
                  <ConversationMessage
                    message={message}
                    agentName={agentName}
                    isInitialPrompt={message.id === initialPromptMessageId}
                  />
                )}
              </MessageScrollerItem>
            ))}
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
