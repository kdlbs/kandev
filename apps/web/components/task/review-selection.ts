"use client";

import { useCallback, useState } from "react";
import type { ReviewItemSummary } from "@/lib/plugins/types";

type ReviewSelection = {
  taskId: string | null;
  reviewId: string | null;
  preferredReviewId: string | null;
};

export function reviewItemId(review: Pick<ReviewItemSummary, "providerId" | "reviewKey">): string {
  return `${encodeURIComponent(review.providerId)}:${encodeURIComponent(review.reviewKey)}`;
}

/**
 * A provider owns the ordering of its reviews, so its first item remains the
 * default when it is the only provider present. Mixed-provider results require
 * an explicit choice so a newly registered provider cannot be hidden behind a
 * built-in result.
 */
export function selectReviewItem(
  reviews: readonly ReviewItemSummary[],
  selectedReviewId: string | null,
): ReviewItemSummary | null {
  if (selectedReviewId) {
    const selected = reviews.find((review) => reviewItemId(review) === selectedReviewId);
    if (selected) return selected;
  }
  const primary = reviews[0];
  if (primary && reviews.every((review) => review.providerId === primary.providerId)) {
    return primary;
  }
  return null;
}

export function useReviewItemSelection(
  taskId: string | null,
  reviews: readonly ReviewItemSummary[],
  preferredReviewId: string | null = null,
) {
  const [selection, setSelection] = useState<ReviewSelection>({
    taskId,
    reviewId: preferredReviewId,
    preferredReviewId,
  });
  if (selection.taskId !== taskId || selection.preferredReviewId !== preferredReviewId) {
    setSelection({ taskId, reviewId: preferredReviewId, preferredReviewId });
  }

  const selectedReview = selectReviewItem(reviews, selection.reviewId);
  const selectReview = useCallback(
    (review: ReviewItemSummary) => {
      setSelection({ taskId, reviewId: reviewItemId(review), preferredReviewId });
    },
    [preferredReviewId, taskId],
  );
  return { selectedReview, selectReview };
}
