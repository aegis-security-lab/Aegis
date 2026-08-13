const unauthorizedEvent = "aegis:unauthorized"

export function authorizationHeaders(headers?: HeadersInit) {
  return new Headers(headers)
}

export function clearAccessToken() {
  // Compatibility no-op: sessions are held only in the HttpOnly cookie.
}

export function reportUnauthorized() {
  clearAccessToken()
  window.dispatchEvent(new Event(unauthorizedEvent))
}

export function onUnauthorized(listener: () => void) {
  window.addEventListener(unauthorizedEvent, listener)
  return () => window.removeEventListener(unauthorizedEvent, listener)
}
