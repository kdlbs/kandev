"use client";

import { memo, useEffect, useRef, useState, useCallback } from "react";
import {
  IconAlertTriangle,
  IconArrowBackUp,
  IconChevronDown,
  IconChevronRight,
  IconCopy,
  IconFold,
  IconFoldDown,
  IconLayoutColumns,
  IconLayoutRows,
  IconPencil,
  IconTextWrap,
  IconEye,
} from "@tabler/icons-react";
import { Checkbox } from "@kandev/ui/checkbox";
import { Button } from "@kandev/ui/button";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { FileDiffViewer } from "@/components/diff";
import type { RevertBlockInfo } from "@/components/diff";
import { FileActionsDropdown } from "@/components/editors/file-actions-dropdown";
import { getWebSocketClient } from "@/lib/ws/connection";
import { requestFileContent, updateFileContent } from "@/lib/ws/workspace-files";
import { generateUnifiedDiff, calculateHash } from "@/lib/utils/file-diff";
import { useGlobalViewMode } from "@/hooks/use-global-view-mode";
import { useAppStore } from "@/components/state-provider";
import { useToast } from "@/components/toast-provider";
import { useRunComment } from "@/hooks/domains/comments/use-run-comment";
import type { DiffComment } from "@/lib/diff/types";
import type { ReviewFile } from "./types";

function isMarkdownPath(filePath: string): boolean {
  const ext = filePath.split(".").pop()?.toLowerCase();
  return ext === "md" || ext === "mdx";
}

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
  // Find index of selected file - we need to force-load all files up to it
  const selectedIndex = selectedFile ? files.findIndex((f) => f.path === selectedFile) : -1;
  return (
    <div ref={scrollContainerRef} className="overflow-y-auto h-full">
      {files.map((file, index) => (
        <FileDiffSection
          key={file.path}
          file={file}
          isReviewed={reviewedFiles.has(file.path) && !staleFiles.has(file.path)}
          isStale={staleFiles.has(file.path)}
          sessionId={sessionId}
          autoMarkOnScroll={autoMarkOnScroll}
          wordWrap={wordWrap}
          isSelected={selectedFile === file.path}
          forceLoad={selectedIndex >= 0 && index <= selectedIndex}
          onToggleReviewed={onToggleReviewed}
          onDiscard={onDiscard}
          onOpenFile={onOpenFile}
          onPreviewMarkdown={onPreviewMarkdown}
          sectionRef={fileRefs.get(file.path)}
          scrollContainer={scrollContainerRef}
        />
      ))}
    </div>
  );
});

type FileDiffSectionProps = {
  file: ReviewFile;
  isReviewed: boolean;
  isStale: boolean;
  sessionId: string;
  autoMarkOnScroll: boolean;
  wordWrap: boolean;
  isSelected?: boolean;
  forceLoad?: boolean;
  onToggleReviewed: (path: string, reviewed: boolean) => void;
  onDiscard: (path: string) => void;
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
  filePath: string;
  onToggleReviewed: (path: string, reviewed: boolean) => void;
  scrollContainer: React.RefObject<HTMLDivElement | null>;
};

function useAutoMarkOnScroll({
  autoMarkOnScroll,
  isReviewed,
  isStale,
  filePath,
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
          console.debug("[review] auto-mark reviewed:", filePath);
          onToggleReviewed(filePath, true);
        }
      },
      { threshold: 0, root },
    );
    observer.observe(sentinel);
    return () => observer.disconnect();
  }, [autoMarkOnScroll, filePath, isReviewed, isStale, onToggleReviewed, scrollContainer]);
  return scrollSentinelRef;
}

const iconBtn = "h-6 w-6 p-0 cursor-pointer opacity-60 hover:opacity-100";
const iconBtnActive = "h-6 w-6 p-0 cursor-pointer opacity-100 bg-muted";

type FileDiffToolbarProps = {
  diff: string;
  filePath: string;
  sessionId: string;
  source: string;
  wordWrap: boolean;
  expandUnchanged: boolean;
  onDiscard: () => void;
  onOpenFile?: (filePath: string) => void;
  onPreviewMarkdown?: (filePath: string) => void;
  onToggleExpandUnchanged: () => void;
  onToggleWordWrap: () => void;
};

function ToolbarIconBtn({
  onClick,
  tooltip,
  active,
  children,
  className,
}: {
  onClick: () => void;
  tooltip: string;
  active?: boolean;
  children: React.ReactNode;
  className?: string;
}) {
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <Button
          type="button"
          aria-label={tooltip}
          aria-pressed={active}
          variant="ghost"
          size="sm"
          className={className ?? (active ? iconBtnActive : iconBtn)}
          onClick={onClick}
        >
          {children}
        </Button>
      </TooltipTrigger>
      <TooltipContent>{tooltip}</TooltipContent>
    </Tooltip>
  );
}

function FileDiffToolbar(props: FileDiffToolbarProps) {
  const {
    diff,
    filePath,
    sessionId,
    source,
    wordWrap,
    expandUnchanged,
    onDiscard,
    onOpenFile,
    onPreviewMarkdown,
    onToggleExpandUnchanged,
    onToggleWordWrap,
  } = props;
  const [globalViewMode, setGlobalViewMode] = useGlobalViewMode();
  const handleCopyDiff = useCallback(() => {
    navigator.clipboard.writeText(diff || "");
  }, [diff]);
  const handleToggleViewMode = useCallback(
    () => setGlobalViewMode(globalViewMode === "split" ? "unified" : "split"),
    [globalViewMode, setGlobalViewMode],
  );
  return (
    <div className="flex items-center gap-0.5">
      <ToolbarIconBtn onClick={handleCopyDiff} tooltip="Copy diff">
        <IconCopy className="h-3.5 w-3.5" />
      </ToolbarIconBtn>
      <ToolbarIconBtn
        onClick={onToggleExpandUnchanged}
        tooltip={expandUnchanged ? "Collapse unchanged" : "Expand all"}
        active={expandUnchanged}
      >
        {expandUnchanged ? (
          <IconFold className="h-3.5 w-3.5" />
        ) : (
          <IconFoldDown className="h-3.5 w-3.5" />
        )}
      </ToolbarIconBtn>
      <ToolbarIconBtn onClick={onToggleWordWrap} tooltip="Toggle word wrap" active={wordWrap}>
        <IconTextWrap className="h-3.5 w-3.5" />
      </ToolbarIconBtn>
      <ToolbarIconBtn
        onClick={handleToggleViewMode}
        tooltip={globalViewMode === "split" ? "Switch to unified view" : "Switch to split view"}
      >
        {globalViewMode === "split" ? (
          <IconLayoutRows className="h-3.5 w-3.5" />
        ) : (
          <IconLayoutColumns className="h-3.5 w-3.5" />
        )}
      </ToolbarIconBtn>
      {onPreviewMarkdown && isMarkdownPath(filePath) && (
        <ToolbarIconBtn onClick={() => onPreviewMarkdown(filePath)} tooltip="Preview markdown">
          <IconEye className="h-3.5 w-3.5" />
        </ToolbarIconBtn>
      )}
      {onOpenFile && (
        <ToolbarIconBtn onClick={() => onOpenFile(filePath)} tooltip="Edit">
          <IconPencil className="h-3.5 w-3.5" />
        </ToolbarIconBtn>
      )}
      <FileActionsDropdown filePath={filePath} sessionId={sessionId} size="xs" />
      {source === "uncommitted" && (
        <ToolbarIconBtn
          onClick={onDiscard}
          tooltip="Revert changes"
          className="h-6 w-6 p-0 cursor-pointer opacity-60 hover:opacity-100 hover:text-destructive"
        >
          <IconArrowBackUp className="h-3.5 w-3.5" />
        </ToolbarIconBtn>
      )}
    </div>
  );
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

async function revertBlock(sessionId: string, filePath: string, info: RevertBlockInfo) {
  const client = getWebSocketClient();
  if (!client) return;
  try {
    const response = await requestFileContent(client, sessionId, filePath);
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

function FileDiffSection({
  file,
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
  // Force load when: visible via intersection observer, or forceLoad is true (all files up to selected)
  const shouldRenderContent = isVisible || forceLoad;
  useScrollIntoViewOnSelect(isSelected, sectionRef, setCollapsed);
  const scrollSentinelRef = useAutoMarkOnScroll({
    autoMarkOnScroll,
    isReviewed,
    isStale,
    filePath: file.path,
    onToggleReviewed,
    scrollContainer,
  });
  const handleCheckboxChange = useCallback(
    (checked: boolean | "indeterminate") => {
      onToggleReviewed(file.path, checked === true);
    },
    [file.path, onToggleReviewed],
  );
  const handleDiscard = useCallback(() => {
    onDiscard(file.path);
  }, [file.path, onDiscard]);

  const handleRevertBlock = useCallback(
    (filePath: string, info: RevertBlockInfo) => revertBlock(sessionId, filePath, info),
    [sessionId],
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
        (shouldRenderContent && file.diff ? (
          <FileDiffViewer
            filePath={file.path}
            diff={file.diff}
            status={file.status}
            enableComments
            enableAcceptReject
            onRevertBlock={handleRevertBlock}
            onCommentRun={handleCommentRun}
            sessionId={sessionId}
            wordWrap={effectiveWordWrap}
            enableExpansion={true}
            baseRef="HEAD"
            hideHeader
            expandUnchanged={expandUnchanged}
            onToggleExpandUnchanged={handleToggleExpandUnchanged}
          />
        ) : (
          <div className="flex items-center justify-center py-12 text-muted-foreground text-sm">
            Loading diff...
          </div>
        ))}
    </div>
  );
}
