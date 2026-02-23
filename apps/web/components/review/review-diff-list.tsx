"use client";

import { memo, useEffect, useRef, useState, useCallback } from "react";
import {
  IconAlertTriangle,
  IconArrowBackUp,
  IconChevronDown,
  IconChevronRight,
  IconPencil,
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
import type { ReviewFile } from "./types";

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
  fileRefs,
}: ReviewDiffListProps) {
  const scrollContainerRef = useRef<HTMLDivElement | null>(null);
  return (
    <div ref={scrollContainerRef} className="overflow-y-auto h-full">
      {files.map((file) => (
        <FileDiffSection
          key={file.path}
          file={file}
          isReviewed={reviewedFiles.has(file.path) && !staleFiles.has(file.path)}
          isStale={staleFiles.has(file.path)}
          sessionId={sessionId}
          autoMarkOnScroll={autoMarkOnScroll}
          wordWrap={wordWrap}
          isSelected={selectedFile === file.path}
          onToggleReviewed={onToggleReviewed}
          onDiscard={onDiscard}
          onOpenFile={onOpenFile}
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
  onToggleReviewed: (path: string, reviewed: boolean) => void;
  onDiscard: (path: string) => void;
  onOpenFile?: (filePath: string) => void;
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

type FileDiffHeaderProps = {
  file: ReviewFile;
  isReviewed: boolean;
  isStale: boolean;
  sessionId: string;
  collapsed: boolean;
  onCheckboxChange: (checked: boolean | "indeterminate") => void;
  onDiscard: () => void;
  onOpenFile?: (filePath: string) => void;
  onToggleCollapse: () => void;
};

function FileDiffHeader({
  file,
  isReviewed,
  isStale,
  sessionId,
  collapsed,
  onCheckboxChange,
  onDiscard,
  onOpenFile,
  onToggleCollapse,
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
      <div className="flex items-center gap-0.5">
        {onOpenFile && (
          <Tooltip>
            <TooltipTrigger asChild>
              <Button
                variant="ghost"
                size="sm"
                className="h-6 w-6 p-0 cursor-pointer opacity-60 hover:opacity-100"
                onClick={() => onOpenFile(file.path)}
              >
                <IconPencil className="h-3.5 w-3.5" />
              </Button>
            </TooltipTrigger>
            <TooltipContent>Edit</TooltipContent>
          </Tooltip>
        )}
        <FileActionsDropdown filePath={file.path} sessionId={sessionId} size="xs" />
        {file.source === "uncommitted" && (
          <Tooltip>
            <TooltipTrigger asChild>
              <Button
                variant="ghost"
                size="sm"
                className="h-6 w-6 p-0 cursor-pointer opacity-60 hover:opacity-100 hover:text-destructive"
                onClick={onDiscard}
              >
                <IconArrowBackUp className="h-3.5 w-3.5" />
              </Button>
            </TooltipTrigger>
            <TooltipContent>Revert changes</TooltipContent>
          </Tooltip>
        )}
      </div>
    </div>
  );
}

function FileDiffSection({
  file,
  isReviewed,
  isStale,
  sessionId,
  autoMarkOnScroll,
  wordWrap,
  isSelected,
  onToggleReviewed,
  onDiscard,
  onOpenFile,
  sectionRef,
  scrollContainer,
}: FileDiffSectionProps) {
  const [collapsed, setCollapsed] = useState(false);
  const handleToggleCollapse = useCallback(() => setCollapsed((v) => !v), []);
  useEffect(() => {
    setCollapsed(isReviewed);
  }, [isReviewed]);
  // Force-expand when the user explicitly navigates to this file
  useEffect(() => {
    if (isSelected) setCollapsed(false);
  }, [isSelected]);
  const { isVisible, sentinelRef } = useLazyVisible(scrollContainer);
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
    async (filePath: string, info: RevertBlockInfo) => {
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
        // Guard against no-op diffs ("No valid patches in input") from createTwoFilesPatch.
        if (!patch || !/^@@/m.test(patch)) return;

        await updateFileContent(client, sessionId, filePath, patch, hash);
      } catch (err) {
        console.error("Failed to revert change block:", err);
      }
    },
    [sessionId],
  );

  return (
    <div ref={sectionRef} className="border-b border-border">
      <div ref={scrollSentinelRef} className="h-0" />
      <FileDiffHeader
        file={file}
        isReviewed={isReviewed}
        isStale={isStale}
        sessionId={sessionId}
        collapsed={collapsed}
        onCheckboxChange={handleCheckboxChange}
        onDiscard={handleDiscard}
        onOpenFile={onOpenFile}
        onToggleCollapse={handleToggleCollapse}
      />
      <div ref={sentinelRef} />
      {!collapsed &&
        (isVisible && file.diff ? (
          <div className="">
            <FileDiffViewer
              filePath={file.path}
              diff={file.diff}
              status={file.status}
              enableComments
              enableAcceptReject
              onRevertBlock={handleRevertBlock}
              sessionId={sessionId}
              hideHeader
              wordWrap={wordWrap}
            />
          </div>
        ) : (
          <div className="flex items-center justify-center py-12 text-muted-foreground text-sm">
            Loading diff...
          </div>
        ))}
    </div>
  );
}
