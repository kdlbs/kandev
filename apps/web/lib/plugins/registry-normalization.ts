import type {
  ReviewItemSummary,
  ReviewTaskAssociation,
  ReviewTaskPipelineState,
  ReviewTaskStatus,
} from "./types";

export function taskActionKey(pluginId: string, actionId: string): string {
  return `${pluginId}:${actionId}`;
}

export function normalizeReviewItems(
  providerId: string,
  items: readonly ReviewItemSummary[],
): readonly ReviewItemSummary[] {
  return items.flatMap((item) => {
    if (
      item.providerId !== providerId ||
      !item.reviewKey ||
      !item.title ||
      !item.url ||
      !item.repositoryId ||
      !item.state
    ) {
      return [];
    }
    const statusBadge = item.statusBadge?.label
      ? {
          label: item.statusBadge.label,
          ...(item.statusBadge.tone ? { tone: item.statusBadge.tone } : {}),
        }
      : undefined;
    const taskStatus = normalizeReviewTaskStatus(item.taskStatus);
    return [
      {
        providerId,
        reviewKey: item.reviewKey,
        title: item.title,
        url: item.url,
        repositoryId: item.repositoryId,
        state: item.state,
        ...(statusBadge ? { statusBadge } : {}),
        ...(taskStatus ? { taskStatus } : {}),
      },
    ];
  });
}

export function normalizeReviewAssociations(
  providerId: string,
  associations: readonly ReviewTaskAssociation[],
): readonly ReviewTaskAssociation[] {
  const seen = new Set<string>();
  return associations.flatMap((association) => {
    if (!association.taskId || !association.reviewKey) return [];
    const repositoryId = association.repositoryId?.trim();
    const changeRequestNumber = normalizeChangeRequestNumber(association.changeRequestNumber);
    const hasImmutableIdentity = Boolean(repositoryId) && changeRequestNumber !== undefined;
    const identity = hasImmutableIdentity
      ? `${repositoryId}\u0000${String(changeRequestNumber)}`
      : association.reviewKey;
    const key = `${association.taskId}\u0000${identity}`;
    if (seen.has(key)) return [];
    seen.add(key);
    return [
      {
        providerId,
        taskId: association.taskId,
        reviewKey: association.reviewKey,
        ...(hasImmutableIdentity
          ? {
              repositoryId,
              changeRequestNumber,
            }
          : {}),
      },
    ];
  });
}

function normalizeChangeRequestNumber(value: string | number | undefined) {
  if (typeof value === "number") return Number.isFinite(value) ? value : undefined;
  if (typeof value !== "string") return undefined;
  return value.trim() || undefined;
}

const REVIEW_TASK_STATES = new Set<ReviewTaskStatus["state"]>([
  "open",
  "merged",
  "closed",
  "draft",
]);
const REVIEW_PIPELINE_STATES = new Set<ReviewTaskPipelineState>([
  "success",
  "failure",
  "pending",
  "neutral",
]);

function normalizeReviewTaskStatus(status: ReviewTaskStatus | undefined): ReviewTaskStatus | null {
  if (
    !status ||
    (typeof status.number !== "string" && typeof status.number !== "number") ||
    !REVIEW_TASK_STATES.has(status.state) ||
    !REVIEW_PIPELINE_STATES.has(status.pipelineState) ||
    !Array.isArray(status.checks)
  ) {
    return null;
  }
  const checks = status.checks.flatMap((check) => {
    if (!check.id || !check.label || !REVIEW_PIPELINE_STATES.has(check.state)) return [];
    return [
      {
        id: check.id,
        label: check.label,
        state: check.state,
        ...(check.detail ? { detail: check.detail } : {}),
        ...(check.url ? { url: check.url } : {}),
      },
    ];
  });
  const review = normalizeReviewTaskReview(status.review);
  return {
    number: status.number,
    state: status.state,
    pipelineState: status.pipelineState,
    checks,
    ...(review ? { review } : {}),
    ...(typeof status.unresolvedComments === "number" && status.unresolvedComments >= 0
      ? { unresolvedComments: status.unresolvedComments }
      : {}),
    ...(status.loading === true ? { loading: true } : {}),
    ...(status.error ? { error: status.error } : {}),
    ...(typeof status.updatedAt === "number" ? { updatedAt: status.updatedAt } : {}),
  };
}

function normalizeReviewTaskReview(review: ReviewTaskStatus["review"]) {
  if (
    !review ||
    !["approved", "changes_requested", "pending"].includes(review.state) ||
    !Number.isFinite(review.approved) ||
    review.approved < 0
  ) {
    return null;
  }
  return {
    state: review.state,
    approved: review.approved,
    ...(Number.isFinite(review.required) && (review.required ?? -1) >= 0
      ? { required: review.required }
      : {}),
    ...(Number.isFinite(review.requested) && (review.requested ?? -1) >= 0
      ? { requested: review.requested }
      : {}),
  };
}

export function pluginSlotOrderingId(pluginId: string, slot: string, ordinal: number): string {
  return `plugin:${encodeURIComponent(pluginId)}:${encodeURIComponent(slot)}:${ordinal}`;
}
