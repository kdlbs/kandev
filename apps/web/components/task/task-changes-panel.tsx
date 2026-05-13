"use client";

import { memo, useMemo, useCallback, createRef, useState, useEffect, useRef } from "react";
import { PanelRoot, PanelBody } from "./panel-primitives";
import { useToast } from "@/components/toast-provider";
import { useAppStore } from "@/components/state-provider";
import { useReviewSources, type ReviewSource } from "@/hooks/domains/session/use-review-sources";
import { useActiveTaskPR } from "@/hooks/domains/github/use-task-pr";
import { useGitOperations } from "@/hooks/use-git-operations";
import { useSessionFileReviews } from "@/hooks/use-session-file-reviews";
import { useCommentsStore, isDiffComment } from "@/lib/state/slices/comments";
import { getWebSocketClient } from "@/lib/ws/connection";
import { updateUserSettings } from "@/lib/api";
import { formatReviewCommentsAsMarkdown } from "@/lib/state/slices/comments/format";
import { ReviewDiffList } from "@/components/review/review-diff-list";
import type { ReviewFile } from "@/components/review/types";
import { hashDiff, reviewFileKey, splitReviewFileKey } from "@/components/review/types";
import { usePanelActions } from "@/hooks/use-panel-actions";
import { ChangesTopBar } from "./changes-top-bar";
import type { SelectedDiff } from "./task-layout";
import { useIsTaskArchived, ArchivedPanelPlaceholder } from "./task-archived-context";

type TaskChangesPanelProps = {
  mode?: "all" | "file";
  filePath?: string;
  fileRepositoryName?: string;
  selectedDiff: SelectedDiff | null;
  onClearSelected: () => void;
  onBecameEmpty?: () => void;
  /** Callback to open file in editor */
  onOpenFile?: (filePath: string) => void;
  /**
   * Restrict the diff list to a single source. Defaults to `"all"` (no
   * filter). Mobile uses this for source tabs; desktop omits it.
   */
  sourceFilter?: "all" | ReviewSource;
  /** Force word-wrap on diffs. Defaults to false (user-controlled). */
  wordWrap?: boolean;
};

// Returns true only after gitStatus loads and the file's uncommitted diff is gone.
export function shouldCloseFileDiffPanel(
  gitStatus: { files?: Record<string, { diff?: string }> } | undefined,
  filePath: string,
): boolean {
  if (!gitStatus) return false;
  const entry = gitStatus.files?.[filePath];
  return !entry?.diff;
}

function scrollToFileAndClear(
  path: string,
  fileRefs: Map<string, React.RefObject<HTMLDivElement | null>>,
  onClearSelected: () => void,
) {
  // Try exact key first (bare path for single-repo, composite if caller provides one).
  let ref = fileRefs.get(path);
  if (!ref) {
    // Fallback: selectedDiff.path is always bare — find the first matching ref.
    for (const [key, r] of fileRefs.entries()) {
      if (splitReviewFileKey(key).path === path) {
        ref = r;
        break;
      }
    }
  }
  if (ref?.current) {
    requestAnimationFrame(() => {
      ref!.current?.scrollIntoView({ behavior: "smooth", block: "start" });
      onClearSelected();
    });
  } else {
    onClearSelected();
  }
}

function useChangesView(selectedDiff: SelectedDiff | null, onClearSelected: () => void) {
  const activeSessionId = useAppStore((state) => state.tasks.activeSessionId);
  const { allFiles, cumulativeLoading, prDiffLoading, gitStatus } =
    useReviewSources(activeSessionId);
  const pr = useActiveTaskPR();
  const { reviews } = useSessionFileReviews(activeSessionId);
  const byId = useCommentsStore((s) => s.byId);
  const commentSessionIds = useCommentsStore((s) =>
    activeSessionId ? s.bySession[activeSessionId] : undefined,
  );

  const { reviewedFiles, staleFiles } = useMemo(() => {
    const reviewed = new Set<string>();
    const stale = new Set<string>();
    // When a PR exists but its diff files are still loading, the file list
    // temporarily uses cumulative diff content which has a different hash.
    // Skip review computation until PR diffs arrive to avoid a 1/1 -> 0/1 flash.
    if (pr && prDiffLoading) {
      return { reviewedFiles: reviewed, staleFiles: stale };
    }
    for (const file of allFiles) {
      const key = reviewFileKey(file);
      const reviewState = reviews.get(key);
      if (!reviewState?.reviewed) continue;
      const currentHash = hashDiff(file.diff);
      if (reviewState.diffHash && reviewState.diffHash !== currentHash) {
        stale.add(key);
      } else {
        reviewed.add(key);
      }
    }
    return { reviewedFiles: reviewed, staleFiles: stale };
  }, [allFiles, reviews, pr, prDiffLoading]);

  const totalCommentCount = useMemo(() => {
    if (!commentSessionIds || commentSessionIds.length === 0) return 0;
    let count = 0;
    for (const id of commentSessionIds) {
      const comment = byId[id];
      if (comment && isDiffComment(comment)) count++;
    }
    return count;
  }, [byId, commentSessionIds]);

  // Derive a stable key from composite file keys so refs are only recreated
  // when the file list itself changes, not when diff content updates.
  const filePathsKey = useMemo(() => allFiles.map((f) => reviewFileKey(f)).join("\0"), [allFiles]);
  const fileRefs = useMemo(() => {
    const refs = new Map<string, React.RefObject<HTMLDivElement | null>>();
    for (const file of allFiles) refs.set(reviewFileKey(file), createRef<HTMLDivElement>());
    return refs;
    // eslint-disable-next-line react-hooks/exhaustive-deps -- keyed on stable path list, not allFiles reference
  }, [filePathsKey]);

  const scrolledRef = useRef<string | null>(null);
  useEffect(() => {
    if (!selectedDiff?.path || scrolledRef.current === selectedDiff.path) return;
    scrolledRef.current = selectedDiff.path;
    scrollToFileAndClear(selectedDiff.path, fileRefs, onClearSelected);
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
    prDiffLoading,
    gitStatus,
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

function useChangesActions(
  activeSessionId: string | null | undefined,
  allFiles: ReviewFile[],
  defaultWordWrap = false,
) {
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
  const [wordWrap, setWordWrap] = useState(defaultWordWrap);

  const handleToggleSplitView = useCallback((split: boolean) => {
    setSplitView(split);
    const mode = split ? "split" : "unified";
    localStorage.setItem("diff-view-mode", mode);
    window.dispatchEvent(new CustomEvent("diff-view-mode-change", { detail: mode }));
  }, []);

  const handleToggleReviewed = useCallback(
    (key: string, reviewed: boolean) => {
      if (reviewed) {
        const file = allFiles.find((f) => reviewFileKey(f) === key);
        markReviewed(key, file ? hashDiff(file.diff) : "");
      } else {
        markUnreviewed(key);
      }
    },
    [allFiles, markReviewed, markUnreviewed],
  );

  const handleDiscard = useCallback(
    async (key: string) => {
      const { path } = splitReviewFileKey(key);
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

function useAutoCloseWhenEmpty(opts: {
  mode: "all" | "file";
  filePath: string | undefined;
  sourceFilter: "all" | ReviewSource;
  gitStatus: { files?: Record<string, { diff?: string }> } | undefined;
  visibleCount: number;
  onBecameEmpty: (() => void) | undefined;
}) {
  const { mode, filePath, sourceFilter, gitStatus, visibleCount, onBecameEmpty } = opts;
  const prevVisibleCountRef = useRef<number | null>(null);
  const prevFileSeenRef = useRef<boolean>(false);

  useEffect(() => {
    if (!onBecameEmpty) return;
    // A filtered tab going empty is not a "panel is empty" signal — the
    // other sources may still have content. Only close in the unfiltered
    // case.
    if (sourceFilter !== "all") return;

    if (mode === "file" && filePath) {
      // File-mode: close when the file no longer has an uncommitted diff,
      // regardless of whether it still appears in PR/cumulative diff sources.
      const shouldClose = shouldCloseFileDiffPanel(gitStatus, filePath);
      if (prevFileSeenRef.current && shouldClose) {
        onBecameEmpty();
        return;
      }
      if (!shouldClose) prevFileSeenRef.current = true;
      return;
    }

    const prevCount = prevVisibleCountRef.current;
    if (prevCount !== null && prevCount > 0 && visibleCount === 0) {
      onBecameEmpty();
    }
    prevVisibleCountRef.current = visibleCount;
  }, [mode, filePath, sourceFilter, gitStatus, onBecameEmpty, visibleCount]);
}

function filterVisibleFiles(
  allFiles: ReviewFile[],
  mode: "all" | "file",
  filePath: string | undefined,
  fileRepositoryName: string | undefined,
  sourceFilter: "all" | ReviewSource,
): ReviewFile[] {
  let files = allFiles;
  if (mode === "file" && filePath) {
    files = files.filter((file) => file.path === filePath);
    if (fileRepositoryName !== undefined) {
      files = files.filter((file) => (file.repository_name ?? "") === fileRepositoryName);
    }
  }
  if (sourceFilter !== "all") {
    files = files.filter((file) => file.source === sourceFilter);
  }
  return files;
}

function useVisibleDiffState(opts: {
  allFiles: ReviewFile[];
  mode: "all" | "file";
  filePath: string | undefined;
  fileRepositoryName: string | undefined;
  sourceFilter: "all" | ReviewSource;
  fileRefs: Map<string, React.RefObject<HTMLDivElement | null>>;
  reviewedFiles: Set<string>;
  staleFiles: Set<string>;
}) {
  const {
    allFiles,
    mode,
    filePath,
    fileRepositoryName,
    sourceFilter,
    fileRefs,
    reviewedFiles,
    staleFiles,
  } = opts;
  const visibleFiles = useMemo(
    () => filterVisibleFiles(allFiles, mode, filePath, fileRepositoryName, sourceFilter),
    [allFiles, mode, filePath, fileRepositoryName, sourceFilter],
  );
  const visibleFileRefs = useMemo(() => {
    if (mode !== "file" || !filePath) return fileRefs;
    const refs = new Map<string, React.RefObject<HTMLDivElement | null>>();
    for (const [key, ref] of fileRefs.entries()) {
      const split = splitReviewFileKey(key);
      if (split.path !== filePath) continue;
      if (fileRepositoryName !== undefined && (split.repositoryName || "") !== fileRepositoryName) {
        continue;
      }
      refs.set(key, ref);
    }
    return refs;
  }, [mode, filePath, fileRepositoryName, fileRefs]);
  const reviewedCount = useMemo(
    () =>
      visibleFiles.reduce((count, file) => {
        const key = reviewFileKey(file);
        if (!staleFiles.has(key) && reviewedFiles.has(key)) return count + 1;
        return count;
      }, 0),
    [visibleFiles, reviewedFiles, staleFiles],
  );
  const totalCount = visibleFiles.length;
  const progressPercent = totalCount > 0 ? (reviewedCount / totalCount) * 100 : 0;
  return { visibleFiles, visibleFileRefs, reviewedCount, totalCount, progressPercent };
}

const TaskChangesPanel = memo(function TaskChangesPanel({
  mode = "all",
  filePath,
  fileRepositoryName,
  selectedDiff,
  onClearSelected,
  onBecameEmpty,
  onOpenFile: onOpenFileProp,
  sourceFilter = "all",
  wordWrap: wordWrapProp = false,
}: TaskChangesPanelProps) {
  const isArchived = useIsTaskArchived();
  const { openFile: panelOpenFile, openFileInMarkdownPreview } = usePanelActions();
  const handleOpenFile = onOpenFileProp ?? panelOpenFile;

  const {
    activeSessionId,
    allFiles,
    reviewedFiles,
    staleFiles,
    totalCommentCount,
    fileRefs,
    cumulativeLoading,
    prDiffLoading,
    gitStatus,
  } = useChangesView(selectedDiff, onClearSelected);
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
  } = useChangesActions(activeSessionId, allFiles, wordWrapProp);
  const { visibleFiles, visibleFileRefs, reviewedCount, totalCount, progressPercent } =
    useVisibleDiffState({
      allFiles,
      mode,
      filePath,
      fileRepositoryName,
      sourceFilter,
      fileRefs,
      reviewedFiles,
      staleFiles,
    });
  const selectedFileKey = useMemo(
    () =>
      mode === "file" && filePath
        ? reviewFileKey({ path: filePath, repository_name: fileRepositoryName })
        : undefined,
    [mode, filePath, fileRepositoryName],
  );
  useAutoCloseWhenEmpty({
    mode,
    filePath,
    sourceFilter,
    gitStatus,
    visibleCount: visibleFiles.length,
    onBecameEmpty,
  });

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
          isLoading={cumulativeLoading || prDiffLoading}
          files={visibleFiles}
          activeSessionId={activeSessionId}
          reviewedFiles={reviewedFiles}
          staleFiles={staleFiles}
          autoMarkOnScroll={autoMarkOnScroll}
          wordWrap={wordWrap}
          selectedFile={selectedFileKey}
          onToggleReviewed={handleToggleReviewed}
          onDiscard={handleDiscard}
          onOpenFile={handleOpenFile}
          onPreviewMarkdown={openFileInMarkdownPreview}
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
  selectedFile,
  onToggleReviewed,
  onDiscard,
  onOpenFile,
  onPreviewMarkdown,
  fileRefs,
}: {
  isLoading: boolean;
  files: ReviewFile[];
  activeSessionId: string | null | undefined;
  reviewedFiles: Set<string>;
  staleFiles: Set<string>;
  autoMarkOnScroll: boolean;
  wordWrap: boolean;
  selectedFile?: string | null;
  onToggleReviewed: (path: string, reviewed: boolean) => void;
  onDiscard: (path: string) => Promise<void>;
  onOpenFile: (path: string) => void;
  onPreviewMarkdown?: (path: string) => void;
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
      selectedFile={selectedFile}
      onToggleReviewed={onToggleReviewed}
      onDiscard={onDiscard}
      onOpenFile={onOpenFile}
      onPreviewMarkdown={onPreviewMarkdown}
      fileRefs={fileRefs}
    />
  );
}

export { TaskChangesPanel, filterVisibleFiles, scrollToFileAndClear };
