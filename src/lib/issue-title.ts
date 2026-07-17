export const ISSUE_TITLE_MAX_LENGTH = 120

export function limitIssueTitle(value: string) {
  return Array.from(value).slice(0, ISSUE_TITLE_MAX_LENGTH).join("")
}

export function issueTitleLength(value: string) {
  return Array.from(value).length
}
