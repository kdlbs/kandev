"use client";

import { useCallback, useEffect, useState } from "react";
import { useAppStore } from "@/components/state-provider";
import { useSessionGitStatus } from "@/hooks/domains/session/use-session-git-status";
import { useCumulativeDiff } from "@/hooks/domains/session/use-cumulative-diff";
import { useFileEditors } from "@/hooks/use-file-editors";
import { useActiveTaskPR } from "@/hooks/domains/github/use-task-pr";
import { usePRDiff } from "@/hooks/domains/github/use-pr-diff";
import { formatReviewCommentsAsMarkdown } from "@/components/task/chat/messages/review-comments-attachment";
import { getWebSocketClient } from "@/lib/ws/connection";
import { useToast } from "@/components/toast-provider";
import type { DiffComment } from "@/lib/diff/types";

export function useReviewDialog(effectiveSessionId: string | null) {
  const [reviewDialogOpen, setReviewDialogOpen] = useState(false);
  const { toast } = useToast();
  const activeTaskId = useAppStore((state) => state.tasks.activeTaskId);
  const baseBranch = useAppStore((state) => {
    if (!effectiveSessionId) return undefined;
    return state.taskSessions.items[effectiveSessionId]?.base_branch;
  });
  const reviewGitStatus = useSessionGitStatus(effectiveSessionId);
  const { diff: reviewCumulativeDiff } = useCumulativeDiff(effectiveSessionId);
  const { openFile: reviewOpenFile } = useFileEditors();
  const reviewTaskPR = useActiveTaskPR();
  const { files: reviewPRDiffFiles } = usePRDiff(
    reviewTaskPR?.owner ?? null,
    reviewTaskPR?.repo ?? null,
    reviewTaskPR?.pr_number ?? null,
  );

  const handleReviewSendComments = useCallback(
    (comments: DiffComment[]) => {
      if (!activeTaskId || !effectiveSessionId || comments.length === 0) return;
      const client = getWebSocketClient();
      if (!client) return;
      const markdown = formatReviewCommentsAsMarkdown(comments);
      client
        .request(
          "message.add",
          { task_id: activeTaskId, session_id: effectiveSessionId, content: markdown },
          10000,
        )
        .catch(() => {
          toast({ title: "Failed to send comments", variant: "error" });
        });
      setReviewDialogOpen(false);
    },
    [activeTaskId, effectiveSessionId, toast],
  );

  useEffect(() => {
    const handler = () => setReviewDialogOpen(true);
    window.addEventListener("open-review-dialog", handler);
    return () => window.removeEventListener("open-review-dialog", handler);
  }, []);

  return {
    reviewDialogOpen,
    setReviewDialogOpen,
    baseBranch,
    reviewGitStatus,
    reviewCumulativeDiff,
    reviewPRDiffFiles,
    reviewOpenFile,
    handleReviewSendComments,
  };
}
