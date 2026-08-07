import { ExternalLink } from "lucide-react"
import { Link } from "react-router-dom"

import { ChatComposer } from "@/components/chat-composer"
import { ConciergeConversationView } from "@/components/concierge-conversation"
import type { useInputAttachments } from "@/hooks/use-input-attachments"
import { Button } from "@/components/ui/button"
import type { ConciergeConversationDetail } from "@/types"

type Attachments = ReturnType<typeof useInputAttachments>

export function ConciergeChatPanel({
  detail,
  draft,
  onDraftChange,
  sending,
  loadingMore,
  onLoadMore,
  onSend,
  attachments,
}: {
  detail: ConciergeConversationDetail | null
  draft: string
  onDraftChange: (value: string) => void
  sending: boolean
  loadingMore: boolean
  onLoadMore: () => void
  onSend: () => void
  attachments: Attachments
}) {
  return (
    <section className="relative flex h-full min-h-0 min-w-0 flex-1 flex-col">
      {detail?.conversation.createdTaskId ? (
        <Button
          variant="outline"
          size="sm"
          className="absolute top-3 right-3 z-10"
          render={
            <Link to={`/tasks/${detail.conversation.createdTaskId}`} />
          }
          nativeButton={false}
        >
          查看任务
          <ExternalLink data-icon="inline-end" />
        </Button>
      ) : null}
      <div className="min-h-0 flex-1">
        {detail?.messages.length ? (
          <ConciergeConversationView
            messages={detail.messages}
            hasMore={detail.messagesPage.hasMore}
            loadingMore={loadingMore}
            onLoadMore={onLoadMore}
          />
        ) : (
          <div className="flex h-full min-h-0 items-center justify-center p-6">
            <p className="max-w-sm text-center text-sm text-muted-foreground">
              从左侧会话列表继续，或直接发送消息开始新对话。
            </p>
          </div>
        )}
      </div>
      <div className="shrink-0 px-3 pt-1 pb-3 sm:px-4 sm:pb-4">
        <ChatComposer
          className="mx-auto max-w-4xl"
          value={draft}
          onValueChange={onDraftChange}
          onSend={onSend}
          sending={sending}
          placeholder="告诉管家你的需求…"
          ariaLabel="给管家发送消息"
          attachments={attachments}
        />
      </div>
    </section>
  )
}
