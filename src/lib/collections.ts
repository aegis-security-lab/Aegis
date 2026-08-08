export function issuesInSubtree<T extends { id: string; parentId?: string }>(
  rootId: string,
  issues: T[]
) {
  const issueById = new Map(issues.map((issue) => [issue.id, issue]))
  const childrenByParent = new Map<string, T[]>()
  for (const issue of issues) {
    if (!issue.parentId) continue
    const children = childrenByParent.get(issue.parentId)
    if (children) children.push(issue)
    else childrenByParent.set(issue.parentId, [issue])
  }

  const result: T[] = []
  const seen = new Set<string>()
  const visit = (id: string) => {
    if (seen.has(id)) return
    seen.add(id)
    const issue = issueById.get(id)
    if (!issue) return
    result.push(issue)
    for (const child of childrenByParent.get(id) ?? []) visit(child.id)
  }
  visit(rootId)
  return result
}

export function issuesForTask<
  T extends { id: string; parentId?: string; taskSourceId?: string },
>(taskSourceId: string, issues: T[]) {
  return issuesByTask(issues).get(taskSourceId) ?? []
}

export function issuesByTask<
  T extends { id: string; parentId?: string; taskSourceId?: string },
>(issues: T[]) {
  const childrenByParent = new Map<string, T[]>()
  const roots: T[] = []
  for (const issue of issues) {
    if (!issue.parentId) {
      if (issue.taskSourceId) roots.push(issue)
      continue
    }
    const children = childrenByParent.get(issue.parentId)
    if (children) children.push(issue)
    else childrenByParent.set(issue.parentId, [issue])
  }

  const result = new Map<string, T[]>()
  const seen = new Set<string>()
  const visit = (taskSourceId: string, issue: T) => {
    if (seen.has(issue.id)) return
    seen.add(issue.id)
    const taskIssues = result.get(taskSourceId)
    if (taskIssues) taskIssues.push(issue)
    else result.set(taskSourceId, [issue])
    for (const child of childrenByParent.get(issue.id) ?? []) {
      visit(taskSourceId, child)
    }
  }
  for (const root of roots) visit(root.taskSourceId!, root)
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

export function latestUpdatedAt<T extends { updatedAt: string }>(
  fallback: string,
  items: T[]
) {
  return items.reduce(
    (latest, item) => (item.updatedAt > latest ? item.updatedAt : latest),
    fallback
  )
}
