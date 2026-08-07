import * as React from "react"
import { ExternalLink, Paperclip, Send } from "lucide-react"
import { Link } from "react-router-dom"

import { ConciergeConversationView } from "@/components/concierge-conversation"
import { InputAttachmentList } from "@/components/input-attachments"
import type { useInputAttachments } from "@/hooks/use-input-attachments"
import { Button } from "@/components/ui/button"
import {
  InputGroup,
  InputGroupAddon,
  InputGroupButton,
  InputGroupTextarea,
} from "@/components/ui/input-group"
import { Spinner } from "@/components/ui/spinner"
import { isSendMessageKey } from "@/lib/keyboard"
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
  const fileInputRef = React.useRef<HTMLInputElement>(null)

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
        <form
          className="mx-auto max-w-4xl"
          onSubmit={(event) => {
            event.preventDefault()
            onSend()
          }}
        >
          <input
            ref={fileInputRef}
            type="file"
            multiple
            className="hidden"
            onChange={(event) => {
              if (event.target.files) attachments.addFiles(event.target.files)
              event.target.value = ""
            }}
          />
          <InputAttachmentList
            items={attachments.items}
            onRemove={(clientId) => void attachments.remove(clientId)}
            className="mb-2 flex-wrap overflow-visible"
          />
          <InputGroup className="min-h-24 items-stretch rounded-lg">
            <InputGroupTextarea
              value={draft}
              maxLength={50000}
              aria-label="给管家发送消息"
              placeholder="告诉管家你的需求…"
              onChange={(event) => onDraftChange(event.target.value)}
              onKeyDown={(event) => {
                if (isSendMessageKey(event)) {
                  event.preventDefault()
                  onSend()
                }
              }}
              className="min-h-14 resize-none text-[13px]"
            />
            <InputGroupAddon align="block-end" className="justify-between">
              <InputGroupButton
                type="button"
                variant="ghost"
                size="icon-sm"
                aria-label="上传附件"
                title="上传附件"
                disabled={sending || attachments.uploading}
                onClick={() => fileInputRef.current?.click()}
              >
                <Paperclip />
              </InputGroupButton>
              <span className="ml-auto px-1 text-xs font-normal text-muted-foreground">
                Enter 发送 · Shift+Enter 换行
              </span>
              <InputGroupButton
                type="submit"
                variant="default"
                size="icon-sm"
                aria-label="发送消息"
                disabled={
                  sending ||
                  attachments.uploading ||
                  attachments.hasErrors ||
                  (!draft.trim() && attachments.attachmentIds.length === 0)
                }
              >
                {sending ? <Spinner /> : <Send />}
              </InputGroupButton>
            </InputGroupAddon>
          </InputGroup>
        </form>
      </div>
    </section>
  )
}
