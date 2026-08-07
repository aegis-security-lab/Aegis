import * as React from "react"
import { Search } from "lucide-react"

import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { cn } from "@/lib/utils"

export function SearchInput({
  value,
  onChange,
  placeholder = "搜索…",
  ariaLabel = "搜索",
  className,
}: {
  value: string
  onChange: (value: string) => void
  placeholder?: string
  ariaLabel?: string
  className?: string
}) {
  const [open, setOpen] = React.useState(false)
  const containerRef = React.useRef<HTMLDivElement>(null)
  const inputRef = React.useRef<HTMLInputElement>(null)

  const openSearch = React.useCallback(() => {
    setOpen(true)
    window.setTimeout(() => inputRef.current?.focus(), 120)
  }, [])

  const closeSearch = React.useCallback(
    (clear: boolean) => {
      setOpen(false)
      if (clear) onChange("")
    },
    [onChange]
  )

  return (
    <div ref={containerRef} className={cn("flex items-center", className)}>
      <div
        className={cn(
          "relative transition-all duration-300 ease-[cubic-bezier(0.23,1,0.32,1)]",
          open
            ? "mr-2 w-40 opacity-100 sm:w-60"
            : "mr-0 w-0 opacity-0"
        )}
      >
        <Search
          className={cn(
            "pointer-events-none absolute top-1/2 left-3.5 size-3.5 -translate-y-1/2 text-muted-foreground transition-opacity duration-300",
            open ? "opacity-100" : "opacity-0"
          )}
        />
        <Input
          ref={inputRef}
          value={value}
          onChange={(event) => onChange(event.target.value)}
          onKeyDown={(event) => {
            if (event.key === "Escape") {
              event.preventDefault()
              closeSearch(true)
            }
          }}
          onBlur={(event) => {
            const next = event.relatedTarget as Node | null
            if (containerRef.current?.contains(next)) return
            closeSearch(false)
          }}
          tabIndex={open ? 0 : -1}
          aria-label={ariaLabel}
          placeholder={placeholder}
          className="h-9 rounded-full border-border bg-background/70 pl-9 pr-4 shadow-sm focus-visible:border-ring focus-visible:ring-2 focus-visible:ring-ring/40 dark:bg-input/30"
        />
      </div>
      <Button
        type="button"
        variant="outline"
        size="icon"
        aria-label={ariaLabel}
        aria-expanded={open}
        className={cn(
          "size-9 shrink-0 rounded-full shadow-sm transition-all duration-300 ease-[cubic-bezier(0.23,1,0.32,1)]",
          open
            ? "border-primary/50 bg-primary/5 text-primary ring-2 ring-primary/10"
            : "hover:border-ring/40 hover:shadow-md active:scale-95"
        )}
        onClick={() => {
          if (open) closeSearch(true)
          else openSearch()
        }}
      >
        <Search />
      </Button>
    </div>
  )
}
