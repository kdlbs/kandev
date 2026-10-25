import type { Task } from "@/lib/types/http";

const REVIEW_WATCH_KEYS = ["review_watch_id", "gitlab_review_watch_id", "azure_devops_pr_watch_id"];
const ISSUE_WATCH_KEYS = [
  "issue_watch_id",
  "gitlab_issue_watch_id",
  "jira_issue_watch_id",
  "linear_issue_watch_id",
  "sentry_issue_watch_id",
  "azure_devops_work_item_watch_id",
];

/** Check whether a metadata key holds a non-empty watch ID string. */
function hasWatchId(metadata: Task["metadata"], key: string): boolean {
  if (!metadata || typeof metadata !== "object") return false;
  const watchId = (metadata as Record<string, unknown>)[key];
  return typeof watchId === "string" && watchId.length > 0 && watchId !== "undefined";
}

/** True when the task was created by a PR review watcher. */
export function isPRReviewFromMetadata(metadata: Task["metadata"]): boolean {
  return REVIEW_WATCH_KEYS.some((key) => hasWatchId(metadata, key));
}

/** True when the task was created by an issue watcher. */
export function isIssueWatchFromMetadata(metadata: Task["metadata"]): boolean {
  return ISSUE_WATCH_KEYS.some((key) => hasWatchId(metadata, key));
}

/** Extract issue-specific fields from task metadata. */
export function issueFieldsFromMetadata(metadata: Task["metadata"]): {
  issueUrl?: string;
  issueNumber?: number;
} {
  if (!metadata || typeof metadata !== "object") return {};
  const m = metadata as Record<string, unknown>;
  const url = typeof m["issue_url"] === "string" ? m["issue_url"] : undefined;
  const num = typeof m["issue_number"] === "number" ? m["issue_number"] : undefined;
  if (!url && !num) return {};
  return { issueUrl: url, issueNumber: num };
}
