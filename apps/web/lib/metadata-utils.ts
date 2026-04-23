import type { Task } from "@/lib/types/http";

/** Check whether a metadata key holds a non-empty watch ID string. */
function hasWatchId(metadata: Task["metadata"], key: string): boolean {
  if (!metadata || typeof metadata !== "object") return false;
  const watchId = (metadata as Record<string, unknown>)[key];
  return typeof watchId === "string" && watchId.length > 0;
}

/** True when the task was created by a PR review watcher. */
export function isPRReviewFromMetadata(metadata: Task["metadata"]): boolean {
  return hasWatchId(metadata, "review_watch_id");
}

/** True when the task was created by an issue watcher. */
export function isIssueWatchFromMetadata(metadata: Task["metadata"]): boolean {
  return hasWatchId(metadata, "issue_watch_id");
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
