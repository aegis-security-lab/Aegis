import * as React from "react"

import { RouteBreadcrumbs } from "@/components/route-breadcrumbs"
import { cn } from "@/lib/utils"

export function ContentArea({
  children,
  fullHeight = false,
}: {
  children: React.ReactNode
  fullHeight?: boolean
}) {
  return (
    <div className="flex size-full min-h-0 flex-col">
      <RouteBreadcrumbs />
      <div
        id="main-content"
        tabIndex={-1}
        data-slot="app-content-scroller"
        className={cn(
          "min-h-0 w-full flex-1 scroll-mt-14 outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-inset",
          fullHeight ? "overflow-hidden" : "overflow-y-auto overscroll-y-contain"
        )}
      >
        <div
          data-slot="app-content"
          className={cn(
            "mx-auto flex w-full max-w-[1600px] flex-col gap-4 px-3 py-3 lg:px-4 lg:py-4",
            fullHeight ? "size-full min-h-0" : "min-h-full"
          )}
        >
          {children}
        </div>
      </div>
    </div>
  )
}
