"use client";

import { memo, useEffect, useMemo, useRef, useState, useCallback } from "react";
import { FileDiffViewer, DiffErrorBoundary } from "@/components/diff";
import type { RevertBlockInfo } from "@/components/diff";
import { getWebSocketClient } from "@/lib/ws/connection";
import { requestFileContent, updateFileContent } from "@/lib/ws/workspace-files";
import { generateUnifiedDiff, calculateHash } from "@/lib/utils/file-diff";
import { useAppStore } from "@/components/state-provider";
import { useToast } from "@/components/toast-provider";
import { useRunComment } from "@/hooks/domains/comments/use-run-comment";
import { useBaseBranchByRepo } from "@/hooks/domains/session/use-base-branch-by-repo";
import type { DiffComment } from "@/lib/diff/types";
import {
  diffSkipReasonLabel,
  hasTextualDiff,
  reviewDiffUnavailableLabel,
  reviewFileKey,
} from "./types";
import type { ReviewFile } from "./types";
import { RepoGroupHeader } from "./review-diff-list-groups";
import { ReviewDiffHeader, type ReviewExternalLinkContext } from "./review-diff-header";
import { groupByRepositoryName } from "@/lib/group-by-repo";
import { useActiveTaskPR } from "@/hooks/domains/github/use-task-pr";

type ReviewDiffListProps = {
  files: ReviewFile[];
  reviewedFiles: Set<string>;
  staleFiles: Set<string>;
  sessionId: string;
  autoMarkOnScroll: boolean;
  wordWrap: boolean;
  selectedFile?: string | null;
  onToggleReviewed: (path: string, reviewed: boolean) => void;
  onDiscard: (path: string) => void;
  onOpenFile?: (filePath: string, repo?: string) => void;
  onPreviewMarkdown?: (filePath: string) => void;
  fileRefs: Map<string, React.RefObject<HTMLDivElement | null>>;
};

export const ReviewDiffList = memo(function ReviewDiffList({
  files,
  reviewedFiles,
  staleFiles,
  sessionId,
  autoMarkOnScroll,
  wordWrap,
  selectedFile,
  onToggleReviewed,
  onDiscard,
  onOpenFile,
  onPreviewMarkdown,
  fileRefs,
}: ReviewDiffListProps) {
  const scrollContainerRef = useRef<HTMLDivElement | null>(null);
  // Resolve base branches once per list (not per row) — the value is identical
  // for every file. Only a single-repo task has an unambiguous fallback; with
  // multiple repos a committed file lacking `repository_name` must NOT borrow
  // an arbitrary repo's base branch, so the fallback stays undefined there.
  const activeTaskId = useAppStore((state) => state.tasks.activeTaskId);
  const activeTaskPR = useActiveTaskPR();
  const baseBranchByRepo = useBaseBranchByRepo(activeTaskId);
  const fallbackBaseBranch = useMemo(() => {
    const branches = Object.values(baseBranchByRepo);
    return branches.length === 1 ? branches[0] : undefined;
  }, [baseBranchByRepo]);
  // All in-memory state (selectedFile, reviewedFiles, staleFiles, fileRefs)
  // is keyed by `reviewFileKey(file)` so two files at the same path in
  // different repos (e.g. `kandev/README.md` + `lvc/README.md`) get
  // distinct rows. Without this every multi-repo review with same-named
  // files would mark them all reviewed when one is checked.
  const selectedIndex = selectedFile
    ? files.findIndex((f) => reviewFileKey(f) === selectedFile)
    : -1;
  const groups = useMemo(() => groupByRepositoryName(files, (f) => f.repository_name), [files]);
  const showRepoHeaders = groups.length > 1 || (groups[0]?.repositoryName ?? "") !== "";
  return (
    <div ref={scrollContainerRef} className="overflow-y-auto h-full">
      {groups.map((group) => (
        <div
          key={group.repositoryName || "__no_repo__"}
          data-testid="changes-repo-group"
          data-repository-name={group.repositoryName || ""}
        >
          {showRepoHeaders && (
            <RepoGroupHeader name={group.repositoryName} fileCount={group.items.length} />
          )}
          {group.items.map((file) => {
            const key = reviewFileKey(file);
            return (
              <FileDiffSection
                key={key}
                file={file}
                fileKey={key}
                isReviewed={reviewedFiles.has(key) && !staleFiles.has(key)}
                isStale={staleFiles.has(key)}
                sessionId={sessionId}
                autoMarkOnScroll={autoMarkOnScroll}
                wordWrap={wordWrap}
                isSelected={selectedFile === key}
                forceLoad={
                  selectedIndex >= 0 &&
                  files.findIndex((f) => reviewFileKey(f) === key) <= selectedIndex
                }
                onToggleReviewed={onToggleReviewed}
                onDiscard={onDiscard}
                onOpenFile={onOpenFile}
                onPreviewMarkdown={onPreviewMarkdown}
                sectionRef={fileRefs.get(key)}
                scrollContainer={scrollContainerRef}
                externalLinkContext={{
                  baseBranchByRepo,
                  fallbackBaseBranch,
                  taskId: activeTaskId,
                  publishedPRBranch: activeTaskPR?.head_branch,
                  publishedPRRepositoryId: activeTaskPR?.repository_id,
                }}
              />
            );
          })}
        </div>
      ))}
    </div>
  );
});

// Per-repo grouping helpers live in ./review-diff-list-groups so this file
// stays under the 600-line lint cap.

type FileDiffSectionProps = {
  file: ReviewFile;
  /** Composite per-file key from `reviewFileKey(file)` — used as the arg
   *  to `onToggleReviewed` / `onDiscard` so callers can disambiguate
   *  same-named files in different repos. */
  fileKey: string;
  isReviewed: boolean;
  isStale: boolean;
  sessionId: string;
  autoMarkOnScroll: boolean;
  wordWrap: boolean;
  isSelected?: boolean;
  forceLoad?: boolean;
  onToggleReviewed: (key: string, reviewed: boolean) => void;
  onDiscard: (key: string) => void;
  onOpenFile?: (filePath: string, repo?: string) => void;
  onPreviewMarkdown?: (filePath: string) => void;
  sectionRef?: React.RefObject<HTMLDivElement | null>;
  scrollContainer: React.RefObject<HTMLDivElement | null>;
  /** Per-repo base branches + single-repo fallback, resolved once by the list
   *  and shared across rows so diff expansion can fetch the correct old side. */
  externalLinkContext: ReviewExternalLinkContext;
};

function useLazyVisible(scrollContainer: React.RefObject<HTMLDivElement | null>) {
  const [isVisible, setIsVisible] = useState(false);
  const sentinelRef = useRef<HTMLDivElement | null>(null);
  useEffect(() => {
    const sentinel = sentinelRef.current;
    if (!sentinel) return;
    const observer = new IntersectionObserver(
      ([entry]) => {
        if (entry.isIntersecting) {
          setIsVisible(true);
          observer.disconnect();
        }
      },
      { rootMargin: "200px 0px", root: scrollContainer.current },
    );
    observer.observe(sentinel);
    return () => observer.disconnect();
  }, [scrollContainer]);
  return { isVisible, sentinelRef };
}

type AutoMarkArgs = {
  autoMarkOnScroll: boolean;
  isReviewed: boolean;
  isStale: boolean;
  /** Composite per-file key (matches what onToggleReviewed expects). */
  fileKey: string;
  onToggleReviewed: (key: string, reviewed: boolean) => void;
  scrollContainer: React.RefObject<HTMLDivElement | null>;
};

function useAutoMarkOnScroll({
  autoMarkOnScroll,
  isReviewed,
  isStale,
  fileKey,
  onToggleReviewed,
  scrollContainer,
}: AutoMarkArgs) {
  const scrollSentinelRef = useRef<HTMLDivElement | null>(null);
  const autoMarkedRef = useRef(false);
  useEffect(() => {
    if (!autoMarkOnScroll || isReviewed || isStale) {
      autoMarkedRef.current = false;
      return;
    }
    const sentinel = scrollSentinelRef.current;
    const root = scrollContainer.current;
    if (!sentinel || !root) return;
    const observer = new IntersectionObserver(
      ([entry]) => {
        if (
          !entry.isIntersecting &&
          entry.boundingClientRect.top < root.getBoundingClientRect().top &&
          !autoMarkedRef.current
        ) {
          autoMarkedRef.current = true;
          console.debug("[review] auto-mark reviewed:", fileKey);
          onToggleReviewed(fileKey, true);
        }
      },
      { threshold: 0, root },
    );
    observer.observe(sentinel);
    return () => observer.disconnect();
  }, [autoMarkOnScroll, fileKey, isReviewed, isStale, onToggleReviewed, scrollContainer]);
  return scrollSentinelRef;
}

function useCommentRunHandler(sessionId: string) {
  const activeTaskId = useAppStore((state) => state.tasks.activeTaskId);
  const { toast } = useToast();
  const { runComment } = useRunComment({
    sessionId,
    taskId: activeTaskId ?? null,
  });
  return useCallback(
    async (comment: DiffComment) => {
      try {
        const { queued } = await runComment(comment);
        toast({
          title: "Comment sent",
          description: queued ? "Queued for the agent." : "Sent to the agent.",
        });
      } catch (err) {
        console.error("Failed to run diff comment:", err);
        toast({
          title: "Failed to send comment",
          description: "Please try again.",
          variant: "error",
        });
      }
    },
    [runComment, toast],
  );
}

async function revertBlock(
  sessionId: string,
  filePath: string,
  info: RevertBlockInfo,
  repo?: string,
) {
  const client = getWebSocketClient();
  if (!client) return;
  try {
    const response = await requestFileContent(client, sessionId, filePath, repo);
    if (response.error) return;
    const currentContent = response.content;
    const hash = await calculateHash(currentContent);
    const lines = currentContent.split("\n");
    lines.splice(info.addStart - 1, info.addCount, ...info.oldLines);
    const nextContent = lines.join("\n");
    if (nextContent === currentContent) return;
    const patch = generateUnifiedDiff(currentContent, nextContent, filePath);
    if (!patch || !/^@@/m.test(patch)) return;
    await updateFileContent(client, sessionId, {
      path: filePath,
      diff: patch,
      originalHash: hash,
      repo,
    });
  } catch (err) {
    console.error("Failed to revert change block:", err);
  }
}

function useScrollIntoViewOnSelect(
  isSelected: boolean | undefined,
  sectionRef: React.RefObject<HTMLDivElement | null> | undefined,
  setCollapsed: React.Dispatch<React.SetStateAction<boolean>>,
) {
  useEffect(() => {
    if (isSelected) {
      setCollapsed(false);
      requestAnimationFrame(() => {
        requestAnimationFrame(() => {
          sectionRef?.current?.scrollIntoView({ behavior: "smooth", block: "start" });
        });
      });
    }
  }, [isSelected, sectionRef, setCollapsed]);
}

/**
 * Decide whether a review row can expand its collapsed context, and which git
 * ref supplies the "old" side when reconstructing full file context.
 *
 * Expansion reveals the *unmodified* lines hidden between hunks. @pierre/diffs
 * needs the full old+new file contents (paired with the patch) to render those
 * controls; we fetch the new side from the working tree and the old side from
 * `baseRef`. The ref has to match the base the diff was computed against, or
 * the reparse comes out inconsistent and silently falls back to a partial
 * (controls-less) render:
 *
 *  - uncommitted rows: diff is working-tree-vs-HEAD, so baseRef="HEAD".
 *  - committed rows: diff is base-vs-HEAD, so baseRef is the exact old-side
 *    commit carried by the file; payloads without that ref fall back to the
 *    repo's base branch. HEAD already contains the commits, so expanding against it
 *    pairs identical old/new content and yields no controls (the bug behind
 *    "expansion stopped working in the review screen"). With no known base
 *    branch we can't fetch the pre-change content, so expansion is disabled
 *    rather than rendering dead separators.
 *  - PR rows: the working tree belongs to the local branch, not the PR head,
 *    so the fetched content would be paired with the wrong patch — disabled.
 *  - untracked files: a synthetic all-additions hunk against /dev/null with no
 *    real context to expand — disabled.
 *
 * The @pierre/diffs trailing-context guard in `useExpandableDiff` keeps any
 * mismatch (stale snapshot, wrong base, file edited mid-flight) from crashing
 * the renderer, so enabling expansion here is always safe.
 */
export function resolveDiffExpansion(
  file: Pick<ReviewFile, "source" | "status" | "repository_name" | "base_ref">,
  baseBranchByRepo: Record<string, string>,
  fallbackBaseBranch?: string,
): { enableExpansion: boolean; baseRef: string } {
  if (file.source === "pr" || file.status === "untracked") {
    return { enableExpansion: false, baseRef: "HEAD" };
  }
  if (file.source === "committed") {
    if (file.base_ref) return { enableExpansion: true, baseRef: file.base_ref };
    const repoName = file.repository_name ?? "";
    // Exact per-repo base wins. Fall back to the task's sole base branch ONLY
    // for single-repo files (no repository_name) — a multi-repo file whose repo
    // isn't in the map must NOT borrow another repo's base branch, which would
    // fetch the wrong "old" content and silently drop expansion.
    const base = baseBranchByRepo[repoName] ?? (repoName === "" ? fallbackBaseBranch : undefined);
    if (!base) return { enableExpansion: false, baseRef: "HEAD" };
    return { enableExpansion: true, baseRef: base };
  }
  return { enableExpansion: true, baseRef: "HEAD" };
}

function renderDiffContent(opts: {
  shouldRender: boolean;
  file: ReviewFile;
  sessionId: string;
  wordWrap: boolean;
  expandUnchanged: boolean;
  enableExpansion: boolean;
  baseRef: string;
  onRevertBlock: (filePath: string, info: RevertBlockInfo) => void;
  onCommentRun: (comment: DiffComment) => void;
  onToggleExpandUnchanged: () => void;
}) {
  const {
    shouldRender,
    file,
    sessionId,
    wordWrap,
    expandUnchanged,
    enableExpansion,
    baseRef,
    onRevertBlock,
    onCommentRun,
    onToggleExpandUnchanged,
  } = opts;
  const hasText = hasTextualDiff(file);
  if (shouldRender && hasText) {
    return (
      <>
        <DiffErrorBoundary filePath={file.path}>
          <FileDiffViewer
            filePath={file.path}
            diff={file.diff}
            status={file.status}
            enableComments
            enableAcceptReject
            onRevertBlock={onRevertBlock}
            onCommentRun={onCommentRun}
            sessionId={sessionId}
            wordWrap={wordWrap}
            enableExpansion={enableExpansion}
            baseRef={baseRef}
            hideHeader
            expandUnchanged={expandUnchanged}
            onToggleExpandUnchanged={onToggleExpandUnchanged}
            repo={file.repository_name}
          />
        </DiffErrorBoundary>
        {file.diff_skip_reason === "truncated" && (
          <div className="py-1 text-center text-xs text-muted-foreground border-t">
            Diff truncated — showing first 256 KB
          </div>
        )}
      </>
    );
  }
  const message = hasText
    ? diffSkipReasonLabel(file.diff_skip_reason)
    : reviewDiffUnavailableLabel(file);
  return (
    <div className="flex items-center justify-center py-12 text-muted-foreground text-sm">
      {message}
    </div>
  );
}

function FileDiffSection({
  file,
  fileKey,
  isReviewed,
  isStale,
  sessionId,
  autoMarkOnScroll,
  wordWrap,
  isSelected,
  forceLoad,
  onToggleReviewed,
  onDiscard,
  onOpenFile,
  onPreviewMarkdown,
  sectionRef,
  scrollContainer,
  externalLinkContext,
}: FileDiffSectionProps) {
  const [collapsed, setCollapsed] = useState(false);
  const [expandUnchanged, setExpandUnchanged] = useState(false);
  const [localWordWrap, setLocalWordWrap] = useState<boolean | undefined>(undefined);
  const effectiveWordWrap = localWordWrap ?? wordWrap;
  const handleToggleCollapse = useCallback(() => setCollapsed((v) => !v), []);
  const handleToggleExpandUnchanged = useCallback(() => setExpandUnchanged((v) => !v), []);
  const handleToggleWordWrap = useCallback(
    () => setLocalWordWrap((v) => !(v ?? wordWrap)),
    [wordWrap],
  );
  const { isVisible, sentinelRef } = useLazyVisible(scrollContainer);
  // Force load when visible via intersection observer, or forceLoad is true
  const shouldRenderContent = isVisible || !!forceLoad;
  useScrollIntoViewOnSelect(isSelected, sectionRef, setCollapsed);
  // Auto-mark sends the composite key (matches the dialog's reviewed-set
  // shape) so cross-repo same-named files don't all get marked when one
  // scrolls past.
  const scrollSentinelRef = useAutoMarkOnScroll({
    autoMarkOnScroll,
    isReviewed,
    isStale,
    fileKey,
    onToggleReviewed,
    scrollContainer,
  });
  const handleCheckboxChange = useCallback(
    (checked: boolean | "indeterminate") => {
      onToggleReviewed(fileKey, checked === true);
    },
    [fileKey, onToggleReviewed],
  );
  const handleDiscard = useCallback(() => {
    onDiscard(fileKey);
  }, [fileKey, onDiscard]);

  const handleRevertBlock = useCallback(
    (filePath: string, info: RevertBlockInfo) =>
      revertBlock(sessionId, filePath, info, file.repository_name),
    [sessionId, file.repository_name],
  );

  const handleCommentRun = useCommentRunHandler(sessionId);

  const { enableExpansion, baseRef } = resolveDiffExpansion(
    file,
    externalLinkContext.baseBranchByRepo,
    externalLinkContext.fallbackBaseBranch,
  );

  return (
    <div ref={sectionRef} className="border-b border-border">
      <div ref={scrollSentinelRef} className="h-0" />
      <ReviewDiffHeader
        file={file}
        isReviewed={isReviewed}
        isStale={isStale}
        sessionId={sessionId}
        collapsed={collapsed}
        wordWrap={effectiveWordWrap}
        expandUnchanged={expandUnchanged}
        onCheckboxChange={handleCheckboxChange}
        onDiscard={handleDiscard}
        onOpenFile={onOpenFile}
        onPreviewMarkdown={onPreviewMarkdown}
        onToggleCollapse={handleToggleCollapse}
        onToggleExpandUnchanged={handleToggleExpandUnchanged}
        onToggleWordWrap={handleToggleWordWrap}
        {...externalLinkContext}
      />
      <div ref={sentinelRef} />
      {!collapsed &&
        renderDiffContent({
          shouldRender: shouldRenderContent,
          file,
          sessionId,
          wordWrap: effectiveWordWrap,
          expandUnchanged,
          enableExpansion,
          baseRef,
          onRevertBlock: handleRevertBlock,
          onCommentRun: handleCommentRun,
          onToggleExpandUnchanged: handleToggleExpandUnchanged,
        })}
    </div>
  );
}
