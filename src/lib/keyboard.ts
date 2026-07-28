import type * as React from "react"

/**
 * Enter sends only after an IME composition has fully finished.
 * keyCode 229 covers browsers that clear isComposing too early while the
 * Chinese/Japanese/Korean candidate window is accepting the Enter key.
 */
export function isSendMessageKey(
  event: React.KeyboardEvent<HTMLTextAreaElement>
) {
  return (
    event.key === "Enter" &&
    !event.shiftKey &&
    !event.nativeEvent.isComposing &&
    event.nativeEvent.keyCode !== 229
  )
}
