const LEADING_EXTERNAL_ISSUE_RE = /^[A-Z][A-Z0-9_-]*-\d+:\s*/;

export function buildLinkedIssueTitle(taskTitle: string | null | undefined, key: string): string {
  const stripped = (taskTitle ?? "").trim().replace(LEADING_EXTERNAL_ISSUE_RE, "");
  return stripped ? `${key}: ${stripped}` : key;
}
