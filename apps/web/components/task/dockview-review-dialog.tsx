"use client";

import { ReviewDialog } from "@/components/review/review-dialog";
import type { useReviewDialog } from "./use-review-dialog";

type DockviewReviewDialogProps = {
  sessionId: string | null;
  review: ReturnType<typeof useReviewDialog>;
};

export function DockviewReviewDialog({ sessionId, review }: DockviewReviewDialogProps) {
  if (!sessionId) {
    return null;
  }

  return (
    <ReviewDialog
      open={review.reviewDialogOpen}
      onOpenChange={review.setReviewDialogOpen}
      sessionId={sessionId}
      baseBranch={review.baseBranch}
      onSendComments={review.handleReviewSendComments}
      onOpenFile={review.reviewOpenFile}
      gitStatusFiles={review.reviewGitStatusFiles}
      cumulativeDiff={review.reviewCumulativeDiff}
      prDiffFiles={review.reviewPRDiffFiles}
      prRepoName={review.reviewPRRepoName}
    />
  );
}
