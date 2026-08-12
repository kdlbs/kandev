import { truncateRemoteTaskTitle } from "@/lib/task-title";

const LEADING_EXTERNAL_ISSUE_RE = /^[A-Z][A-Z0-9_-]*-\d+:\s*/;

export function buildLinkedIssueTitle(taskTitle: string | null | undefined, key: string): string {
  const stripped = (taskTitle ?? "").trim().replace(LEADING_EXTERNAL_ISSUE_RE, "");
  const composed = stripped ? `${key}: ${stripped}` : key;
  // The task title limit is enforced server-side; cap the composed title here
  // so linking an issue never fails with "task title is too long".
  return truncateRemoteTaskTitle(composed);
}
