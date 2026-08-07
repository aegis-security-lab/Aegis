import * as React from "react"
import { Paperclip, Send } from "lucide-react"

import { InputAttachmentList } from "@/components/input-attachments"
import type { useInputAttachments } from "@/hooks/use-input-attachments"
import {
  InputGroup,
  InputGroupAddon,
  InputGroupButton,
  InputGroupTextarea,
} from "@/components/ui/input-group"
import { Spinner } from "@/components/ui/spinner"
import { isSendMessageKey } from "@/lib/keyboard"
import { cn } from "@/lib/utils"

type Attachments = ReturnType<typeof useInputAttachments>

export function ChatComposer({
  value,
  onValueChange,
  onSend,
  sending,
  placeholder = "输入消息…",
  ariaLabel = "发送消息",
  sendLabel,
  hint = "Enter 发送 · Shift+Enter 换行",
  showSend = true,
  attachments,
  extra,
  className,
}: {
  value: string
  onValueChange: (value: string) => void
  onSend: () => void
  sending: boolean
  placeholder?: string
  ariaLabel?: string
  sendLabel?: string
  hint?: string
  showSend?: boolean
  attachments?: Attachments
  extra?: React.ReactNode
  className?: string
}) {
  const fileInputRef = React.useRef<HTMLInputElement>(null)
  const canSend =
    Boolean(value.trim()) ||
    Boolean(attachments && attachments.attachmentIds.length > 0)
  const disabled =
    sending ||
    Boolean(attachments?.uploading) ||
    Boolean(attachments?.hasErrors) ||
    !canSend

  return (
    <form
      className={className}
      onSubmit={(event) => {
        event.preventDefault()
        if (canSend && !sending) onSend()
      }}
    >
      {attachments ? (
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
      ) : null}
      {attachments?.items.length ? (
        <InputAttachmentList
          items={attachments.items}
          onRemove={(clientId) => void attachments.remove(clientId)}
          className="mb-2 flex-wrap overflow-visible"
        />
      ) : null}
      <InputGroup className="min-h-24 items-stretch rounded-lg">
        <InputGroupTextarea
          value={value}
          maxLength={50000}
          aria-label={ariaLabel}
          placeholder={placeholder}
          onChange={(event) => onValueChange(event.target.value)}
          onKeyDown={(event) => {
            if (showSend && isSendMessageKey(event)) {
              event.preventDefault()
              if (canSend && !sending) onSend()
            }
          }}
          className="min-h-14 resize-none text-[13px]"
        />
        <InputGroupAddon align="block-end" className="justify-between">
          <div className="flex min-w-0 items-center gap-1.5">
            {attachments ? (
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
            ) : null}
            {extra ? extra : null}
          </div>
          {hint && showSend ? (
            <span
              className={cn(
                "px-1 text-xs font-normal text-muted-foreground",
                attachments && "ml-auto"
              )}
            >
              {hint}
            </span>
          ) : null}
          {showSend ? (
            <InputGroupButton
              type="submit"
              variant="default"
              size="icon-sm"
              aria-label={ariaLabel}
              disabled={disabled}
              className={cn(sendLabel && "px-2 text-xs")}
            >
              {sending ? <Spinner /> : <Send />}
              {sendLabel}
            </InputGroupButton>
          ) : null}
        </InputGroupAddon>
      </InputGroup>
    </form>
  )
}
