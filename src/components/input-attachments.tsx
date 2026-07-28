import { FileArchive, X } from "lucide-react"

import {
  Attachment,
  AttachmentAction,
  AttachmentActions,
  AttachmentContent,
  AttachmentDescription,
  AttachmentGroup,
  AttachmentMedia,
  AttachmentTitle,
} from "@/components/ui/attachment"
import { Spinner } from "@/components/ui/spinner"
import type { PendingInputAttachment } from "@/hooks/use-input-attachments"

export function InputAttachmentList({
  items,
  onRemove,
  className,
}: {
  items: PendingInputAttachment[]
  onRemove: (clientId: string) => void
  className?: string
}) {
  if (items.length === 0) return null
  return (
    <AttachmentGroup className={className}>
      {items.map((item) => (
        <Attachment
          key={item.clientId}
          state={item.state}
          size="sm"
          className="max-w-80"
        >
          <AttachmentMedia>
            {item.state === "uploading" || item.state === "processing" ? (
              <Spinner />
            ) : (
              <FileArchive />
            )}
          </AttachmentMedia>
          <AttachmentContent>
            <AttachmentTitle title={item.file.name}>
              {item.file.name}
            </AttachmentTitle>
            <AttachmentDescription>
              {item.error ??
                (item.state === "uploading"
                  ? `上传中 ${item.progress}% · ${formatBytes(item.file.size)}`
                  : `${formatBytes(item.file.size)} · 已上传到服务端`)}
            </AttachmentDescription>
          </AttachmentContent>
          <AttachmentActions>
            <AttachmentAction
              aria-label={`移除 ${item.file.name}`}
              disabled={
                item.state === "uploading" || item.state === "processing"
              }
              onClick={() => onRemove(item.clientId)}
            >
              <X />
            </AttachmentAction>
          </AttachmentActions>
        </Attachment>
      ))}
    </AttachmentGroup>
  )
}

function formatBytes(size: number) {
  if (size < 1024) return `${size} B`
  const units = ["KiB", "MiB", "GiB", "TiB"]
  let value = size / 1024
  let index = 0
  while (value >= 1024 && index < units.length - 1) {
    value /= 1024
    index += 1
  }
  return `${value >= 10 ? value.toFixed(1) : value.toFixed(2)} ${units[index]}`
}
