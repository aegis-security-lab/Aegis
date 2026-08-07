import * as React from "react"
import { ChevronRight } from "lucide-react"
import { Link, useLocation } from "react-router-dom"

import { buildCrumbs } from "@/lib/route-crumbs"
import { useAppState } from "@/lib/state"

export function RouteBreadcrumbs() {
  const { state } = useAppState()
  const location = useLocation()
  const fromBoard = new URLSearchParams(location.search).get("view") === "board"
  const crumbs = buildCrumbs(
    location.pathname,
    state?.issues ?? [],
    state?.tasks ?? [],
    fromBoard
  )
  if (crumbs.length === 0) return null

  return (
    <nav
      aria-label="面包屑"
      className="flex h-9 shrink-0 items-center gap-0.5 overflow-x-auto border-b border-border/70 px-3"
    >
      {crumbs.map((crumb, index) => (
        <React.Fragment key={`${crumb.label}-${index}`}>
          {index > 0 ? (
            <ChevronRight className="size-3.5 shrink-0 text-muted-foreground/60" />
          ) : null}
          {crumb.to && !crumb.current ? (
            <Link
              to={crumb.to}
              className="max-w-56 truncate rounded-md px-1.5 py-0.5 text-[0.8rem] text-muted-foreground transition-colors hover:bg-muted hover:text-foreground"
            >
              {crumb.label}
            </Link>
          ) : (
            <span
              aria-current="page"
              className="max-w-56 truncate rounded-md px-1.5 py-0.5 text-[0.8rem] font-medium text-foreground"
            >
              {crumb.label}
            </span>
          )}
        </React.Fragment>
      ))}
    </nav>
  )
}
