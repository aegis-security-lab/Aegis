export async function agentTemplateId(
  provider: string,
  model: string,
  systemPrompt: string
) {
  const identity = [provider, model, systemPrompt]
    .map((value) => value.trim())
    .join("\u0000")
  const bytes = new TextEncoder().encode(identity)
  const digest = await crypto.subtle.digest("SHA-256", bytes)
  const hash = [...new Uint8Array(digest)]
    .map((value) => value.toString(16).padStart(2, "0"))
    .join("")
  return hash
}
