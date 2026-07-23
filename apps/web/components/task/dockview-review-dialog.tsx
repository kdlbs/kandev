"use client";

import { ReviewDialog } from "@/components/review/review-dialog";
import { WalkthroughOverlay } from "@/components/review/walkthrough-overlay";
import { useReviewDialog } from "./use-review-dialog";

type TaskReviewDialogMountProps = {
  sessionId: string | null;
  taskId: string | null;
  onSelectWalkthroughFile?: (path: string, repo?: string) => void;
};

export function TaskReviewDialogMount({
  sessionId,
  taskId,
  onSelectWalkthroughFile,
}: TaskReviewDialogMountProps) {
  const review = useReviewDialog(sessionId);

  if (!sessionId) {
    return null;
  }

  return (
    <>
      <ReviewDialog
        open={review.reviewDialogOpen}
        onOpenChange={review.setReviewDialogOpen}
        sessionId={sessionId}
        baseBranch={review.baseBranch}
        onSendComments={review.handleReviewSendComments}
        onOpenFile={review.reviewOpenFile}
        gitStatusFiles={review.reviewGitStatusFiles}
        cumulativeDiff={review.reviewCumulativeDiff}
        prs={review.reviewPRs}
        selectedPR={review.reviewSelectedPR}
        selectedPRKey={review.reviewSelectedPRKey}
        onSelectPR={review.reviewSelectPR}
        prDiffFiles={review.reviewPRDiffFiles}
        prDiffLoading={review.reviewPRDiffLoading}
        prDiffError={review.reviewPRDiffError}
        onRetryPRDiff={review.reviewRefreshPRDiff}
        prRepoName={review.reviewPRRepoName}
        useRepositoryKeys={review.reviewUseRepositoryKeys}
      />
      <WalkthroughOverlay
        taskId={taskId}
        sessionId={sessionId}
        onSelectFile={onSelectWalkthroughFile ?? review.reviewOpenFile}
      />
    </>
  );
}
