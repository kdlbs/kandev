"use client";

import { useCallback, useEffect, useMemo, useState } from "react";
import { useAppStore } from "@/components/state-provider";
import {
  useSessionGitStatus,
  useSessionGitStatusByRepo,
} from "@/hooks/domains/session/use-session-git-status";
import { useCumulativeDiff } from "@/hooks/domains/session/use-cumulative-diff";
import { useFileEditors } from "@/hooks/use-file-editors";
import { useReviewPRSelection } from "@/hooks/domains/github/use-review-pr-selection";
import { usePRDiff } from "@/hooks/domains/github/use-pr-diff";
import { usePRReviewRepositoryIdentity } from "@/hooks/domains/github/use-pr-review-repository-identity";
import { useTaskRepositories } from "@/hooks/domains/kanban/use-task-repositories";
import { formatReviewCommentsAsMarkdown } from "@/components/task/chat/messages/review-comments-attachment";
import {
  getCumulativeReviewRepositoryNames,
  isReviewMultiRepo,
  reviewFileKey,
} from "@/components/review/types";
import { getWebSocketClient } from "@/lib/ws/connection";
import { generateUUID } from "@/lib/utils";
import { useToast } from "@/components/toast-provider";
import type { DiffComment } from "@/lib/diff/types";
import type { FileInfo, GitStatusEntry } from "@/lib/state/slices/session-runtime/types";
import { normalizeGitStatusFiles } from "@/lib/state/slices/session-runtime/git-status-normalizer";
import { t } from "@/lib/i18n";

type ReviewGitStatusFiles = {
  files: Record<string, FileInfo> | null;
  isMultiRepo: boolean;
};

function buildMultiRepoReviewFiles(
  statusByRepo: Array<{ repository_name: string; status: GitStatusEntry }>,
): Record<string, FileInfo> {
  const files: Record<string, FileInfo> = {};
  for (const { repository_name, status } of statusByRepo) {
    if (!status?.files) continue;
    for (const [keyPath, file] of Object.entries(normalizeGitStatusFiles(status.files) ?? {})) {
      const path = file.path || keyPath;
      const key = reviewFileKey({ path, repository_name });
      files[key] = {
        ...file,
        repository_name,
        is_submodule: file.is_submodule ?? status.is_submodule,
      };
    }
  }
  return files;
}

export function buildReviewGitStatusFiles(
  reviewGitStatus: GitStatusEntry | undefined,
  statusByRepo: Array<{ repository_name: string; status: GitStatusEntry }>,
  taskRepositoryCount: number,
  cumulativeRepositoryNames: Iterable<string> = [],
): ReviewGitStatusFiles {
  const named = statusByRepo.filter((entry) => entry.repository_name !== "");
  const isMultiRepo = isReviewMultiRepo(
    taskRepositoryCount,
    statusByRepo
      .map((entry) => entry.repository_name)
      .concat(Array.from(cumulativeRepositoryNames)),
  );
  if (!isMultiRepo) {
    const sourceStatus = reviewGitStatus?.files ? reviewGitStatus : named[0]?.status;
    const sourceFiles = normalizeGitStatusFiles(sourceStatus?.files);
    const files = sourceFiles
      ? Object.fromEntries(
          Object.entries(sourceFiles).map(([path, file]) => [
            path,
            { ...file, is_submodule: file.is_submodule ?? sourceStatus?.is_submodule },
          ]),
        )
      : null;
    return {
      files,
      isMultiRepo: false,
    };
  }
  if (named.length === 0) {
    return {
      files: normalizeGitStatusFiles(reviewGitStatus?.files) ?? null,
      isMultiRepo: true,
    };
  }

  const files = buildMultiRepoReviewFiles(statusByRepo);
  return {
    files: Object.keys(files).length > 0 ? files : null,
    isMultiRepo: true,
  };
}

/**
 * Builds the unified gitStatus.files map fed into the ReviewDialog. Multi-repo
 * tasks have one git status per repo, and two repos can have files at the same
 * relative path (`README.md` in both), so the map key is `repo\u0000path` and
 * every FileInfo is stamped with its `repository_name`, including the explicit
 * empty scope for the real workspace root. Single-repo tasks keep the legacy
 * path-only keying.
 */
function useReviewGitStatusFiles(
  sessionId: string | null,
  taskRepositoryCount: number,
  cumulativeRepositoryNames: string[],
): ReviewGitStatusFiles {
  const reviewGitStatus = useSessionGitStatus(sessionId);
  const statusByRepo = useSessionGitStatusByRepo(sessionId);
  return useMemo(
    () =>
      buildReviewGitStatusFiles(
        reviewGitStatus,
        statusByRepo,
        taskRepositoryCount,
        cumulativeRepositoryNames,
      ),
    [reviewGitStatus, statusByRepo, taskRepositoryCount, cumulativeRepositoryNames],
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
  const { diff: reviewCumulativeDiff } = useCumulativeDiff(effectiveSessionId);
  const cumulativeRepositoryNames = useMemo(
    () => getCumulativeReviewRepositoryNames(reviewCumulativeDiff?.files),
    [reviewCumulativeDiff],
  );
  const reviewGitStatus = useReviewGitStatusFiles(
    effectiveSessionId,
    taskRepositories.length,
    cumulativeRepositoryNames,
  );
  const { openFile: reviewOpenFile } = useFileEditors();
  const {
    prs: reviewPRs,
    selectedPR: reviewTaskPR,
    selectedKey: reviewSelectedPRKey,
    selectPR: reviewSelectPR,
  } = useReviewPRSelection(activeTaskId);
  const reviewPRRepositoryName = usePRReviewRepositoryIdentity(
    activeTaskId,
    effectiveSessionId,
    reviewTaskPR,
  );
  const reviewPRDiff = usePRDiff(
    reviewTaskPR?.owner ?? null,
    reviewTaskPR?.repo ?? null,
    reviewTaskPR?.pr_number ?? null,
    reviewTaskPR?.last_synced_at ?? null,
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
          {
            task_id: activeTaskId,
            session_id: effectiveSessionId,
            client_message_id: generateUUID(),
            content: markdown,
          },
          10000,
        )
        .catch(() => {
          toast({ title: t("task:failedToSendComments"), variant: "error" });
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
    reviewPRs,
    reviewSelectedPR: reviewTaskPR,
    reviewSelectedPRKey,
    reviewSelectPR,
    reviewPRDiffFiles: reviewPRDiff.files,
    reviewPRDiffLoading: reviewPRDiff.loading,
    reviewPRDiffError: reviewPRDiff.error,
    reviewRefreshPRDiff: reviewPRDiff.refresh,
    reviewPRRepoName: reviewGitStatus.isMultiRepo ? reviewPRRepositoryName : undefined,
    reviewUseRepositoryKeys: reviewGitStatus.isMultiRepo,
    reviewOpenFile,
    handleReviewSendComments,
  };
}
