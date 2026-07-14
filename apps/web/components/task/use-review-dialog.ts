"use client";

import { useCallback, useEffect, useMemo, useState } from "react";
import { useAppStore } from "@/components/state-provider";
import {
  useSessionGitStatus,
  useSessionGitStatusByRepo,
} from "@/hooks/domains/session/use-session-git-status";
import { useCumulativeDiff } from "@/hooks/domains/session/use-cumulative-diff";
import { useFileEditors } from "@/hooks/use-file-editors";
import { useActiveTaskPR } from "@/hooks/domains/github/use-task-pr";
import { usePRDiff } from "@/hooks/domains/github/use-pr-diff";
import { useTaskRepositories } from "@/hooks/domains/kanban/use-task-repositories";
import { formatReviewCommentsAsMarkdown } from "@/components/task/chat/messages/review-comments-attachment";
import { getWebSocketClient } from "@/lib/ws/connection";
import { useToast } from "@/components/toast-provider";
import type { DiffComment } from "@/lib/diff/types";
import type { FileInfo, GitStatusEntry } from "@/lib/state/slices/session-runtime/types";

type ReviewGitStatusFiles = {
  files: Record<string, FileInfo> | null;
  isMultiRepo: boolean;
};

export function buildReviewGitStatusFiles(
  reviewGitStatus: GitStatusEntry | undefined,
  statusByRepo: Array<{ repository_name: string; status: GitStatusEntry }>,
  taskRepositoryCount: number,
): ReviewGitStatusFiles {
  const named = statusByRepo.filter((entry) => entry.repository_name !== "");
  const isMultiRepo = taskRepositoryCount > 1 || named.length > 1;
  if (!isMultiRepo) {
    return {
      files: reviewGitStatus?.files ?? named[0]?.status.files ?? null,
      isMultiRepo: false,
    };
  }

  const files: Record<string, FileInfo> = {};
  for (const { repository_name, status } of named) {
    if (!status?.files) continue;
    for (const [path, file] of Object.entries(status.files)) {
      files[`${repository_name}\u0000${path}`] = { ...file, repository_name };
    }
  }
  return {
    files: Object.keys(files).length > 0 ? files : null,
    isMultiRepo: true,
  };
}

/**
 * Builds the unified gitStatus.files map fed into the ReviewDialog. Multi-repo
 * tasks have one git status per repo, and two repos can have files at the same
 * relative path (`README.md` in both), so the map key is `repo\u0000path` and
 * every FileInfo is stamped with its `repository_name`. Single-repo tasks keep
 * the legacy path-only keying.
 */
function useReviewGitStatusFiles(
  sessionId: string | null,
  taskRepositoryCount: number,
): ReviewGitStatusFiles {
  const reviewGitStatus = useSessionGitStatus(sessionId);
  const statusByRepo = useSessionGitStatusByRepo(sessionId);
  return useMemo(
    () => buildReviewGitStatusFiles(reviewGitStatus, statusByRepo, taskRepositoryCount),
    [reviewGitStatus, statusByRepo, taskRepositoryCount],
  );
}

export function useReviewDialog(effectiveSessionId: string | null) {
  const [reviewDialogOpen, setReviewDialogOpen] = useState(false);
  const { toast } = useToast();
  const activeTaskId = useAppStore((state) => state.tasks.activeTaskId);
  const taskRepositories = useTaskRepositories(activeTaskId);
  const baseBranch = useAppStore((state) => {
    if (!effectiveSessionId) return undefined;
    return state.taskSessions.items[effectiveSessionId]?.base_branch;
  });
  const reviewGitStatus = useReviewGitStatusFiles(effectiveSessionId, taskRepositories.length);
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
    reviewGitStatusFiles: reviewGitStatus.files,
    reviewCumulativeDiff,
    reviewPRDiffFiles,
    reviewPRRepoName: reviewGitStatus.isMultiRepo ? reviewTaskPR?.repo : undefined,
    reviewOpenFile,
    handleReviewSendComments,
  };
}
