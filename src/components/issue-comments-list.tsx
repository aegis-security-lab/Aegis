import * as React from "react"
import { useVirtualizer } from "@tanstack/react-virtual"
import { Copy, Download, FileText } from "lucide-react"

import { MarkdownContent } from "@/components/markdown-content"
import {
  Attachment,
  AttachmentActions,
  AttachmentContent,
  AttachmentDescription,
  AttachmentGroup,
  AttachmentMedia,
  AttachmentTitle,
} from "@/components/ui/attachment"
import { Button, buttonVariants } from "@/components/ui/button"
import { Empty, EmptyHeader, EmptyTitle } from "@/components/ui/empty"
import { ScrollArea } from "@/components/ui/scroll-area"
import { Spinner } from "@/components/ui/spinner"
import { formatBytes, formatTime } from "@/lib/format"
import type { IssueComment } from "@/types"

export function IssueCommentsList({
  comments,
  hasMore,
  loadingMore,
  onLoadMore,
  onCopy,
  embedded = false,
}: {
  comments: IssueComment[]
  hasMore: boolean
  loadingMore: boolean
  onLoadMore: () => void
  onCopy: (body: string) => void
  embedded?: boolean
}) {
  const viewportRef = React.useRef<HTMLDivElement>(null)
  const count = comments.length + (hasMore ? 1 : 0)
  // eslint-disable-next-line react-hooks/incompatible-library
  const virtualizer = useVirtualizer({
    count,
    getScrollElement: () => viewportRef.current,
    estimateSize: (index) => {
      if (hasMore && index === 0) return 52
      const comment = comments[index - (hasMore ? 1 : 0)]
      return Math.min(760, 116 + Math.ceil((comment?.body.length ?? 0) / 5))
    },
    getItemKey: (index) => {
      if (hasMore && index === 0) return "load-more-comments"
      return comments[index - (hasMore ? 1 : 0)]?.id ?? index
    },
    overscan: 5,
    initialOffset: 1_000_000_000,
  })

  if (comments.length === 0 && !hasMore) {
    return (
      <Empty
        className={
          embedded
            ? "rounded-xl border bg-card py-8"
            : "h-full border-0"
        }
      >
        <EmptyHeader>
          <EmptyTitle>还没有评论</EmptyTitle>
        </EmptyHeader>
      </Empty>
    )
  }

  if (embedded) {
    return (
      <div className="overflow-hidden rounded-xl border bg-card">
        {hasMore ? (
          <div className="flex justify-center border-b px-4 py-2.5">
            <Button
              type="button"
              variant="outline"
              size="sm"
              disabled={loadingMore}
              onClick={onLoadMore}
            >
              {loadingMore ? <Spinner data-icon="inline-start" /> : null}
              加载更早评论
            </Button>
          </div>
        ) : null}
        <div className="divide-y divide-border/70">
          {comments.map((comment) => (
            <CommentItem
              key={comment.id}
              comment={comment}
              onCopy={onCopy}
              embedded
            />
          ))}
        </div>
      </div>
    )
  }

  return (
    <ScrollArea
      viewportRef={viewportRef}
      className="size-full min-h-0"
    >
      <div
        className="relative"
        style={{ height: virtualizer.getTotalSize() + 24 }}
      >
        {virtualizer.getVirtualItems().map((virtualItem) => {
          const comment = comments[virtualItem.index - (hasMore ? 1 : 0)]
          return (
            <div
              key={virtualItem.key}
              ref={virtualizer.measureElement}
              data-index={virtualItem.index}
              className="absolute top-0 left-0 w-full px-3 pb-3"
              style={{ transform: `translateY(${virtualItem.start + 12}px)` }}
            >
              {comment ? (
                <CommentItem comment={comment} onCopy={onCopy} />
              ) : (
                <div className="flex justify-center py-2">
                  <Button
                    type="button"
                    variant="outline"
                    size="sm"
                    disabled={loadingMore}
                    onClick={onLoadMore}
                  >
                    {loadingMore ? <Spinner data-icon="inline-start" /> : null}
                    加载更早评论
                  </Button>
                </div>
              )}
            </div>
          )
        })}
      </div>
    </ScrollArea>
  )
}

function CommentItem({
  comment,
  onCopy,
  embedded = false,
}: {
  comment: IssueComment
  onCopy: (body: string) => void
  embedded?: boolean
}) {
  return (
    <article className={embedded ? "px-4 py-3.5" : "px-1 pb-4"}>
      <header className="flex items-center justify-between gap-3">
        <span className="text-xs font-medium">{comment.authorId}</span>
        <div className="flex items-center gap-1">
          <time className="text-xs text-muted-foreground">
            {formatTime(comment.createdAt)}
          </time>
          <Button
            type="button"
            variant="ghost"
            size="icon-sm"
            title="复制评论"
            aria-label={`复制 ${comment.authorId} 的评论`}
            onClick={() => onCopy(comment.body)}
          >
            <Copy data-icon="inline-start" />
          </Button>
        </div>
      </header>
      <MarkdownContent className="mt-2 text-sm">{comment.body}</MarkdownContent>
      {comment.attachments.length > 0 ? (
        <AttachmentGroup className="mt-3">
          {comment.attachments.map((attachment) => (
            <Attachment key={attachment.id} size="sm" className="w-72">
              <AttachmentMedia>
                <FileText />
              </AttachmentMedia>
              <AttachmentContent>
                <AttachmentTitle title={attachment.name}>
                  {attachment.name}
                </AttachmentTitle>
                <AttachmentDescription
                  title={attachment.description || attachment.sourcePath}
                >
                  {attachment.description || attachment.sourcePath}
                  {" · "}
                  {formatBytes(attachment.size)}
                </AttachmentDescription>
              </AttachmentContent>
              <AttachmentActions>
                <a
                  className={buttonVariants({
                    variant: "ghost",
                    size: "icon-xs",
                  })}
                  aria-label={`下载 ${attachment.name}`}
                  title="下载附件"
                  href={`/api/attachments/${encodeURIComponent(attachment.id)}`}
                  download={attachment.name}
                >
                  <Download data-icon="inline-start" />
                </a>
              </AttachmentActions>
            </Attachment>
          ))}
        </AttachmentGroup>
      ) : null}
    </article>
  )
}
