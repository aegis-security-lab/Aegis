const storageKey = "aegis-access-token"
const unauthorizedEvent = "aegis:unauthorized"

export function accessToken() {
  return window.localStorage.getItem(storageKey) ?? ""
}

export function authorizationHeaders(headers?: HeadersInit) {
  const result = new Headers(headers)
  const token = accessToken()
  if (token) result.set("Authorization", `Bearer ${token}`)
  return result
}

export function saveAccessToken(token: string) {
  window.localStorage.setItem(storageKey, token)
}

export function clearAccessToken() {
  window.localStorage.removeItem(storageKey)
}

export function reportUnauthorized() {
  clearAccessToken()
  window.dispatchEvent(new Event(unauthorizedEvent))
}

export function onUnauthorized(listener: () => void) {
  window.addEventListener(unauthorizedEvent, listener)
  return () => window.removeEventListener(unauthorizedEvent, listener)
}
