/* eslint-disable react-hooks/refs, react-hooks/set-state-in-effect, react-refresh/only-export-components */
import * as React from "react"
import { createPortal } from "react-dom"

import { Textarea } from "@/components/ui/textarea"
import { cn } from "@/lib/utils"
import type { Issue } from "@/types"

type MentionRange = {
  start: number
  end: number
  query: string
}

type IssueMentionController = {
  listboxId: string
  inputRef: React.RefObject<HTMLTextAreaElement | null>
  activeIndex: number
  candidates: Issue[]
  references: Issue[]
  open: boolean
  handleBlur: () => void
  handleChange: (event: React.ChangeEvent<HTMLTextAreaElement>) => void
  handleKeyDown: (event: React.KeyboardEvent<HTMLTextAreaElement>) => boolean
  handleSelect: (issue: Issue) => void
}

export function useIssueMentions({
  value,
  onValueChange,
  issues = [],
}: {
  value: string
  onValueChange: (value: string) => void
  issues?: Issue[]
}): IssueMentionController {
  const inputRef = React.useRef<HTMLTextAreaElement>(null)
  const listboxId = React.useId()
  const [mention, setMention] = React.useState<MentionRange | null>(null)
  const [activeIndex, setActiveIndex] = React.useState(0)

  const candidates = React.useMemo(
    () => rankIssues(issues, mention?.query ?? "").slice(0, 100),
    [issues, mention?.query]
  )
  const references = React.useMemo(
    () => referencedIssues(value, issues),
    [issues, value]
  )

  React.useEffect(() => {
    setActiveIndex(0)
  }, [mention?.query])

  const updateMention = React.useCallback(
    (nextValue: string, caret: number | null) => {
      setMention(issues.length > 0 ? activeMention(nextValue, caret) : null)
    },
    [issues.length]
  )

  const handleChange = (event: React.ChangeEvent<HTMLTextAreaElement>) => {
    const nextValue = event.target.value
    onValueChange(nextValue)
    updateMention(nextValue, event.target.selectionStart)
  }

  const handleSelect = React.useCallback(
    (issue: Issue) => {
      if (!mention) return
      const replacement = `@${issue.identifier} `
      const nextValue =
        value.slice(0, mention.start) + replacement + value.slice(mention.end)
      const nextCaret = mention.start + replacement.length
      onValueChange(nextValue)
      setMention(null)
      requestAnimationFrame(() => {
        inputRef.current?.focus()
        inputRef.current?.setSelectionRange(nextCaret, nextCaret)
      })
    },
    [mention, onValueChange, value]
  )

  const handleKeyDown = (event: React.KeyboardEvent<HTMLTextAreaElement>) => {
    if (!mention) return false
    if (event.key === "Escape") {
      event.preventDefault()
      setMention(null)
      return true
    }
    if (event.key === "ArrowDown" || event.key === "ArrowUp") {
      event.preventDefault()
      if (candidates.length > 0) {
        const direction = event.key === "ArrowDown" ? 1 : -1
        setActiveIndex(
          (current) =>
            (current + direction + candidates.length) % candidates.length
        )
      }
      return true
    }
    if ((event.key === "Enter" || event.key === "Tab") && candidates.length) {
      event.preventDefault()
      handleSelect(candidates[activeIndex] ?? candidates[0])
      return true
    }
    return false
  }

  return {
    listboxId,
    inputRef,
    activeIndex,
    candidates,
    references,
    open: mention !== null,
    handleBlur: () => setMention(null),
    handleChange,
    handleKeyDown,
    handleSelect,
  }
}

export function IssueMentionTextarea({
  issues,
  value,
  onValueChange,
  className,
  ...props
}: Omit<React.ComponentProps<typeof Textarea>, "value" | "onChange"> & {
  issues: Issue[]
  value: string
  onValueChange: (value: string) => void
}) {
  const mentions = useIssueMentions({ value, onValueChange, issues })
  return (
    <div className="relative min-w-0">
      <Textarea
        {...props}
        ref={mentions.inputRef}
        value={value}
        onChange={mentions.handleChange}
        onKeyDown={(event) => {
          if (!mentions.handleKeyDown(event)) props.onKeyDown?.(event)
        }}
        onBlur={(event) => {
          mentions.handleBlur()
          props.onBlur?.(event)
        }}
        aria-autocomplete="list"
        aria-expanded={mentions.open}
        aria-controls={mentions.open ? mentions.listboxId : undefined}
        className={className}
      />
      <IssueMentionSuggestions controller={mentions} />
      <IssueReferencePreview references={mentions.references} />
    </div>
  )
}

export function IssueMentionSuggestions({
  controller,
}: {
  controller: IssueMentionController
}) {
  const activeOptionRef = React.useRef<HTMLButtonElement>(null)
  const [layout, setLayout] = React.useState<{
    left: number
    top: number
    width: number
    maxHeight: number
    placement: "top" | "bottom"
  } | null>(null)
  React.useEffect(() => {
    activeOptionRef.current?.scrollIntoView({ block: "nearest" })
  }, [controller.activeIndex])

  React.useLayoutEffect(() => {
    if (!controller.open) {
      setLayout(null)
      return
    }
    const update = () => {
      const anchor = controller.inputRef.current
      if (!anchor) return
      const rect = anchor.getBoundingClientRect()
      const viewportPadding = 12
      const gap = 6
      const desiredHeight = Math.min(
        240,
        Math.max(96, controller.candidates.length * 40 + 8)
      )
      const spaceBelow = window.innerHeight - rect.bottom - viewportPadding
      const spaceAbove = rect.top - viewportPadding
      const placement =
        spaceBelow >= Math.min(desiredHeight, 160) || spaceBelow >= spaceAbove
          ? "bottom"
          : "top"
      const available = placement === "bottom" ? spaceBelow : spaceAbove
      const width = Math.min(
        600,
        Math.max(320, rect.width),
        window.innerWidth - viewportPadding * 2
      )
      setLayout({
        left: Math.max(
          viewportPadding,
          Math.min(
            rect.right - width,
            window.innerWidth - width - viewportPadding
          )
        ),
        top: placement === "bottom" ? rect.bottom + gap : rect.top - gap,
        width,
        maxHeight: Math.max(96, Math.min(240, available - gap)),
        placement,
      })
    }
    update()
    window.addEventListener("resize", update)
    window.addEventListener("scroll", update, true)
    return () => {
      window.removeEventListener("resize", update)
      window.removeEventListener("scroll", update, true)
    }
  }, [controller.candidates.length, controller.inputRef, controller.open])

  if (!controller.open || !layout) return null
  return createPortal(
    <div
      id={controller.listboxId}
      role="listbox"
      aria-label="当前任务的 Issue"
      style={{
        left: layout.left,
        top: layout.top,
        width: layout.width,
        maxHeight: layout.maxHeight,
        transform: layout.placement === "top" ? "translateY(-100%)" : undefined,
      }}
      className="fixed z-100 overflow-y-auto rounded-lg border border-border/80 bg-popover p-1 text-popover-foreground shadow-xl"
    >
      {controller.candidates.length ? (
        controller.candidates.map((issue, index) => (
          <button
            key={issue.id}
            ref={index === controller.activeIndex ? activeOptionRef : undefined}
            type="button"
            role="option"
            aria-selected={index === controller.activeIndex}
            onMouseDown={(event) => event.preventDefault()}
            onClick={() => controller.handleSelect(issue)}
            className={cn(
              "group grid w-full grid-cols-[5.75rem_minmax(0,1fr)] items-baseline gap-1.5 rounded-md px-2.5 py-2 text-left text-sm transition-[background-color,box-shadow] duration-100 outline-none",
              index === controller.activeIndex
                ? "bg-zinc-200/85 text-foreground ring-1 ring-violet-400/25 dark:bg-zinc-700/80"
                : "text-foreground hover:bg-zinc-100/90 dark:hover:bg-zinc-800/80"
            )}
          >
            <span
              className={cn(
                "font-mono text-xs font-semibold",
                index === controller.activeIndex
                  ? "text-zinc-950 dark:text-zinc-50"
                  : "text-zinc-700 dark:text-zinc-200"
              )}
            >
              {issue.identifier}
            </span>
            <span
              className={cn(
                "truncate",
                index === controller.activeIndex
                  ? "text-zinc-900 dark:text-zinc-100"
                  : "text-zinc-600 dark:text-zinc-300"
              )}
              title={[issue.title, issue.description]
                .filter(Boolean)
                .join(" · ")}
            >
              {issue.description || issue.title || "无描述"}
            </span>
          </button>
        ))
      ) : (
        <div className="px-3 py-6 text-center text-sm text-muted-foreground">
          当前任务内没有匹配的 Issue
        </div>
      )}
    </div>,
    document.body
  )
}

export function IssueReferencePreview({ references }: { references: Issue[] }) {
  if (!references.length) return null
  return (
    <div
      className="mt-1.5 flex min-w-0 flex-wrap items-center gap-1.5"
      aria-label="已引用 Issue"
    >
      <span className="text-[11px] text-muted-foreground">已引用</span>
      {references.map((issue) => (
        <span
          key={issue.id}
          title={issue.title}
          className="inline-flex h-6 items-center rounded-md border border-primary/25 bg-primary/10 px-2 font-mono text-[11px] font-semibold text-primary"
        >
          @{issue.identifier}
        </span>
      ))}
    </div>
  )
}

function activeMention(
  value: string,
  caret: number | null
): MentionRange | null {
  if (caret === null) return null
  const beforeCaret = value.slice(0, caret)
  const match = beforeCaret.match(/(^|\s)@([^\s@]*)$/)
  if (!match) return null
  const atOffset = match[0].lastIndexOf("@")
  return {
    start: caret - match[0].length + atOffset,
    end: caret,
    query: match[2],
  }
}

function rankIssues(issues: Issue[], query: string) {
  const normalized = query.trim().toLocaleLowerCase()
  if (!normalized) return [...issues].sort((a, b) => b.number - a.number)
  return issues
    .map((issue) => {
      const identifier = issue.identifier.toLocaleLowerCase()
      const title = issue.title.toLocaleLowerCase()
      const description = issue.description.toLocaleLowerCase()
      const objective = issue.objective.toLocaleLowerCase()
      let score = 0
      if (identifier === normalized) score = 100
      else if (identifier.startsWith(normalized)) score = 80
      else if (identifier.includes(normalized)) score = 60
      else if (title.includes(normalized)) score = 40
      else if (description.includes(normalized)) score = 30
      else if (objective.includes(normalized)) score = 20
      return { issue, score }
    })
    .filter(({ score }) => score > 0)
    .sort((a, b) => b.score - a.score || b.issue.number - a.issue.number)
    .map(({ issue }) => issue)
}

function referencedIssues(value: string, issues: Issue[]) {
  const issueByIdentifier = new Map(
    issues.map((issue) => [issue.identifier.toLocaleLowerCase(), issue])
  )
  const references: Issue[] = []
  const seen = new Set<string>()
  for (const match of value.matchAll(/@([a-z0-9][a-z0-9_-]*)/gi)) {
    const issue = issueByIdentifier.get(match[1].toLocaleLowerCase())
    if (!issue || seen.has(issue.id)) continue
    seen.add(issue.id)
    references.push(issue)
  }
  return references
}
