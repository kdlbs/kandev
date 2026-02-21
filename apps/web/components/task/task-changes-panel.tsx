"use client";

import { memo, useMemo, useCallback, createRef, useState, useEffect, useRef } from "react";
import { PanelRoot, PanelBody } from "./panel-primitives";
import { useToast } from "@/components/toast-provider";
import { useAppStore } from "@/components/state-provider";
import { useSessionGitStatus } from "@/hooks/domains/session/use-session-git-status";
import { useCumulativeDiff } from "@/hooks/domains/session/use-cumulative-diff";
import { useGitOperations } from "@/hooks/use-git-operations";
import { useSessionFileReviews } from "@/hooks/use-session-file-reviews";
import { useCommentsStore, isDiffComment } from "@/lib/state/slices/comments";
import { getWebSocketClient } from "@/lib/ws/connection";
import { updateUserSettings } from "@/lib/api";
import { formatReviewCommentsAsMarkdown } from "@/lib/state/slices/comments/format";
import { ReviewDiffList } from "@/components/review/review-diff-list";
import type { ReviewFile } from "@/components/review/types";
import { hashDiff, normalizeDiffContent } from "@/components/review/types";
import { usePanelActions } from "@/hooks/use-panel-actions";
import { ChangesTopBar } from "./changes-top-bar";
import type { SelectedDiff } from "./task-layout";
import { useIsTaskArchived, ArchivedPanelPlaceholder } from "./task-archived-context";

type TaskChangesPanelProps = {
  mode?: "all" | "file";
  filePath?: string;
  selectedDiff: SelectedDiff | null;
  onClearSelected: () => void;
  onBecameEmpty?: () => void;
  /** Callback to open file in editor */
  onOpenFile?: (filePath: string) => void;
};

type UncommittedFile = {
  diff?: string;
  status?: string;
  additions?: number;
  deletions?: number;
  staged?: boolean;
};
type CumulativeFile = { diff?: string; status?: string; additions?: number; deletions?: number };

function addUncommittedFiles(
  fileMap: Map<string, ReviewFile>,
  files: Record<string, UncommittedFile>,
) {
  for (const [path, file] of Object.entries(files)) {
    const diff = file.diff ? normalizeDiffContent(file.diff) : "";
    if (diff) {
      fileMap.set(path, {
        path,
        diff,
        status: file.status ?? "modified",
        additions: file.additions ?? 0,
        deletions: file.deletions ?? 0,
        staged: file.staged ?? false,
        source: "uncommitted",
      });
    }
  }
}

function addCumulativeFiles(
  fileMap: Map<string, ReviewFile>,
  files: Record<string, CumulativeFile>,
) {
  for (const [path, file] of Object.entries(files)) {
    if (fileMap.has(path)) continue;
    const diff = file.diff ? normalizeDiffContent(file.diff) : "";
    if (diff) {
      fileMap.set(path, {
        path,
        diff,
        status: file.status || "modified",
        additions: file.additions ?? 0,
        deletions: file.deletions ?? 0,
        staged: false,
        source: "committed",
      });
    }
  }
}

/** Merge uncommitted + committed files into a single sorted list */
function mergeReviewFiles(
  gitStatus: ReturnType<typeof useSessionGitStatus>,
  cumulativeDiff: { files?: Record<string, CumulativeFile> } | null,
): ReviewFile[] {
  const fileMap = new Map<string, ReviewFile>();
  if (gitStatus?.files)
    addUncommittedFiles(fileMap, gitStatus.files as Record<string, UncommittedFile>);
  if (cumulativeDiff?.files) addCumulativeFiles(fileMap, cumulativeDiff.files);
  return Array.from(fileMap.values()).sort((a, b) => a.path.localeCompare(b.path));
}

function useChangesData(selectedDiff: SelectedDiff | null, onClearSelected: () => void) {
  const activeSessionId = useAppStore((state) => state.tasks.activeSessionId);
  const gitStatus = useSessionGitStatus(activeSessionId);
  const { diff: cumulativeDiff, loading: cumulativeLoading } = useCumulativeDiff(activeSessionId);
  const { reviews } = useSessionFileReviews(activeSessionId);
  const byId = useCommentsStore((s) => s.byId);
  const commentSessionIds = useCommentsStore((s) =>
    activeSessionId ? s.bySession[activeSessionId] : undefined,
  );

  const allFiles = useMemo<ReviewFile[]>(
    () => mergeReviewFiles(gitStatus, cumulativeDiff),
    [gitStatus, cumulativeDiff],
  );

  const { reviewedFiles, staleFiles } = useMemo(() => {
    const reviewed = new Set<string>();
    const stale = new Set<string>();
    for (const file of allFiles) {
      const reviewState = reviews.get(file.path);
      if (!reviewState?.reviewed) continue;
      const currentHash = hashDiff(file.diff);
      if (reviewState.diffHash && reviewState.diffHash !== currentHash) {
        stale.add(file.path);
      } else {
        reviewed.add(file.path);
      }
    }
    return { reviewedFiles: reviewed, staleFiles: stale };
  }, [allFiles, reviews]);

  const totalCommentCount = useMemo(() => {
    if (!commentSessionIds || commentSessionIds.length === 0) return 0;
    let count = 0;
    for (const id of commentSessionIds) {
      const comment = byId[id];
      if (comment && isDiffComment(comment)) count++;
    }
    return count;
  }, [byId, commentSessionIds]);

  const fileRefs = useMemo(() => {
    const refs = new Map<string, React.RefObject<HTMLDivElement | null>>();
    for (const file of allFiles) refs.set(file.path, createRef<HTMLDivElement>());
    return refs;
  }, [allFiles]);

  const scrolledRef = useRef<string | null>(null);
  useEffect(() => {
    if (!selectedDiff?.path || scrolledRef.current === selectedDiff.path) return;
    scrolledRef.current = selectedDiff.path;
    const ref = fileRefs.get(selectedDiff.path);
    if (ref?.current)
      requestAnimationFrame(() => {
        ref.current?.scrollIntoView({ behavior: "smooth", block: "start" });
      });
    onClearSelected();
  }, [selectedDiff, fileRefs, onClearSelected]);

  useEffect(() => {
    if (!selectedDiff) scrolledRef.current = null;
  }, [selectedDiff]);

  return {
    activeSessionId,
    allFiles,
    reviewedFiles,
    staleFiles,
    totalCommentCount,
    fileRefs,
    cumulativeLoading,
  };
}

function persistAutoMarkSetting(checked: boolean) {
  const client = getWebSocketClient();
  const payload = { review_auto_mark_on_scroll: checked };
  if (client) {
    client.request("user.settings.update", payload).catch(() => {
      updateUserSettings(payload, { cache: "no-store" }).catch(() => {});
    });
    return;
  }
  updateUserSettings(payload, { cache: "no-store" }).catch(() => {});
}

function useChangesActions(activeSessionId: string | null | undefined, allFiles: ReviewFile[]) {
  const activeTaskId = useAppStore((state) => state.tasks.activeTaskId);
  const autoMarkOnScroll = useAppStore((s) => s.userSettings.reviewAutoMarkOnScroll);
  const setUserSettings = useAppStore((state) => state.setUserSettings);
  const userSettings = useAppStore((state) => state.userSettings);
  const { discard } = useGitOperations(activeSessionId ?? null);
  const { markReviewed, markUnreviewed } = useSessionFileReviews(activeSessionId ?? null);
  const getPendingComments = useCommentsStore((s) => s.getPendingComments);
  const markCommentsSent = useCommentsStore((s) => s.markCommentsSent);
  const { toast } = useToast();

  const [splitView, setSplitView] = useState(
    () => typeof window !== "undefined" && localStorage.getItem("diff-view-mode") === "split",
  );
  const [wordWrap, setWordWrap] = useState(false);

  const handleToggleSplitView = useCallback((split: boolean) => {
    setSplitView(split);
    const mode = split ? "split" : "unified";
    localStorage.setItem("diff-view-mode", mode);
    window.dispatchEvent(new CustomEvent("diff-view-mode-change", { detail: mode }));
  }, []);

  const handleToggleReviewed = useCallback(
    (path: string, reviewed: boolean) => {
      if (reviewed) {
        const file = allFiles.find((f) => f.path === path);
        markReviewed(path, file ? hashDiff(file.diff) : "");
      } else {
        markUnreviewed(path);
      }
    },
    [allFiles, markReviewed, markUnreviewed],
  );

  const handleDiscard = useCallback(
    async (path: string) => {
      try {
        const result = await discard([path]);
        if (result.success) {
          toast({ title: "Changes discarded", description: path, variant: "success" });
        } else {
          toast({
            title: "Discard failed",
            description: result.error || "An error occurred",
            variant: "error",
          });
        }
      } catch (e) {
        toast({
          title: "Discard failed",
          description: e instanceof Error ? e.message : "An error occurred",
          variant: "error",
        });
      }
    },
    [discard, toast],
  );

  const handleToggleAutoMark = useCallback(
    (checked: boolean) => {
      const next = { ...userSettings, reviewAutoMarkOnScroll: checked };
      setUserSettings(next);
      persistAutoMarkSetting(checked);
    },
    [setUserSettings, userSettings],
  );

  const handleFixComments = useCallback(() => {
    if (!activeSessionId || !activeTaskId) return;
    const allPending = getPendingComments();
    const comments = allPending.filter(isDiffComment);
    if (comments.length === 0) return;
    const markdown = formatReviewCommentsAsMarkdown(comments);
    if (!markdown) return;
    const client = getWebSocketClient();
    if (client) {
      client
        .request("message.add", {
          task_id: activeTaskId,
          session_id: activeSessionId,
          content: markdown,
        })
        .catch(() => {
          toast({ title: "Failed to send comments", variant: "error" });
        });
    }
    markCommentsSent(comments.map((c) => c.id));
  }, [activeSessionId, activeTaskId, getPendingComments, markCommentsSent, toast]);

  return {
    splitView,
    wordWrap,
    setWordWrap,
    autoMarkOnScroll,
    handleToggleSplitView,
    handleToggleReviewed,
    handleDiscard,
    handleToggleAutoMark,
    handleFixComments,
  };
}

const TaskChangesPanel = memo(function TaskChangesPanel({
  mode = "all",
  filePath,
  selectedDiff,
  onClearSelected,
  onBecameEmpty,
  onOpenFile: onOpenFileProp,
}: TaskChangesPanelProps) {
  const isArchived = useIsTaskArchived();
  const { openFile: panelOpenFile } = usePanelActions();
  const handleOpenFile = onOpenFileProp ?? panelOpenFile;

  const {
    activeSessionId,
    allFiles,
    reviewedFiles,
    staleFiles,
    totalCommentCount,
    fileRefs,
    cumulativeLoading,
  } = useChangesData(selectedDiff, onClearSelected);
  const visibleFiles = useMemo(() => {
    if (mode === "file" && filePath) {
      return allFiles.filter((file) => file.path === filePath);
    }
    return allFiles;
  }, [allFiles, mode, filePath]);
  const visibleFileRefs = useMemo(() => {
    if (mode !== "file" || !filePath) return fileRefs;
    const refs = new Map<string, React.RefObject<HTMLDivElement | null>>();
    const fileRef = fileRefs.get(filePath);
    if (fileRef) refs.set(filePath, fileRef);
    return refs;
  }, [mode, filePath, fileRefs]);
  const {
    splitView,
    wordWrap,
    setWordWrap,
    autoMarkOnScroll,
    handleToggleSplitView,
    handleToggleReviewed,
    handleDiscard,
    handleToggleAutoMark,
    handleFixComments,
  } = useChangesActions(activeSessionId, allFiles);

  const reviewedCount = useMemo(
    () =>
      visibleFiles.reduce((count, file) => {
        if (!staleFiles.has(file.path) && reviewedFiles.has(file.path)) return count + 1;
        return count;
      }, 0),
    [visibleFiles, reviewedFiles, staleFiles],
  );
  const totalCount = visibleFiles.length;
  const progressPercent = totalCount > 0 ? (reviewedCount / totalCount) * 100 : 0;
  const prevVisibleCountRef = useRef<number | null>(null);

  useEffect(() => {
    if (!onBecameEmpty) return;
    const prevCount = prevVisibleCountRef.current;
    if (prevCount !== null && prevCount > 0 && visibleFiles.length === 0) {
      onBecameEmpty();
    }
    prevVisibleCountRef.current = visibleFiles.length;
  }, [onBecameEmpty, visibleFiles.length]);

  if (isArchived) return <ArchivedPanelPlaceholder />;

  return (
    <PanelRoot>
      <ChangesTopBar
        autoMarkOnScroll={autoMarkOnScroll}
        splitView={splitView}
        wordWrap={wordWrap}
        totalCommentCount={totalCommentCount}
        reviewedCount={reviewedCount}
        totalCount={totalCount}
        progressPercent={progressPercent}
        setWordWrap={setWordWrap}
        handleToggleSplitView={handleToggleSplitView}
        handleToggleAutoMark={handleToggleAutoMark}
        handleFixComments={handleFixComments}
      />
      <PanelBody padding={false} scroll={false} className="overflow-hidden">
        <ChangesPanelContent
          isLoading={cumulativeLoading}
          files={visibleFiles}
          activeSessionId={activeSessionId}
          reviewedFiles={reviewedFiles}
          staleFiles={staleFiles}
          autoMarkOnScroll={autoMarkOnScroll}
          wordWrap={wordWrap}
          onToggleReviewed={handleToggleReviewed}
          onDiscard={handleDiscard}
          onOpenFile={handleOpenFile}
          fileRefs={visibleFileRefs}
        />
      </PanelBody>
    </PanelRoot>
  );
});

function ChangesPanelContent({
  isLoading,
  files,
  activeSessionId,
  reviewedFiles,
  staleFiles,
  autoMarkOnScroll,
  wordWrap,
  onToggleReviewed,
  onDiscard,
  onOpenFile,
  fileRefs,
}: {
  isLoading: boolean;
  files: ReviewFile[];
  activeSessionId: string | null | undefined;
  reviewedFiles: Set<string>;
  staleFiles: Set<string>;
  autoMarkOnScroll: boolean;
  wordWrap: boolean;
  onToggleReviewed: (path: string, reviewed: boolean) => void;
  onDiscard: (path: string) => Promise<void>;
  onOpenFile: (path: string) => void;
  fileRefs: Map<string, React.RefObject<HTMLDivElement | null>>;
}) {
  if (isLoading && files.length === 0) {
    return (
      <div className="flex items-center justify-center h-full text-muted-foreground text-sm">
        Loading changes...
      </div>
    );
  }
  if (files.length === 0) {
    return (
      <div className="flex items-center justify-center h-full text-muted-foreground text-sm">
        No changes
      </div>
    );
  }
  if (!activeSessionId) return null;
  return (
    <ReviewDiffList
      files={files}
      reviewedFiles={reviewedFiles}
      staleFiles={staleFiles}
      sessionId={activeSessionId}
      autoMarkOnScroll={autoMarkOnScroll}
      wordWrap={wordWrap}
      onToggleReviewed={onToggleReviewed}
      onDiscard={onDiscard}
      onOpenFile={onOpenFile}
      fileRefs={fileRefs}
    />
  );
}

export { TaskChangesPanel };
