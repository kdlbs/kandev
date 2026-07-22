"use client";

import { memo, useMemo, useCallback, createRef, useState, useEffect } from "react";
import type { DiffComment } from "@/lib/diff/types";
import type { FileInfo, CumulativeDiff } from "@/lib/state/slices/session-runtime/types";
import type { PRDiffFile, TaskPR } from "@/lib/types/github";
import type { Comment } from "@/lib/state/slices/comments";
import { useCommentsStore, isDiffComment } from "@/lib/state/slices/comments";
import { useSessionFileReviews } from "@/hooks/use-session-file-reviews";
import { useGitOperations } from "@/hooks/use-git-operations";
import { walkthroughStepMatchesFile } from "@/lib/diff/walkthrough-match";
import { useAppStore } from "@/components/state-provider";
import { useToast } from "@/components/toast-provider";
import { DEFAULT_DIFF_WORD_WRAP } from "@/components/diff/diff-defaults";
import { normalizeFileChangeStatus } from "@/lib/utils/file-change-status";
import { useRequestChangesWalkthrough } from "@/hooks/domains/session/use-request-changes-walkthrough";
import { ReviewDialogSurface } from "./review-dialog-surface";
import {
  reviewDialogSourceKey,
  usePRKeyedReviewFileSelection,
  useReviewDialogAutoClose,
  useReviewDialogTransientState,
} from "./review-dialog-pr-state";
import type { ReviewFile } from "./types";
import {
  hashDiff,
  normalizeDiffContent,
  reviewFileKey,
  splitReviewFileKey as splitFileKey,
} from "./types";

/**
 * Multi-repo dedup: keying ReviewFile entries by `path` only collapses
 * `kandev/README.md` and `lvc/README.md` into a single row. Use the shared
 * `reviewFileKey` helper from `./types` so the in-memory state, persisted
 * reviews, and the file tree all agree on one key shape. `fileMapKey` is
 * the lower-level form for code that has the path + repo name as separate
 * variables (e.g. when building maps from `Object.entries` of the backend
 * payload).
 */
function fileMapKey(path: string, repositoryName?: string): string {
  return reviewFileKey({ path, repository_name: repositoryName });
}

function addCumulativeDiffFiles(
  fileMap: Map<string, ReviewFile>,
  files: CumulativeDiff["files"],
  useRepositoryKeys: boolean,
) {
  // Multi-repo: backend stamps each per-file payload with `repository_name`
  // + the repo-relative `path`, and uses a NUL-composite `<repo>\x00<path>`
  // map key. Single-repo payloads from `parseCommitDiff` carry the bare path
  // only on the map key (no `path` field on the value). Prefer the stamped
  // value so the composite key doesn't bleed into the displayed path, and
  // fall back to the map key so single-repo files aren't silently dropped.
  for (const [mapKey, file] of Object.entries(files)) {
    const repoName = useRepositoryKeys ? file.repository_name : undefined;
    const path = file.path ?? splitFileKey(mapKey).path;
    if (!path) continue;
    const key = fileMapKey(path, repoName);
    const hasRepoUnawareCollision = key !== path && fileMap.has(path);
    if (fileMap.has(key) || hasRepoUnawareCollision) continue;
    const diff = file.diff ? normalizeDiffContent(file.diff) : "";
    fileMap.set(key, {
      path,
      diff,
      status: normalizeFileChangeStatus(file.status),
      additions: file.additions ?? 0,
      deletions: file.deletions ?? 0,
      staged: false,
      source: "committed",
      old_path: file.old_path,
      diff_skip_reason: file.diff_skip_reason,
      repository_name: repoName,
    });
  }
}

function addUncommittedFiles(
  fileMap: Map<string, ReviewFile>,
  gitStatusFiles: Record<string, FileInfo>,
) {
  for (const file of Object.values(gitStatusFiles)) {
    const key = fileMapKey(file.path, file.repository_name);
    if (fileMap.has(key)) continue;
    const diff = file.diff ? normalizeDiffContent(file.diff) : "";
    fileMap.set(key, {
      path: file.path,
      diff,
      status: normalizeFileChangeStatus(file.status),
      additions: file.additions ?? 0,
      deletions: file.deletions ?? 0,
      staged: file.staged,
      source: "uncommitted",
      old_path: file.old_path,
      diff_skip_reason: file.diff_skip_reason,
      repository_name: file.repository_name,
    });
  }
}

function addPRFiles(fileMap: Map<string, ReviewFile>, files: PRDiffFile[], repoName?: string) {
  const repositoryName = repoName || undefined;
  for (const file of files) {
    const key = fileMapKey(file.filename, repositoryName);
    const hasRepoUnawareCollision = !!repositoryName && fileMap.has(file.filename);
    if (fileMap.has(key) || hasRepoUnawareCollision) continue;
    const diff = file.patch ? normalizeDiffContent(file.patch) : "";
    fileMap.set(key, {
      path: file.filename,
      diff,
      status: normalizeFileChangeStatus(file.status),
      additions: file.additions ?? 0,
      deletions: file.deletions ?? 0,
      staged: false,
      source: "pr",
      old_path: file.old_path,
      repository_name: repositoryName,
    });
  }
}

export function buildAllFiles(
  gitStatusFiles: Record<string, FileInfo> | null,
  cumulativeDiff: CumulativeDiff | null,
  prDiffFiles?: PRDiffFile[],
  prRepoName?: string,
  useRepositoryKeys = true,
): ReviewFile[] {
  const fileMap = new Map<string, ReviewFile>();
  // Keep priority, dedup keys, and sorting aligned with `buildReviewSources`
  // in `hooks/domains/session/use-review-sources.ts`. Uncommitted goes first,
  // so its always-fresh WS-pushed diff content wins over the polled cumulative
  // diff for files that exist in both. Before
  // this, the dialog and the panel disagreed on `datasource.ts`-style files
  // (panel: fresh worktree content from `git-status`, dialog: stale cumulative
  // diff snapshot from the last fetch) — the dialog appeared to show outdated
  // content even though the cumulative-diff hook was successfully refetching.
  if (gitStatusFiles) addUncommittedFiles(fileMap, gitStatusFiles);
  if (cumulativeDiff?.files) {
    addCumulativeDiffFiles(fileMap, cumulativeDiff.files, useRepositoryKeys);
  }
  if (prDiffFiles) addPRFiles(fileMap, prDiffFiles, prRepoName);
  return Array.from(fileMap.values()).sort((a, b) => {
    const repoCmp = (a.repository_name ?? "").localeCompare(b.repository_name ?? "");
    if (repoCmp !== 0) return repoCmp;
    return a.path.localeCompare(b.path);
  });
}

export function filterPendingDiffCommentsForSession(
  comments: Comment[],
  sessionId: string,
): DiffComment[] {
  return comments.filter(
    (comment): comment is DiffComment => comment.sessionId === sessionId && isDiffComment(comment),
  );
}

export type ReviewDialogProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  sessionId: string;
  baseBranch?: string;
  onSendComments: (comments: DiffComment[]) => void;
  onOpenFile?: (filePath: string) => void;
  gitStatusFiles: Record<string, FileInfo> | null;
  cumulativeDiff: CumulativeDiff | null;
  prs?: TaskPR[];
  selectedPR?: TaskPR | null;
  selectedPRKey?: string | null;
  onSelectPR?: (pr: TaskPR) => void;
  prDiffFiles?: PRDiffFile[];
  prDiffLoading?: boolean;
  prDiffError?: string | null;
  onRetryPRDiff?: () => void;
  prRepoName?: string;
  useRepositoryKeys?: boolean;
};

function computeReviewSets(
  allFiles: ReviewFile[],
  reviews: ReturnType<typeof useSessionFileReviews>["reviews"],
) {
  const reviewed = new Set<string>();
  const stale = new Set<string>();
  for (const file of allFiles) {
    const key = reviewFileKey(file);
    const reviewState = reviews.get(key);
    if (!reviewState?.reviewed) continue;
    const currentHash = hashDiff(file.diff);
    if (reviewState.diffHash && reviewState.diffHash !== currentHash) stale.add(key);
    else reviewed.add(key);
  }
  return { reviewedFiles: reviewed, staleFiles: stale };
}

/**
 * Counts diff comments per file, scoped by repo when known. Multi-repo:
 * comments carrying `repositoryId` are matched only against files in that
 * repo (translated from `repository_name` via `repositoryNameToId`); legacy
 * comments without `repositoryId` and same-repo unattributed comments
 * match by path. Returned record is keyed by `reviewFileKey(file)` so the
 * file tree's per-row badge correctly disambiguates same-named files.
 */
function computeCommentCounts(
  byId: Record<string, import("@/lib/state/slices/comments").Comment>,
  sessionCommentIds: string[] | undefined,
  allFiles: ReviewFile[],
  repositoryNameToId: Map<string, string>,
): Record<string, number> {
  const counts: Record<string, number> = {};
  if (!sessionCommentIds) return counts;

  type BucketKey = string;
  const bucketKey = (repoId: string, path: string) => `${repoId}::${path}`;
  const bucket = new Map<BucketKey, number>();
  for (const id of sessionCommentIds) {
    const comment = byId[id];
    if (!comment || !isDiffComment(comment)) continue;
    const k = bucketKey(comment.repositoryId ?? "", comment.filePath);
    bucket.set(k, (bucket.get(k) ?? 0) + 1);
  }

  for (const file of allFiles) {
    let total = 0;
    if (file.repository_name) {
      const repoId = repositoryNameToId.get(file.repository_name) ?? "";
      if (repoId) total += bucket.get(bucketKey(repoId, file.path)) ?? 0;
    }
    // Path-only comments (legacy / pre-multi-repo data) match every file
    // sharing the path; this preserves the prior single-repo behavior and
    // keeps existing comments visible after the multi-repo rollout.
    total += bucket.get(bucketKey("", file.path)) ?? 0;
    if (total > 0) counts[reviewFileKey(file)] = total;
  }
  return counts;
}

type ReviewDialogHandlerOptions = {
  allFiles: ReviewFile[];
  markReviewed: (path: string, hash: string) => void;
  markUnreviewed: (path: string) => void;
  onSendComments: ReviewDialogProps["onSendComments"];
  onOpenChange: ReviewDialogProps["onOpenChange"];
  sessionId: string;
};

function useReviewDialogHandlers(opts: ReviewDialogHandlerOptions) {
  const { allFiles, markReviewed, markUnreviewed, onSendComments, onOpenChange, sessionId } = opts;
  const { discard } = useGitOperations(sessionId);
  const { toast } = useToast();

  const handleToggleSplitView = useCallback((split: boolean) => {
    const mode = split ? "split" : "unified";
    localStorage.setItem("diff-view-mode", mode);
    window.dispatchEvent(new CustomEvent("diff-view-mode-change", { detail: mode }));
  }, []);

  const handleSelectFile = useCallback((key: string, setSelectedFile: (k: string) => void) => {
    setSelectedFile(key);
    // Note: scrolling is now handled by FileDiffSection when isSelected changes
    // This ensures proper timing after the section expands
  }, []);

  const handleToggleReviewed = useCallback(
    (key: string, reviewed: boolean) => {
      if (reviewed) {
        // Look up by composite key so two same-name files in different repos
        // don't share their reviewed/diff-hash.
        const file = allFiles.find((f) => reviewFileKey(f) === key);
        markReviewed(key, file ? hashDiff(file.diff) : "");
      } else markUnreviewed(key);
    },
    [allFiles, markReviewed, markUnreviewed],
  );

  const handleSendComments = useCallback(
    (comments: DiffComment[]) => {
      onSendComments(comments);
      onOpenChange(false);
    },
    [onSendComments, onOpenChange],
  );

  const handleDiscard = useCallback(
    async (key: string) => {
      // Caller passes the composite key; split out the repo + path so
      // discard runs in the correct repo's worktree.
      const { repositoryName, path } = splitFileKey(key);
      try {
        const result = await discard([path], repositoryName || undefined);
        if (result.success)
          toast({ title: "Changes discarded", description: path, variant: "success" });
        else
          toast({
            title: "Discard failed",
            description: result.error || "An error occurred",
            variant: "error",
          });
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

  return {
    handleToggleSplitView,
    handleSelectFile,
    handleToggleReviewed,
    handleSendComments,
    handleDiscard,
  };
}

function useRepositoryNameToId() {
  const reposByWorkspace = useAppStore((s) => s.repositories.itemsByWorkspaceId);
  return useMemo(() => {
    const m = new Map<string, string>();
    for (const list of Object.values(reposByWorkspace)) {
      for (const r of list) m.set(r.name, r.id);
    }
    return m;
  }, [reposByWorkspace]);
}

function useFilteredReviewFiles(allFiles: ReviewFile[], filter: string) {
  return useMemo(() => {
    if (!filter.trim()) return allFiles;
    const q = filter.toLowerCase();
    return allFiles.filter((f) => f.path.toLowerCase().includes(q));
  }, [allFiles, filter]);
}

function useReviewDialogState(props: ReviewDialogProps) {
  const {
    open,
    onOpenChange,
    sessionId,
    onSendComments,
    gitStatusFiles,
    cumulativeDiff,
    selectedPRKey = null,
    prDiffFiles,
    prDiffLoading = false,
    prRepoName,
    useRepositoryKeys = true,
  } = props;
  const reviewSourceKey = reviewDialogSourceKey(sessionId, selectedPRKey);
  const { selectedFile, filter, setSelectedFile, setFilter } =
    useReviewDialogTransientState(reviewSourceKey);
  const [splitView, setSplitView] = useState(() =>
    typeof window === "undefined" ? false : localStorage.getItem("diff-view-mode") === "split",
  );
  const [wordWrap, setWordWrap] = useState(DEFAULT_DIFF_WORD_WRAP);
  const autoMarkOnScroll = useAppStore((s) => s.userSettings.reviewAutoMarkOnScroll);
  const { reviews, markReviewed, markUnreviewed } = useSessionFileReviews(sessionId);
  const byId = useCommentsStore((s) => s.byId);
  const sessionCommentIds = useCommentsStore((s) => s.bySession[sessionId]);
  const getStorePendingComments = useCommentsStore((s) => s.getPendingComments);
  const getPendingComments = useCallback((): DiffComment[] => {
    return filterPendingDiffCommentsForSession(getStorePendingComments(), sessionId);
  }, [getStorePendingComments, sessionId]);
  const markCommentsSent = useCommentsStore((s) => s.markCommentsSent);

  const allFiles = useMemo<ReviewFile[]>(
    () => buildAllFiles(gitStatusFiles, cumulativeDiff, prDiffFiles, prRepoName, useRepositoryKeys),
    [gitStatusFiles, cumulativeDiff, prDiffFiles, prRepoName, useRepositoryKeys],
  );
  const filteredFiles = useFilteredReviewFiles(allFiles, filter);
  const { reviewedFiles, staleFiles } = useMemo(
    () => computeReviewSets(allFiles, reviews),
    [allFiles, reviews],
  );
  const repositoryNameToId = useRepositoryNameToId();
  const commentCountByFile = useMemo(
    () => computeCommentCounts(byId, sessionCommentIds, allFiles, repositoryNameToId),
    [byId, sessionCommentIds, allFiles, repositoryNameToId],
  );
  const totalCommentCount = useMemo(
    () => Object.values(commentCountByFile).reduce((sum, c) => sum + c, 0),
    [commentCountByFile],
  );
  // Composite-key the file refs so two files with the same path in different
  // repos get distinct refs (otherwise auto-scroll-on-select / "selected"
  // styling lands on the wrong row).
  const fileRefs = useMemo(() => {
    const refs = new Map<string, React.RefObject<HTMLDivElement | null>>();
    for (const file of allFiles) refs.set(reviewFileKey(file), createRef<HTMLDivElement>());
    return refs;
  }, [allFiles]);
  useReviewDialogAutoClose({ open, fileCount: allFiles.length, prDiffLoading, onOpenChange });

  const handlers = useReviewDialogHandlers({
    allFiles,
    markReviewed,
    markUnreviewed,
    onSendComments,
    onOpenChange,
    sessionId,
  });
  const handleToggleSplitView = useCallback(
    (split: boolean) => {
      setSplitView(split);
      handlers.handleToggleSplitView(split);
    },
    [handlers.handleToggleSplitView],
  );
  const handleSelectFile = usePRKeyedReviewFileSelection(
    handlers.handleSelectFile,
    setSelectedFile,
  );

  return {
    reviewSourceKey,
    selectedFile,
    splitView,
    wordWrap,
    setWordWrap,
    autoMarkOnScroll,
    filter,
    setFilter,
    allFiles,
    filteredFiles,
    reviewedFiles,
    staleFiles,
    commentCountByFile,
    totalCommentCount,
    fileRefs,
    getPendingComments,
    markCommentsSent,
    handleToggleSplitView,
    handleSelectFile,
    handleToggleReviewed: handlers.handleToggleReviewed,
    handleSendComments: handlers.handleSendComments,
    handleDiscard: handlers.handleDiscard,
  };
}

export type ReviewDialogViewState = ReturnType<typeof useReviewDialogState>;

/**
 * Drives review file-selection from the active walkthrough step: when the tour
 * advances (or the dialog opens on a tour), select+scroll to the step's file so
 * its diff expands and the inline WalkthroughStepCard renders at the line.
 */
function useWalkthroughFileSelection(
  open: boolean,
  allFiles: ReviewFile[],
  filter: string,
  setFilter: (value: string) => void,
  handleSelectFile: (key: string) => void,
) {
  const step = useAppStore((state) => {
    const taskId = state.tasks.activeTaskId;
    if (!taskId) return null;
    const wt = state.walkthroughs.byTaskId[taskId];
    const idx = state.walkthroughs.activeStepByTaskId[taskId] ?? 0;
    return wt?.steps[idx] ?? null;
  });
  useEffect(() => {
    if (!open || !step) return;
    const file = allFiles.find((f) => walkthroughStepMatchesFile(f, step, allFiles));
    if (!file) return;
    const q = filter.trim().toLowerCase();
    if (q && !file.path.toLowerCase().includes(q)) setFilter("");
    handleSelectFile(reviewFileKey(file));
  }, [open, step, allFiles, filter, setFilter, handleSelectFile]);
}

function useReviewDialogWalkthroughRequest({
  sessionId,
  allFiles,
}: {
  sessionId: string;
  allFiles: ReviewFile[];
}) {
  const activeTaskId = useAppStore((state) => state.tasks.activeTaskId);
  return useRequestChangesWalkthrough({
    taskId: activeTaskId,
    sessionId,
    ready: allFiles.length > 0,
  });
}

export const ReviewDialog = memo(function ReviewDialog(props: ReviewDialogProps) {
  const {
    open,
    onOpenChange,
    sessionId,
    baseBranch,
    onOpenFile,
    prs = [],
    selectedPR = null,
    onSelectPR,
    prDiffLoading = false,
    prDiffError = null,
    onRetryPRDiff,
  } = props;
  const s = useReviewDialogState(props);
  const handleRequestWalkthrough = useReviewDialogWalkthroughRequest({
    sessionId,
    allFiles: s.allFiles,
  });
  useWalkthroughFileSelection(open, s.allFiles, s.filter, s.setFilter, s.handleSelectFile);

  return (
    <ReviewDialogSurface
      open={open}
      onOpenChange={onOpenChange}
      sessionId={sessionId}
      baseBranch={baseBranch}
      onOpenFile={onOpenFile}
      prs={prs}
      selectedPR={selectedPR}
      onSelectPR={onSelectPR}
      prDiffLoading={prDiffLoading}
      prDiffError={prDiffError}
      onRetryPRDiff={onRetryPRDiff}
      onRequestWalkthrough={handleRequestWalkthrough}
      state={s}
    />
  );
});
