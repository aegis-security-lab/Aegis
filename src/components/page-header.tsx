import type { ReactNode } from "react"

interface PageHeaderProps {
  eyebrow?: string
  title: string
  description?: string
  actions?: ReactNode
  showIdentity?: boolean
}

export function PageHeader({
  actions,
}: PageHeaderProps) {
  if (!actions) return null
  return (
    <div className="flex shrink-0 flex-wrap items-center justify-start gap-2">
      {actions}
    </div>
  )
}
