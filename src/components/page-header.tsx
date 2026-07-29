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
    if (!actions) return null
    return <div className="flex items-center justify-end gap-2">{actions}</div>
  }

  return (
    <header className="flex flex-col gap-3 border-b pb-4 sm:flex-row sm:items-start sm:justify-between">
      <div className="flex min-w-0 flex-col gap-1">
        {eyebrow ? (
          <p className="font-mono text-[11px] font-medium tracking-wide break-all text-muted-foreground uppercase">
            {eyebrow}
          </p>
        ) : null}
        <h1
          className="text-xl font-semibold tracking-tight text-balance break-words"
          title={title}
        >
          {title}
        </h1>
        {description ? (
          <p className="max-w-3xl text-sm leading-5 text-pretty text-muted-foreground">
            {description}
          </p>
        ) : null}
      </div>
      {actions ? (
        <div className="flex shrink-0 flex-wrap items-center gap-2">
          {actions}
        </div>
      ) : null}
    </header>
  )
}
