export type TabState = {
  id: string
  path: string
}

const TABS_KEY = "aegis.tabs.v1"
const ACTIVE_KEY = "aegis.active-tab.v1"

export function newTabId() {
  return `tab-${crypto.randomUUID()}`
}

export function loadTabs(): TabState[] | null {
  try {
    const raw = localStorage.getItem(TABS_KEY)
    if (!raw) return null
    const parsed: unknown = JSON.parse(raw)
    if (!Array.isArray(parsed)) return null
    const tabs = parsed.filter(
      (item): item is TabState =>
        typeof item === "object" &&
        item !== null &&
        typeof (item as TabState).id === "string" &&
        typeof (item as TabState).path === "string"
    )
    return tabs.length > 0 ? tabs : null
  } catch {
    return null
  }
}

export function loadActiveTabId(): string | null {
  try {
    const raw = localStorage.getItem(ACTIVE_KEY)
    return typeof raw === "string" && raw.length > 0 ? raw : null
  } catch {
    return null
  }
}

export function saveTabs(tabs: TabState[], activeId: string) {
  try {
    localStorage.setItem(TABS_KEY, JSON.stringify(tabs))
    localStorage.setItem(ACTIVE_KEY, activeId)
  } catch {
    // ignore storage failures (e.g. private mode)
  }
}
