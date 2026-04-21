/**
 * A task is treated as a PR review when its metadata contains a
 * non-empty `review_watch_id` (set by the GitHub PR review watcher).
 */
export function isPRReviewFromMetadata(metadata: unknown): boolean {
  if (!metadata || typeof metadata !== "object") return false;
  const watchId = (metadata as Record<string, unknown>)["review_watch_id"];
  return typeof watchId === "string" && watchId.length > 0;
}
