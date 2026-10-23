"use client";

import { TaskReviewDialogMount } from "../dockview-review-dialog";

type SessionMobileReviewDialogProps = {
  sessionId: string | null;
  taskId: string | null;
  onOpenFile: (path: string, repo?: string, preview?: boolean) => void;
  onOpenWalkthroughFile?: (path: string, repo?: string) => void;
};

export function SessionMobileReviewDialog({
  sessionId,
  taskId,
  onOpenFile,
  onOpenWalkthroughFile,
}: SessionMobileReviewDialogProps) {
  return (
    <TaskReviewDialogMount
      sessionId={sessionId}
      taskId={taskId}
      onOpenFile={onOpenFile}
      onSelectWalkthroughFile={onOpenWalkthroughFile ?? onOpenFile}
    />
  );
}
