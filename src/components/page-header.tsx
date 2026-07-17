import type { ReactNode } from "react"

interface PageHeaderProps {
  eyebrow?: string
  title: string
  description?: string
  actions?: ReactNode
}

export function PageHeader({ actions }: PageHeaderProps) {
  if (!actions) return null

  return (
    <div className="flex items-center justify-end gap-2">
      {actions}
    </div>
  )
}
