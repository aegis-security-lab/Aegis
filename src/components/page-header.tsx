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
  description,
  actions,
  showIdentity = true,
}: PageHeaderProps) {
  if (!showIdentity) {
    return actions ? (
      <div className="flex shrink-0 flex-wrap items-center justify-end gap-2">
        {actions}
      </div>
    ) : null
  }
  return (
    <header
      data-slot="page-header"
      className="flex shrink-0 flex-col gap-3 border-b border-border/60 pb-4 sm:flex-row sm:items-end sm:justify-between"
    >
      <div data-slot="page-header-identity" className="min-w-0">
        {eyebrow ? (
          <p className="mb-1 text-[10px] font-semibold tracking-[0.14em] text-muted-foreground uppercase">
            {eyebrow}
          </p>
        ) : null}
        <h1 className="truncate text-xl leading-tight font-semibold tracking-[-0.02em] text-foreground sm:text-2xl">
          {title}
        </h1>
        {description ? (
          <p className="mt-1 max-w-2xl text-sm leading-5 text-muted-foreground">
            {description}
          </p>
        ) : null}
      </div>
      {actions ? (
        <div
          data-slot="page-header-actions"
          className="flex shrink-0 flex-wrap items-center gap-2"
        >
          {actions}
        </div>
      ) : null}
    </header>
  )
}
