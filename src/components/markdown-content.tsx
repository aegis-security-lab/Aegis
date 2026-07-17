import Markdown from "react-markdown"
import remarkGfm from "remark-gfm"

import { cn } from "@/lib/utils"

const allowedProtocol = /^(https?:|mailto:|agent:)/i

export function MarkdownContent({
  children,
  className,
}: {
  children: string
  className?: string
}) {
  return (
    <div
      className={cn(
        "min-w-0 overflow-x-auto break-words leading-7",
        "[&>*+*]:mt-3 [&_a]:font-medium [&_a]:underline [&_a]:underline-offset-4",
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
          url.startsWith("/") || url.startsWith("#") || allowedProtocol.test(url)
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
        {children}
      </Markdown>
    </div>
  )
}
