import * as React from "react"
import { Search, X } from "lucide-react"

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
  const [open, setOpen] = React.useState(() => Boolean(value))
  const containerRef = React.useRef<HTMLDivElement>(null)
  const inputRef = React.useRef<HTMLInputElement>(null)

  const openSearch = React.useCallback(() => {
    setOpen(true)
    window.requestAnimationFrame(() => inputRef.current?.focus())
  }, [])

  const closeSearch = React.useCallback(
    (clear: boolean) => {
      setOpen(false)
      if (clear) onChange("")
    },
    [onChange]
  )

  return (
    <div
      ref={containerRef}
      className={cn(
        "relative h-7 shrink-0 overflow-hidden rounded-full border border-border bg-background/70 shadow-sm transition-[width,border-color,box-shadow] duration-200 ease-[cubic-bezier(0.23,1,0.32,1)] focus-within:border-ring focus-within:ring-2 focus-within:ring-ring/40 max-md:h-11 dark:bg-input/30",
        open
          ? "w-48 sm:w-56"
          : "w-7 hover:border-ring/40 hover:bg-muted max-md:w-11",
        className
      )}
    >
      <Search
        className={cn(
          "pointer-events-none absolute top-1/2 left-2 z-10 size-3.5 -translate-y-1/2 transition-colors duration-150 max-md:left-3.5",
          open ? "text-foreground" : "text-muted-foreground"
        )}
      />
      <Button
        type="button"
        variant="ghost"
        size="icon"
        aria-label={ariaLabel}
        aria-expanded={open}
        className={cn(
          "absolute inset-0 z-20 size-full rounded-full transition-opacity duration-100 focus-visible:ring-2 focus-visible:ring-ring/40",
          open ? "pointer-events-none opacity-0" : "opacity-100"
        )}
        onClick={openSearch}
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
          if (containerRef.current?.contains(next) || value) return
          closeSearch(false)
        }}
        tabIndex={open ? 0 : -1}
        aria-hidden={!open}
        aria-label={ariaLabel}
        placeholder={placeholder}
        className={cn(
          "absolute inset-0 h-full w-full rounded-full border-0 bg-transparent pr-8 pl-8 text-[13px] shadow-none transition-opacity duration-150 focus-visible:border-0 focus-visible:ring-0 max-md:pl-10 dark:bg-transparent",
          open
            ? "pointer-events-auto opacity-100"
            : "pointer-events-none opacity-0"
        )}
      />
      <Button
        type="button"
        variant="ghost"
        size="icon-xs"
        aria-label="清除并收起搜索"
        tabIndex={open ? 0 : -1}
        className={cn(
          "absolute top-1/2 right-1.5 z-20 -translate-y-1/2 rounded-full text-muted-foreground transition-[opacity,color,background-color] duration-150 after:absolute after:-inset-1 hover:bg-muted hover:text-foreground max-md:right-3",
          open ? "opacity-100" : "pointer-events-none opacity-0"
        )}
        onClick={() => closeSearch(true)}
      >
        <X />
      </Button>
    </div>
  )
}
