import type { ReactNode } from "react"

interface PageHeaderProps {
  eyebrow?: string
  title: string
  description?: string
  actions?: ReactNode
  showIdentity?: boolean
}

export function PageHeader({
  eyebrow,
  title,
  actions,
  showIdentity = false,
}: PageHeaderProps) {
  if (!showIdentity) {
    if (!actions) return null
    return (
      <div className="flex items-center justify-end gap-2">{actions}</div>
    )
  }

  return (
    <header className="flex flex-col gap-4 border-b pb-5 lg:flex-row lg:items-start lg:justify-between">
      <div className="flex min-w-0 flex-col gap-1.5">
        {eyebrow ? (
          <p className="font-mono text-xs text-muted-foreground break-all">
            {eyebrow}
          </p>
        ) : null}
        <h1
          className="text-2xl font-semibold tracking-tight break-words"
          title={title}
        >
          {title}
        </h1>
      </div>
      {actions ? (
        <div className="flex shrink-0 flex-wrap items-center gap-2">
          {actions}
        </div>
      ) : null}
    </header>
  )
}
