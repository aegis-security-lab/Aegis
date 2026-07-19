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
