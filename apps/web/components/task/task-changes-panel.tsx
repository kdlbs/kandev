"use client";

import { memo, useMemo, useCallback, createRef, useState, useEffect, useRef } from "react";
import { PanelRoot, PanelBody } from "./panel-primitives";
import { TruncatedFilesBanner, useSelectedFileKey } from "./changes-panel-banner";
import { useToast } from "@/components/toast-provider";
import { useAppStore } from "@/components/state-provider";
import { useReviewSources, type ReviewSource } from "@/hooks/domains/session/use-review-sources";
import { useGitOperations } from "@/hooks/use-git-operations";
import { useSessionFileReviews } from "@/hooks/use-session-file-reviews";
import { useCommentsStore, isDiffComment } from "@/lib/state/slices/comments";
import { getWebSocketClient } from "@/lib/ws/connection";
import { updateUserSettings } from "@/lib/api";
import { formatReviewCommentsAsMarkdown } from "@/lib/state/slices/comments/format";
import { ReviewDiffList } from "@/components/review/review-diff-list";
import { ReviewPRDiffBoundary } from "@/components/review/review-dialog-pr-state";
import { DEFAULT_DIFF_WORD_WRAP } from "@/components/diff/diff-defaults";
import type { ReviewFile } from "@/components/review/types";
import { hashDiff, reviewFileKey, splitReviewFileKey } from "@/components/review/types";
import { usePanelActions } from "@/hooks/use-panel-actions";
import { useRequestChangesWalkthrough } from "@/hooks/domains/session/use-request-changes-walkthrough";
import { ChangesTopBar } from "./changes-top-bar";
import {
  resolveSelectedFileRepositoryName,
  shouldBlockChangesForPR,
  shouldDeferReviewStateForPR,
  useAutoCloseWhenEmpty,
  useVisibleDiffState,
} from "./task-changes-panel-state";
import type { SelectedDiff } from "./task-layout";
import { useIsTaskArchived, ArchivedPanelPlaceholder } from "./task-archived-context";

type TaskChangesPanelProps = {
  mode?: "all" | "file";
  filePath?: string;
  fileRepositoryName?: string;
  prKey?: string;
  selectedDiff: SelectedDiff | null;
  onClearSelected: () => void;
  onBecameEmpty?: () => void;
  /** Callback to open file in editor */
  onOpenFile?: (filePath: string, repo?: string) => void;
  /**
   * Restrict the diff list to a single source. Defaults to `"all"` (no
   * filter). Mobile uses this for source tabs; desktop omits it.
   */
  sourceFilter?: "all" | ReviewSource;
  /** Force word-wrap on diffs. Defaults to the app diff preference. */
  wordWrap?: boolean;
};

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

function useChangesView(
  selectedDiff: SelectedDiff | null,
  onClearSelected: () => void,
  sourceFilter: "all" | ReviewSource,
  explicitPRKey?: string,
) {
  const activeSessionId = useAppStore((state) => state.tasks.activeSessionId);
  const reviewSources = useReviewSources(activeSessionId, explicitPRKey);
  const {
    allFiles,
    cumulativeLoading,
    prDiffLoading,
    prDiffError,
    refreshPRDiff,
    gitStatus,
    rawPRFiles,
    truncatedFilesCount,
    selectedPR: pr,
  } = reviewSources;
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
    if (shouldDeferReviewStateForPR(Boolean(pr), prDiffLoading, sourceFilter)) {
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
  }, [allFiles, reviews, pr, prDiffLoading, sourceFilter]);

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
    rawPRFiles,
    reviewedFiles,
    staleFiles,
    totalCommentCount,
    fileRefs,
    cumulativeLoading,
    prDiffLoading,
    prDiffError,
    refreshPRDiff,
    gitStatus,
    truncatedFilesCount,
    prs: reviewSources.prs,
    selectedPR: reviewSources.selectedPR,
    selectPR: reviewSources.selectPR,
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

function useWalkthroughRequest(activeSessionId: string | null | undefined, allFiles: ReviewFile[]) {
  const activeTaskId = useAppStore((s) => s.tasks.activeTaskId);
  return useRequestChangesWalkthrough({
    taskId: activeTaskId,
    sessionId: activeSessionId,
    ready: allFiles.length > 0,
  });
}

function useChangesPRPresentation(opts: {
  sourceFilter: "all" | ReviewSource;
  prKey: string | undefined;
  mode: "all" | "file";
  filePath: string | undefined;
  fileRepositoryName: string | undefined;
  visibleFiles: ReviewFile[];
}) {
  const visibleSelectedFile = opts.visibleFiles.find((file) => file.path === opts.filePath);
  const selectedFileKey = useSelectedFileKey(
    opts.mode,
    opts.filePath,
    resolveSelectedFileRepositoryName(
      opts.sourceFilter,
      opts.prKey,
      opts.fileRepositoryName,
      visibleSelectedFile?.repository_name,
    ),
  );
  const blockChangesForPR = shouldBlockChangesForPR(opts.sourceFilter, opts.visibleFiles);
  return { selectedFileKey, blockChangesForPR };
}

function useChangesActions(
  activeSessionId: string | null | undefined,
  allFiles: ReviewFile[],
  defaultWordWrap = DEFAULT_DIFF_WORD_WRAP,
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

const TaskChangesPanel = memo(function TaskChangesPanel({
  mode = "all",
  filePath,
  fileRepositoryName,
  prKey,
  selectedDiff,
  onClearSelected,
  onBecameEmpty,
  onOpenFile: onOpenFileProp,
  sourceFilter = "all",
  wordWrap: wordWrapProp = DEFAULT_DIFF_WORD_WRAP,
}: TaskChangesPanelProps) {
  const isArchived = useIsTaskArchived();
  const { openFile: panelOpenFile, openFileInMarkdownPreview } = usePanelActions();
  const handleOpenFile = onOpenFileProp ?? panelOpenFile;

  const view = useChangesView(selectedDiff, onClearSelected, sourceFilter, prKey);
  const usesPRDiff = sourceFilter === "all" || sourceFilter === "pr";
  const relevantPRLoading = usesPRDiff && view.prDiffLoading;
  const actions = useChangesActions(view.activeSessionId, view.allFiles, wordWrapProp);
  const handleRequestWalkthrough = useWalkthroughRequest(view.activeSessionId, view.allFiles);
  const visible = useVisibleDiffState({
    allFiles: view.allFiles,
    rawPRFiles: view.rawPRFiles,
    mode,
    filePath,
    fileRepositoryName,
    sourceFilter,
    prKey,
    fileRefs: view.fileRefs,
    reviewedFiles: view.reviewedFiles,
    staleFiles: view.staleFiles,
  });
  const { selectedFileKey, blockChangesForPR } = useChangesPRPresentation({
    sourceFilter,
    prKey,
    mode,
    filePath,
    fileRepositoryName,
    visibleFiles: visible.visibleFiles,
  });
  useAutoCloseWhenEmpty({
    mode,
    filePath,
    sourceFilter,
    gitStatus: view.gitStatus,
    visibleCount: visible.visibleFiles.length,
    prDiffLoading: relevantPRLoading,
    onBecameEmpty,
  });

  if (isArchived) return <ArchivedPanelPlaceholder />;

  return (
    <PanelRoot>
      <ChangesTopBar
        autoMarkOnScroll={actions.autoMarkOnScroll}
        splitView={actions.splitView}
        wordWrap={actions.wordWrap}
        totalCommentCount={view.totalCommentCount}
        reviewedCount={visible.reviewedCount}
        totalCount={visible.totalCount}
        progressPercent={visible.progressPercent}
        setWordWrap={actions.setWordWrap}
        handleToggleSplitView={actions.handleToggleSplitView}
        handleToggleAutoMark={actions.handleToggleAutoMark}
        handleFixComments={actions.handleFixComments}
        handleRequestWalkthrough={handleRequestWalkthrough}
        requestWalkthroughDisabled={view.allFiles.length === 0}
        prs={mode === "all" ? view.prs : []}
        selectedPR={mode === "all" ? view.selectedPR : null}
        prDiffLoading={view.prDiffLoading}
        onSelectPR={view.selectPR}
      />
      <PanelBody padding={false} scroll={false} className="overflow-hidden">
        <TruncatedFilesBanner count={view.truncatedFilesCount} />
        <ReviewPRDiffBoundary
          selectedPR={usesPRDiff && blockChangesForPR ? view.selectedPR : null}
          loading={blockChangesForPR && relevantPRLoading}
          error={blockChangesForPR ? view.prDiffError : null}
          onRetry={view.refreshPRDiff}
        >
          <ChangesPanelContent
            isLoading={view.cumulativeLoading || relevantPRLoading}
            files={visible.visibleFiles}
            activeSessionId={view.activeSessionId}
            reviewedFiles={view.reviewedFiles}
            staleFiles={view.staleFiles}
            autoMarkOnScroll={actions.autoMarkOnScroll}
            wordWrap={actions.wordWrap}
            selectedFile={selectedFileKey}
            onToggleReviewed={actions.handleToggleReviewed}
            onDiscard={actions.handleDiscard}
            onOpenFile={handleOpenFile}
            onPreviewMarkdown={openFileInMarkdownPreview}
            fileRefs={visible.visibleFileRefs}
          />
        </ReviewPRDiffBoundary>
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
  onOpenFile: (path: string, repo?: string) => void;
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

export { TaskChangesPanel, scrollToFileAndClear };
