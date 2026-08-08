import { Clock3 } from "lucide-react"

import { Switch } from "@/components/ui/switch"

const pillClass =
  "flex h-7 items-center gap-2 rounded-full border border-input bg-background px-3 text-xs font-medium shadow-xs transition-colors hover:bg-muted/60"

export function ObjectiveToggle({
  checked,
  onCheckedChange,
}: {
  checked: boolean
  onCheckedChange: (checked: boolean) => void
}) {
  return (
    <label className={`${pillClass} cursor-pointer`}>
      <Switch size="sm" checked={checked} onCheckedChange={onCheckedChange} />
      设置目标
    </label>
  )
}

export function TimeBudgetControl({
  value,
  onValueChange,
}: {
  value?: number
  onValueChange: (value?: number) => void
}) {
  return (
    <label className={pillClass} title="时间预算（分钟）">
      <Clock3 className="size-3.5 text-muted-foreground" />
      <input
        type="number"
        min={1}
        step={1}
        value={value ?? ""}
        onChange={(event) =>
          onValueChange(
            event.target.value
              ? Math.max(1, Number.parseInt(event.target.value, 10))
              : undefined
          )
        }
        placeholder="不限"
        aria-label="时间预算（分钟）"
        className="w-12 bg-transparent text-right tabular-nums outline-none placeholder:text-muted-foreground"
      />
      <span className="text-muted-foreground">分钟</span>
    </label>
  )
}

export function HumanValidationToggle({
  checked,
  onCheckedChange,
}: {
  checked: boolean
  onCheckedChange: (checked: boolean) => void
}) {
  return (
    <label className={`${pillClass} cursor-pointer`}>
      <Switch size="sm" checked={checked} onCheckedChange={onCheckedChange} />
      人工兜底验收
    </label>
  )
}
