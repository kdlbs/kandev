import type { ReviewItemSummary } from "@/lib/plugins/types";

export type ReviewPanelTarget = Pick<
  ReviewItemSummary,
  "providerId" | "reviewKey" | "connectionScope" | "repositoryId" | "changeRequestNumber" | "title"
>;

export function reviewPanelId(
  review: Pick<
    ReviewItemSummary,
    "providerId" | "connectionScope" | "repositoryId" | "changeRequestNumber"
  >,
): string {
  return `review-detail|${[
    review.providerId,
    review.connectionScope,
    review.repositoryId,
    String(review.changeRequestNumber),
  ]
    .map(encodeURIComponent)
    .join("|")}`;
}
