"use client";

import { TaskReviewDialogMount } from "../dockview-review-dialog";

type SessionMobileReviewDialogProps = {
  sessionId: string | null;
  taskId: string | null;
  onOpenFile: (path: string, repo?: string, preview?: boolean) => void;
};

export function SessionMobileReviewDialog({
  sessionId,
  taskId,
  onOpenFile,
}: SessionMobileReviewDialogProps) {
  return (
    <TaskReviewDialogMount
      sessionId={sessionId}
      taskId={taskId}
      onOpenFile={onOpenFile}
      onPreviewMarkdown={(path, repo) => onOpenFile(path, repo, true)}
      onSelectWalkthroughFile={onOpenFile}
    />
  );
}
