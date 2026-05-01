"use client";

import { memo, useEffect, useMemo, useRef, useState, useCallback } from "react";
import { IconAlertTriangle, IconChevronDown, IconChevronRight } from "@tabler/icons-react";
import { Checkbox } from "@kandev/ui/checkbox";
import { FileDiffViewer } from "@/components/diff";
import type { RevertBlockInfo } from "@/components/diff";
import { getWebSocketClient } from "@/lib/ws/connection";
import { requestFileContent, updateFileContent } from "@/lib/ws/workspace-files";
import { generateUnifiedDiff, calculateHash } from "@/lib/utils/file-diff";
import { useAppStore } from "@/components/state-provider";
import { useToast } from "@/components/toast-provider";
import { useRunComment } from "@/hooks/domains/comments/use-run-comment";
import type { DiffComment } from "@/lib/diff/types";
import { diffSkipReasonLabel, reviewFileKey } from "./types";
import type { ReviewFile } from "./types";
import { RepoGroupHeader } from "./review-diff-list-groups";
import { FileDiffToolbar } from "./review-diff-toolbar";
import { groupByRepositoryName } from "@/lib/group-by-repo";

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
  onOpenFile?: (filePath: string) => void;
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
  onOpenFile?: (filePath: string) => void;
  onPreviewMarkdown?: (filePath: string) => void;
  sectionRef?: React.RefObject<HTMLDivElement | null>;
  scrollContainer: React.RefObject<HTMLDivElement | null>;
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

type FileDiffHeaderProps = {
  file: ReviewFile;
  isReviewed: boolean;
  isStale: boolean;
  sessionId: string;
  collapsed: boolean;
  wordWrap: boolean;
  expandUnchanged: boolean;
  onCheckboxChange: (checked: boolean | "indeterminate") => void;
  onDiscard: () => void;
  onOpenFile?: (filePath: string) => void;
  onPreviewMarkdown?: (filePath: string) => void;
  onToggleCollapse: () => void;
  onToggleExpandUnchanged: () => void;
  onToggleWordWrap: () => void;
};

function FileDiffHeader({
  file,
  isReviewed,
  isStale,
  collapsed,
  wordWrap,
  expandUnchanged,
  sessionId,
  onCheckboxChange,
  onDiscard,
  onOpenFile,
  onPreviewMarkdown,
  onToggleCollapse,
  onToggleExpandUnchanged,
  onToggleWordWrap,
}: FileDiffHeaderProps) {
  return (
    <div className="sticky top-0 z-10 flex items-center gap-2 px-4 py-2 bg-card/95 backdrop-blur-sm border-b border-border/50">
      <Checkbox
        checked={isReviewed}
        onCheckedChange={onCheckboxChange}
        className="h-4 w-4 cursor-pointer"
      />
      <button
        onClick={onToggleCollapse}
        className="flex items-center gap-1.5 flex-1 min-w-0 cursor-pointer text-left hover:text-foreground"
      >
        {collapsed ? (
          <IconChevronRight className="h-3.5 w-3.5 shrink-0 text-muted-foreground" />
        ) : (
          <IconChevronDown className="h-3.5 w-3.5 shrink-0 text-muted-foreground" />
        )}
        <span className="text-[13px] font-medium truncate">{file.path}</span>
      </button>
      {isStale && (
        <span className="flex items-center gap-1 text-xs text-yellow-500">
          <IconAlertTriangle className="h-3.5 w-3.5" />
          changed
        </span>
      )}
      <span className="text-xs text-muted-foreground">
        {file.additions > 0 && <span className="text-emerald-500">+{file.additions}</span>}
        {file.additions > 0 && file.deletions > 0 && " / "}
        {file.deletions > 0 && <span className="text-rose-500">-{file.deletions}</span>}
      </span>
      <FileDiffToolbar
        diff={file.diff}
        filePath={file.path}
        sessionId={sessionId}
        source={file.source}
        wordWrap={wordWrap}
        expandUnchanged={expandUnchanged}
        onDiscard={onDiscard}
        onOpenFile={onOpenFile}
        onPreviewMarkdown={onPreviewMarkdown}
        onToggleExpandUnchanged={onToggleExpandUnchanged}
        onToggleWordWrap={onToggleWordWrap}
      />
    </div>
  );
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

function renderDiffContent(opts: {
  shouldRender: boolean;
  file: ReviewFile;
  sessionId: string;
  wordWrap: boolean;
  expandUnchanged: boolean;
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
    onRevertBlock,
    onCommentRun,
    onToggleExpandUnchanged,
  } = opts;
  if (shouldRender && file.diff) {
    return (
      <>
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
          enableExpansion={true}
          baseRef="HEAD"
          hideHeader
          expandUnchanged={expandUnchanged}
          onToggleExpandUnchanged={onToggleExpandUnchanged}
          repo={file.repository_name}
        />
        {file.diff_skip_reason === "truncated" && (
          <div className="py-1 text-center text-xs text-muted-foreground border-t">
            Diff truncated — showing first 256 KB
          </div>
        )}
      </>
    );
  }
  return (
    <div className="flex items-center justify-center py-12 text-muted-foreground text-sm">
      {diffSkipReasonLabel(file.diff_skip_reason)}
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

  return (
    <div ref={sectionRef} className="border-b border-border">
      <div ref={scrollSentinelRef} className="h-0" />
      <FileDiffHeader
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
      />
      <div ref={sentinelRef} />
      {!collapsed &&
        renderDiffContent({
          shouldRender: shouldRenderContent,
          file,
          sessionId,
          wordWrap: effectiveWordWrap,
          expandUnchanged,
          onRevertBlock: handleRevertBlock,
          onCommentRun: handleCommentRun,
          onToggleExpandUnchanged: handleToggleExpandUnchanged,
        })}
    </div>
  );
}
