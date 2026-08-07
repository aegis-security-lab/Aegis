export function issuesInSubtree<T extends { id: string; parentId?: string }>(
  rootId: string,
  issues: T[]
) {
  const result: T[] = []
  const seen = new Set<string>()
  const visit = (id: string) => {
    if (seen.has(id)) return
    seen.add(id)
    const issue = issues.find((candidate) => candidate.id === id)
    if (!issue) return
    result.push(issue)
    for (const candidate of issues) {
      if (candidate.parentId === id) visit(candidate.id)
    }
  }
  visit(rootId)
  return result
}

export function mergeById<T extends { id: string }>(
  current: T[],
  incoming: T[],
  order: (a: T, b: T) => number
) {
  const byId = new Map(current.map((item) => [item.id, item]))
  for (const item of incoming) byId.set(item.id, item)
  return [...byId.values()].sort(order)
}

export const chronological = <T extends { createdAt: string }>(a: T, b: T) =>
  a.createdAt.localeCompare(b.createdAt)

export const reverseChronological = <T extends { createdAt: string }>(
  a: T,
  b: T
) => b.createdAt.localeCompare(a.createdAt)
