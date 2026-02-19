"use client";

import { useState, useCallback, useEffect, useRef, useMemo } from "react";
import { useAppStore } from "@/components/state-provider";
import { useOpenSessionInEditor } from "@/hooks/use-open-session-in-editor";
import { useSessionGitStatus } from "@/hooks/domains/session/use-session-git-status";
import { useSessionCommits } from "@/hooks/domains/session/use-session-commits";
import { useGitOperations } from "@/hooks/use-git-operations";
import { useSessionFileReviews } from "@/hooks/use-session-file-reviews";
import { useCumulativeDiff } from "@/hooks/domains/session/use-cumulative-diff";
import { hashDiff, normalizeDiffContent } from "@/components/review/types";
import { getWebSocketClient } from "@/lib/ws/connection";
import type { FileInfo } from "@/lib/state/store";
import { useToast } from "@/components/toast-provider";
import { useFileOperations } from "@/hooks/use-file-operations";
import type { OpenFileTab } from "@/lib/types/backend";
import {
  getFilesPanelTab,
  setFilesPanelTab,
  hasUserSelectedFilesPanelTab,
  setUserSelectedFilesPanelTab,
} from "@/lib/local-storage";
import { usePanelActions } from "@/hooks/use-panel-actions";

export type GitStatusFiles = Record<
  string,
  {
    diff?: string;
    additions?: number;
    deletions?: number;
    staged?: boolean;
    status?: string;
    old_path?: string;
  }
>;
export type CumulativeDiffFiles = Record<string, { diff?: string }>;
export type ReviewMap = Map<string, { reviewed?: boolean; diffHash?: string }>;

function collectDiffPaths(
  gitStatusFiles: GitStatusFiles | undefined,
  cumulativeFiles: CumulativeDiffFiles | undefined,
): Set<string> {
  const paths = new Set<string>();
  if (gitStatusFiles) {
    for (const [path, file] of Object.entries(gitStatusFiles)) {
      if (file.diff && normalizeDiffContent(file.diff)) paths.add(path);
    }
  }
  if (cumulativeFiles) {
    for (const [path, file] of Object.entries(cumulativeFiles)) {
      if (!paths.has(path) && file.diff && normalizeDiffContent(file.diff)) paths.add(path);
    }
  }
  return paths;
}

function getDiffContentForPath(
  path: string,
  gitStatusFiles: GitStatusFiles | undefined,
  cumulativeFiles: CumulativeDiffFiles | undefined,
): string {
  const uncommittedFile = gitStatusFiles?.[path];
  if (uncommittedFile?.diff) return normalizeDiffContent(uncommittedFile.diff);
  const cumulativeFile = cumulativeFiles?.[path];
  if (cumulativeFile?.diff) return normalizeDiffContent(cumulativeFile.diff);
  return "";
}

export function computeReviewProgress(
  gitStatusFiles: GitStatusFiles | undefined,
  cumulativeFiles: CumulativeDiffFiles | undefined,
  reviews: ReviewMap,
): { reviewedCount: number; totalFileCount: number } {
  const paths = collectDiffPaths(gitStatusFiles, cumulativeFiles);
  let reviewed = 0;
  for (const path of paths) {
    const state = reviews.get(path);
    if (!state?.reviewed) continue;
    const diffContent = getDiffContentForPath(path, gitStatusFiles, cumulativeFiles);
    if (diffContent && state.diffHash && state.diffHash !== hashDiff(diffContent)) continue;
    reviewed++;
  }
  return { reviewedCount: reviewed, totalFileCount: paths.size };
}

export function useCommitDiffs(activeSessionId: string | null | undefined) {
  const [expandedCommit, setExpandedCommit] = useState<string | null>(null);
  const [commitDiffs, setCommitDiffs] = useState<Record<string, Record<string, FileInfo>>>({});
  const [loadingCommitSha, setLoadingCommitSha] = useState<string | null>(null);
  const { toast } = useToast();

  const fetchCommitDiff = useCallback(
    async (commitSha: string) => {
      if (!activeSessionId || commitDiffs[commitSha]) return;
      setLoadingCommitSha(commitSha);
      try {
        const ws = getWebSocketClient();
        if (!ws) return;
        const response = await ws.request<{ success: boolean; files: Record<string, FileInfo> }>(
          "session.commit_diff",
          { session_id: activeSessionId, commit_sha: commitSha },
          10000,
        );
        if (response?.success && response.files)
          setCommitDiffs((prev) => ({ ...prev, [commitSha]: response.files }));
      } catch (err) {
        toast({
          title: "Failed to load commit diff",
          description: err instanceof Error ? err.message : "An unexpected error occurred",
          variant: "error",
        });
      } finally {
        setLoadingCommitSha(null);
      }
    },
    [activeSessionId, commitDiffs, toast],
  );

  const toggleCommit = useCallback(
    (commitSha: string) => {
      if (expandedCommit === commitSha) {
        setExpandedCommit(null);
      } else {
        setExpandedCommit(commitSha);
        void fetchCommitDiff(commitSha);
      }
    },
    [expandedCommit, fetchCommitDiff],
  );

  return { expandedCommit, commitDiffs, loadingCommitSha, toggleCommit };
}

export function useGitStagingActions(
  activeSessionId: string | null | undefined,
  changedFiles: unknown[],
) {
  const gitOps = useGitOperations(activeSessionId ?? null);
  const [pendingStageFiles, setPendingStageFiles] = useState<Set<string>>(new Set());

  useEffect(() => {
    if (pendingStageFiles.size > 0) setPendingStageFiles(new Set());
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [changedFiles]);

  const handleStage = useCallback(
    async (path: string) => {
      setPendingStageFiles((prev) => new Set(prev).add(path));
      try {
        await gitOps.stage([path]);
      } catch {
        setPendingStageFiles((prev) => {
          const next = new Set(prev);
          next.delete(path);
          return next;
        });
      }
    },
    [gitOps],
  );

  const handleUnstage = useCallback(
    async (path: string) => {
      setPendingStageFiles((prev) => new Set(prev).add(path));
      try {
        await gitOps.unstage([path]);
      } catch {
        setPendingStageFiles((prev) => {
          const next = new Set(prev);
          next.delete(path);
          return next;
        });
      }
    },
    [gitOps],
  );

  return { pendingStageFiles, handleStage, handleUnstage };
}

export function useFilesPanelTab(
  activeSessionId: string | null | undefined,
  changedFilesLength: number,
  commitsLength: number,
) {
  const [topTab, setTopTab] = useState<"diff" | "files">("diff");
  const hasInitializedTabRef = useRef<string | null>(null);
  const userClickedFilesTabRef = useRef(false);
  const prevChangedCountRef = useRef(0);

  useEffect(() => {
    if (!activeSessionId) return;
    if (hasInitializedTabRef.current === activeSessionId) return;
    hasInitializedTabRef.current = activeSessionId;
    if (hasUserSelectedFilesPanelTab(activeSessionId)) {
      const savedTab = getFilesPanelTab(activeSessionId, "diff");
      userClickedFilesTabRef.current = savedTab === "files";
      queueMicrotask(() => setTopTab(savedTab));
      return;
    }
    userClickedFilesTabRef.current = false;
    const nextTab = changedFilesLength > 0 || commitsLength > 0 ? "diff" : "files";
    prevChangedCountRef.current = changedFilesLength;
    queueMicrotask(() => setTopTab(nextTab));
  }, [activeSessionId, changedFilesLength, commitsLength]);

  useEffect(() => {
    const prev = prevChangedCountRef.current;
    prevChangedCountRef.current = changedFilesLength;
    if (changedFilesLength !== prev) {
      if (topTab === "files" && userClickedFilesTabRef.current) return;
      queueMicrotask(() => setTopTab("diff"));
    }
  }, [changedFilesLength, topTab]);

  const handleTabChange = useCallback(
    (tab: "diff" | "files") => {
      setTopTab(tab);
      userClickedFilesTabRef.current = tab === "files";
      if (activeSessionId) {
        setFilesPanelTab(activeSessionId, tab);
        setUserSelectedFilesPanelTab(activeSessionId);
      }
    },
    [activeSessionId],
  );

  return { topTab, handleTabChange };
}

export function useDiscardDialog(activeSessionId: string | null | undefined) {
  const [showDiscardDialog, setShowDiscardDialog] = useState(false);
  const [fileToDiscard, setFileToDiscard] = useState<string | null>(null);
  const gitOps = useGitOperations(activeSessionId ?? null);
  const { toast } = useToast();

  const handleDiscardClick = useCallback((filePath: string) => {
    setFileToDiscard(filePath);
    setShowDiscardDialog(true);
  }, []);

  const handleDiscardConfirm = useCallback(async () => {
    if (!fileToDiscard) return;
    try {
      const result = await gitOps.discard([fileToDiscard]);
      if (!result.success)
        toast({
          title: "Failed to discard changes",
          description: result.error || "An unknown error occurred",
          variant: "error",
        });
    } catch (error) {
      toast({
        title: "Failed to discard changes",
        description: error instanceof Error ? error.message : "An unknown error occurred",
        variant: "error",
      });
    } finally {
      setShowDiscardDialog(false);
      setFileToDiscard(null);
    }
  }, [fileToDiscard, gitOps, toast]);

  return {
    showDiscardDialog,
    setShowDiscardDialog,
    fileToDiscard,
    handleDiscardClick,
    handleDiscardConfirm,
  };
}

export function useFilesPanelData(onOpenFile: (file: OpenFileTab) => void) {
  const activeSessionId = useAppStore((state) => state.tasks.activeSessionId);
  const gitStatus = useSessionGitStatus(activeSessionId);
  const { commits } = useSessionCommits(activeSessionId ?? null);
  const openEditor = useOpenSessionInEditor(activeSessionId ?? null);
  const { createFile: baseCreateFile, deleteFile: hookDeleteFile } = useFileOperations(
    activeSessionId ?? null,
  );
  const { reviews } = useSessionFileReviews(activeSessionId);
  const { diff: cumulativeDiff } = useCumulativeDiff(activeSessionId);
  const { openFile: panelOpenFile } = usePanelActions();

  const changedFiles = useMemo(() => {
    if (!gitStatus?.files || Object.keys(gitStatus.files).length === 0) return [];
    return (Object.values(gitStatus.files) as FileInfo[]).map((file: FileInfo) => ({
      path: file.path,
      status: file.status,
      staged: file.staged,
      plus: file.additions,
      minus: file.deletions,
      oldPath: file.old_path,
    }));
  }, [gitStatus]);

  const { reviewedCount, totalFileCount } = useMemo(
    () =>
      computeReviewProgress(
        gitStatus?.files as GitStatusFiles | undefined,
        cumulativeDiff?.files as CumulativeDiffFiles | undefined,
        reviews as ReviewMap,
      ),
    [gitStatus, cumulativeDiff, reviews],
  );

  const handleCreateFile = useCallback(
    async (path: string): Promise<boolean> => {
      const ok = await baseCreateFile(path);
      if (ok) {
        const name = path.split("/").pop() || path;
        const { calculateHash } = await import("@/lib/utils/file-diff");
        const hash = await calculateHash("");
        onOpenFile({
          path,
          name,
          content: "",
          originalContent: "",
          originalHash: hash,
          isDirty: false,
        });
      }
      return ok;
    },
    [baseCreateFile, onOpenFile],
  );

  const handleOpenFileInDocumentPanel = useCallback(
    (path: string) => {
      if (activeSessionId) panelOpenFile(path);
    },
    [activeSessionId, panelOpenFile],
  );

  const handleOpenInEditor = useCallback(
    (path: string) => {
      if (activeSessionId) openEditor.open({ filePath: path });
    },
    [activeSessionId, openEditor],
  );

  return {
    activeSessionId,
    gitStatus,
    commits,
    changedFiles,
    reviewedCount,
    totalFileCount,
    hookDeleteFile,
    handleCreateFile,
    handleOpenFileInDocumentPanel,
    handleOpenInEditor,
  };
}
