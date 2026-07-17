export function formatTime(value?: string) {
  if (!value) return "—"
  return new Intl.DateTimeFormat("zh-CN", {
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
    second: "2-digit",
  }).format(new Date(value))
}

export function formatDuration(start: string, end?: string) {
  const seconds = Math.max(
    0,
    Math.round(
      (new Date(end ?? Date.now()).getTime() - new Date(start).getTime()) / 1000
    )
  )
  if (seconds < 60) return `${seconds}s`
  const minutes = Math.floor(seconds / 60)
  if (minutes < 60) return `${minutes}m ${seconds % 60}s`
  return `${Math.floor(minutes / 60)}h ${minutes % 60}m`
}

export function formatTokens(tokens: number) {
  if (tokens < 1000) return String(tokens)
  return `${(tokens / 1000).toFixed(tokens < 10000 ? 1 : 0)}k`
}

export function formatCost(cost: number) {
  if (!Number.isFinite(cost) || cost <= 0) return "$0.0000"
  if (cost < 0.01) return `$${cost.toFixed(6)}`
  if (cost < 100) return `$${cost.toFixed(4)}`
  return new Intl.NumberFormat("en-US", {
    style: "currency",
    currency: "USD",
    minimumFractionDigits: 2,
    maximumFractionDigits: 2,
  }).format(cost)
}

export function formatBytes(bytes: number) {
  if (bytes < 1024) return `${bytes} B`
  const units = ["KB", "MB", "GB"]
  let value = bytes / 1024
  let unit = units[0]
  for (let index = 1; index < units.length && value >= 1024; index += 1) {
    value /= 1024
    unit = units[index]
  }
  return `${value.toFixed(value < 10 ? 1 : 0)} ${unit}`
}
