import type { ReviewTaskStatus } from "@/lib/plugins/types";

export const CHANGE_REQUEST_STATUS_COLORS = {
  muted: "text-muted-foreground",
  merged: "text-purple-500",
  danger: "text-red-500",
  warning: "text-yellow-500",
  review: "text-sky-400",
  ready: "text-emerald-400",
  queued: "text-[#966600]",
  passing: "text-green-500",
} as const;

export function getChangeRequestAggregateStatusColor(state: string | null | undefined): string {
  switch (state?.toLowerCase()) {
    case "merged":
      return CHANGE_REQUEST_STATUS_COLORS.merged;
    case "closed":
    case "failure":
      return CHANGE_REQUEST_STATUS_COLORS.danger;
    case "pending":
      return CHANGE_REQUEST_STATUS_COLORS.warning;
    case "awaiting_review":
      return CHANGE_REQUEST_STATUS_COLORS.review;
    case "ready":
      return CHANGE_REQUEST_STATUS_COLORS.ready;
    case "queued":
      return CHANGE_REQUEST_STATUS_COLORS.queued;
    case "passing":
      return CHANGE_REQUEST_STATUS_COLORS.passing;
    case "draft":
    case "blocked":
    case "neutral":
    case "open":
    default:
      return CHANGE_REQUEST_STATUS_COLORS.muted;
  }
}

export const CHANGE_REQUEST_STATUS_RANK: Readonly<Record<string, number>> = {
  [CHANGE_REQUEST_STATUS_COLORS.danger]: 5,
  [CHANGE_REQUEST_STATUS_COLORS.warning]: 4,
  [CHANGE_REQUEST_STATUS_COLORS.review]: 3,
  [CHANGE_REQUEST_STATUS_COLORS.ready]: 2,
  [CHANGE_REQUEST_STATUS_COLORS.queued]: 1.5,
  [CHANGE_REQUEST_STATUS_COLORS.passing]: 1,
  [CHANGE_REQUEST_STATUS_COLORS.merged]: 0,
  [CHANGE_REQUEST_STATUS_COLORS.muted]: 0,
};

function isAwaitingReview(status: ReviewTaskStatus): boolean {
  const review = status.review;
  if (!review || status.pipelineState !== "success") return false;
  if (review.state === "pending") return true;
  if (review.required != null && review.approved < review.required) return true;
  return (review.requested ?? 0) > 0;
}

export function getReviewTaskStatusColor(status: ReviewTaskStatus): string {
  if (status.state === "merged") return CHANGE_REQUEST_STATUS_COLORS.merged;
  if (
    status.state === "closed" ||
    status.pipelineState === "failure" ||
    status.review?.state === "changes_requested"
  ) {
    return CHANGE_REQUEST_STATUS_COLORS.danger;
  }
  if (status.state === "draft") return CHANGE_REQUEST_STATUS_COLORS.muted;
  if (status.pipelineState === "pending") return CHANGE_REQUEST_STATUS_COLORS.warning;
  if (isAwaitingReview(status)) return CHANGE_REQUEST_STATUS_COLORS.review;
  if (status.pipelineState === "success") return CHANGE_REQUEST_STATUS_COLORS.passing;
  return CHANGE_REQUEST_STATUS_COLORS.muted;
}

export function aggregateReviewTaskStatusColor(statuses: readonly ReviewTaskStatus[]): string {
  if (statuses.length === 0) return CHANGE_REQUEST_STATUS_COLORS.muted;
  const active = statuses.filter((status) => status.state === "open" || status.state === "draft");
  const target = active.length > 0 ? active : statuses;
  let bestColor: string = CHANGE_REQUEST_STATUS_COLORS.muted;
  let bestRank = -1;
  for (const status of target) {
    const color = getReviewTaskStatusColor(status);
    const rank = CHANGE_REQUEST_STATUS_RANK[color] ?? 0;
    if (rank > bestRank) {
      bestRank = rank;
      bestColor = color;
    }
  }
  return bestColor;
}
