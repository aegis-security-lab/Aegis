import ReactMarkdown from 'react-markdown';
import remarkGfm from 'remark-gfm';
import { cn } from '@/lib/utils';

export function MarkdownContent({ children, compact = false, className }: { children: string; compact?: boolean; className?: string }) {
  return <div className={cn('min-w-0 wrap-break-word', compact ? 'text-xs leading-5' : 'text-sm leading-6', className)}>
    <ReactMarkdown
      remarkPlugins={[remarkGfm]}
      components={{
        h1: ({ children: value }) => <h1 className="mb-3 mt-5 text-xl font-semibold tracking-tight first:mt-0">{value}</h1>,
        h2: ({ children: value }) => <h2 className="mb-2 mt-4 text-lg font-semibold tracking-tight first:mt-0">{value}</h2>,
        h3: ({ children: value }) => <h3 className="mb-2 mt-3 font-semibold first:mt-0">{value}</h3>,
        p: ({ children: value }) => <p className="mb-3 last:mb-0">{value}</p>,
        ul: ({ children: value }) => <ul className="mb-3 flex list-disc flex-col gap-1 pl-5 last:mb-0">{value}</ul>,
        ol: ({ children: value }) => <ol className="mb-3 flex list-decimal flex-col gap-1 pl-5 last:mb-0">{value}</ol>,
        li: ({ children: value }) => <li className="pl-0.5">{value}</li>,
        blockquote: ({ children: value }) => <blockquote className="my-3 border-l-2 border-border pl-3 text-muted-foreground">{value}</blockquote>,
        a: ({ children: value, href }) => <a href={href} target="_blank" rel="noreferrer" className="font-medium text-primary underline underline-offset-4">{value}</a>,
        code: ({ children: value, className: codeClassName }) => codeClassName
          ? <code className={codeClassName}>{value}</code>
          : <code className="rounded bg-muted px-1 py-0.5 font-mono text-[0.9em]">{value}</code>,
        pre: ({ children: value }) => <pre className="my-3 max-w-full overflow-x-auto rounded-lg border bg-muted/50 p-3 font-mono text-xs leading-5">{value}</pre>,
        hr: () => <hr className="my-4 border-border"/>,
        table: ({ children: value }) => <div className="my-3 max-w-full overflow-x-auto rounded-lg border"><table className="w-full border-collapse text-left text-xs">{value}</table></div>,
        thead: ({ children: value }) => <thead className="bg-muted/60">{value}</thead>,
        th: ({ children: value }) => <th className="border-b px-3 py-2 font-medium">{value}</th>,
        td: ({ children: value }) => <td className="border-b px-3 py-2 align-top last:border-b-0">{value}</td>,
        strong: ({ children: value }) => <strong className="font-semibold text-foreground">{value}</strong>,
      }}
    >
      {children}
    </ReactMarkdown>
  </div>;
}
