'use client';

import { memo, useEffect, useRef, useState, useCallback } from 'react';
import {
  IconAlertTriangle,
  IconArrowBackUp,
  IconPencil,
} from '@tabler/icons-react';
import { Checkbox } from '@kandev/ui/checkbox';
import { Button } from '@kandev/ui/button';
import { Tooltip, TooltipContent, TooltipTrigger } from '@kandev/ui/tooltip';
import { FileDiffViewer } from '@/components/diff';
import { FileActionsDropdown } from '@/components/editors/file-actions-dropdown';
import type { ReviewFile } from './types';

type ReviewDiffListProps = {
  files: ReviewFile[];
  reviewedFiles: Set<string>;
  staleFiles: Set<string>;
  sessionId: string;
  autoMarkOnScroll: boolean;
  wordWrap: boolean;
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
  onToggleReviewed: (path: string, reviewed: boolean) => void;
  onDiscard: (path: string) => void;
  onOpenFile?: (filePath: string) => void;
  sectionRef?: React.RefObject<HTMLDivElement | null>;
  scrollContainer: React.RefObject<HTMLDivElement | null>;
};

function FileDiffSection({
  file,
  isReviewed,
  isStale,
  sessionId,
  autoMarkOnScroll,
  wordWrap,
  onToggleReviewed,
  onDiscard,
  onOpenFile,
  sectionRef,
  scrollContainer,
}: FileDiffSectionProps) {
  const [isVisible, setIsVisible] = useState(false);
  const sentinelRef = useRef<HTMLDivElement | null>(null);
  const autoMarkedRef = useRef(false);

  // Lazy rendering: mount the diff viewer only when section becomes visible
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
      { rootMargin: '200px 0px', root: scrollContainer.current }
    );

    observer.observe(sentinel);
    return () => observer.disconnect();
  }, [scrollContainer]);

  // Auto-mark on scroll: when the file section leaves the top of the scroll container
  const scrollSentinelRef = useRef<HTMLDivElement | null>(null);

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
        // File has scrolled above the scroll container (not intersecting + bounding rect is above root)
        if (!entry.isIntersecting && entry.boundingClientRect.top < root.getBoundingClientRect().top && !autoMarkedRef.current) {
          autoMarkedRef.current = true;
          console.debug('[review] auto-mark reviewed:', file.path);
          onToggleReviewed(file.path, true);
        }
      },
      { threshold: 0, root }
    );

    observer.observe(sentinel);
    return () => observer.disconnect();
  }, [autoMarkOnScroll, file.path, isReviewed, isStale, onToggleReviewed, scrollContainer]);

  const handleCheckboxChange = useCallback(
    (checked: boolean | 'indeterminate') => {
      onToggleReviewed(file.path, checked === true);
    },
    [file.path, onToggleReviewed]
  );

  const handleDiscard = useCallback(() => {
    onDiscard(file.path);
  }, [file.path, onDiscard]);

  return (
    <div ref={sectionRef} className="border-b border-border">
      {/* Sentinel for auto-mark-on-scroll */}
      <div ref={scrollSentinelRef} className="h-0" />

      {/* Sticky file header */}
      <div className="sticky top-0 z-10 flex items-center gap-2 px-4 py-2 bg-card/95 backdrop-blur-sm border-b border-border/50">
        <Checkbox
          checked={isReviewed}
          onCheckedChange={handleCheckboxChange}
          className="h-4 w-4 cursor-pointer"
        />
        <span className="text-sm font-medium truncate">{file.path}</span>

        <div className="flex-1" />

        {isStale && (
          <span className="flex items-center gap-1 text-xs text-yellow-500">
            <IconAlertTriangle className="h-3.5 w-3.5" />
            changed
          </span>
        )}

        <span className="text-xs text-muted-foreground">
          {file.additions > 0 && <span className="text-emerald-500">+{file.additions}</span>}
          {file.additions > 0 && file.deletions > 0 && ' / '}
          {file.deletions > 0 && <span className="text-rose-500">-{file.deletions}</span>}
        </span>

        {/* File actions */}
        <div className="flex items-center gap-0.5">
          {/* Edit in editor tab */}
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

          {/* Open with external editor / copy path / open folder */}
          <FileActionsDropdown filePath={file.path} sessionId={sessionId} size="xs" />

          {/* Discard changes */}
          {file.source === 'uncommitted' && (
            <Tooltip>
              <TooltipTrigger asChild>
                <Button
                  variant="ghost"
                  size="sm"
                  className="h-6 w-6 p-0 cursor-pointer opacity-60 hover:opacity-100 hover:text-destructive"
                  onClick={handleDiscard}
                >
                  <IconArrowBackUp className="h-3.5 w-3.5" />
                </Button>
              </TooltipTrigger>
              <TooltipContent>Revert changes</TooltipContent>
            </Tooltip>
          )}
        </div>
      </div>

      {/* Sentinel for lazy loading */}
      <div ref={sentinelRef} />

      {/* Diff content (lazy rendered) */}
      {isVisible && file.diff ? (
        <div className="px-2 py-2">
          <FileDiffViewer
            filePath={file.path}
            diff={file.diff}
            status={file.status}
            enableComments
            sessionId={sessionId}
            hideHeader
            wordWrap={wordWrap}
          />
        </div>
      ) : (
        <div className="flex items-center justify-center py-12 text-muted-foreground text-sm">
          Loading diff...
        </div>
      )}
    </div>
  );
}
