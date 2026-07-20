const normalize = (value: string) =>
  value
    .trim()
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, "-")
    .replace(/^-+|-+$/g, "")

export async function agentTemplateId(
  provider: string,
  model: string,
  systemPrompt: string
) {
  const bytes = new TextEncoder().encode(systemPrompt.trim())
  const digest = await crypto.subtle.digest("SHA-256", bytes)
  const hash = [...new Uint8Array(digest)]
    .map((value) => value.toString(16).padStart(2, "0"))
    .join("")
    .slice(0, 6)
  return `${normalize(provider) || "global"}-${normalize(model) || "default"}-${hash}`
}
