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
