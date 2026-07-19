import * as React from "react"
import Markdown from "react-markdown"
import remarkGfm from "remark-gfm"

import { Button } from "@/components/ui/button"
import { cn } from "@/lib/utils"

const allowedProtocol = /^(https?:|mailto:|agent:)/i
const markdownPreviewLimit = 20_000

export function MarkdownContent({
  children,
  className,
}: {
  children: string
  className?: string
}) {
  const [expanded, setExpanded] = React.useState(false)
  const truncated = children.length > markdownPreviewLimit && !expanded
  const content = truncated
    ? `${children.slice(0, markdownPreviewLimit)}\n\n…`
    : children
  return (
    <div
      className={cn(
        "min-w-0 overflow-x-auto leading-7 break-words",
        "[&_a]:font-medium [&_a]:underline [&_a]:underline-offset-4 [&>*+*]:mt-3",
        "[&_blockquote]:border-l-2 [&_blockquote]:pl-4 [&_blockquote]:text-muted-foreground",
        "[&_code]:rounded [&_code]:bg-muted [&_code]:px-1 [&_code]:py-0.5 [&_code]:font-mono [&_code]:text-foreground",
        "[&_h1]:text-lg [&_h1]:font-semibold [&_h2]:text-base [&_h2]:font-semibold [&_h3]:font-medium",
        "[&_ol]:list-decimal [&_ol]:pl-5 [&_p]:whitespace-pre-wrap [&_ul]:list-disc [&_ul]:pl-5",
        "[&_pre]:max-w-full [&_pre]:overflow-x-auto [&_pre]:rounded-lg [&_pre]:border [&_pre]:bg-muted [&_pre]:p-4",
        "[&_pre_code]:bg-transparent [&_pre_code]:p-0 [&_pre_code]:whitespace-pre",
        "[&_table]:w-full [&_table]:border-collapse [&_td]:border [&_td]:p-2 [&_th]:border [&_th]:bg-muted [&_th]:p-2 [&_th]:text-left",
        className
      )}
    >
      <Markdown
        remarkPlugins={[remarkGfm]}
        urlTransform={(url) =>
          url.startsWith("/") ||
          url.startsWith("#") ||
          allowedProtocol.test(url)
            ? url
            : ""
        }
        components={{
          a: ({ href, children: linkChildren }) => {
            if (href?.startsWith("agent://")) {
              return (
                <span className="rounded bg-muted px-1.5 py-0.5 font-medium text-foreground">
                  {linkChildren}
                </span>
              )
            }
            const external = /^https?:/i.test(href ?? "")
            return (
              <a
                href={href}
                target={external ? "_blank" : undefined}
                rel={external ? "noreferrer" : undefined}
              >
                {linkChildren}
              </a>
            )
          },
        }}
      >
        {content}
      </Markdown>
      {truncated ? (
        <div className="mt-3 flex items-center justify-between gap-3 rounded-lg border bg-muted/30 p-3">
          <p className="text-xs text-muted-foreground">
            内容较长，已先渲染前 {markdownPreviewLimit.toLocaleString()}{" "}
            个字符。
          </p>
          <Button
            type="button"
            variant="outline"
            size="sm"
            onClick={() => setExpanded(true)}
          >
            渲染完整内容
          </Button>
        </div>
      ) : null}
    </div>
  )
}
