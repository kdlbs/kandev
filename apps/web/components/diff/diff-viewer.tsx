"use client";

import { useState, useRef, useCallback, memo, useEffect } from "react";
import { FileDiff } from "@pierre/diffs/react";
import { cn } from "@kandev/ui/lib/utils";
import type { FileDiffData, DiffComment } from "@/lib/diff/types";
import { useHunkHover } from "./use-hunk-hover";
import { useAnnotationRenderer } from "./use-diff-annotation-renderer";
import { useDiffOptions } from "./use-diff-options";
import { useDiffViewerState } from "./use-diff-viewer-state";

export type RevertBlockInfo = {
  /** 1-based line number in the new file where additions start */
  addStart: number;
  /** Number of addition lines to remove (0 for pure deletions) */
  addCount: number;
  /** Original lines to restore (empty for pure additions) */
  oldLines: string[];
};

interface DiffViewerProps {
  data: FileDiffData;
  enableComments?: boolean;
  sessionId?: string;
  onCommentAdd?: (comment: DiffComment) => void;
  onCommentDelete?: (commentId: string) => void;
  onCommentRun?: (comment: DiffComment) => void;
  comments?: DiffComment[];
  className?: string;
  compact?: boolean;
  hideHeader?: boolean;
  onOpenFile?: (filePath: string) => void;
  onRevert?: (filePath: string) => void;
  enableAcceptReject?: boolean;
  onRevertBlock?: (filePath: string, info: RevertBlockInfo) => Promise<void> | void;
  wordWrap?: boolean;
  /** Enable diff expansion (show expand up/down buttons at hunk separators) */
  enableExpansion?: boolean;
  /** Base git ref for fetching old content (e.g., "origin/main", "HEAD~1") */
  baseRef?: string;
}

const SCALAR_PROP_KEYS: (keyof DiffViewerProps)[] = [
  "enableComments",
  "sessionId",
  "compact",
  "hideHeader",
  "className",
  "onOpenFile",
  "onRevert",
  "enableAcceptReject",
  "onRevertBlock",
  "wordWrap",
  "enableExpansion",
  "baseRef",
];

const DATA_KEYS: (keyof FileDiffData)[] = ["filePath", "diff", "oldContent", "newContent"];

function areCommentsEqual(
  prev: DiffComment[] | undefined,
  next: DiffComment[] | undefined,
): boolean {
  if (prev === next) return true;
  if (!prev || !next || prev.length !== next.length) return false;
  return prev.every((c, i) => c.id === next[i].id && c.text === next[i].text);
}

/** Auto-load expansion content and return whether expansion can be used. */
function useAutoLoadExpansion(
  enableExpansion: boolean,
  state: ReturnType<typeof useDiffViewerState>,
): boolean {
  const { isExpansionContentLoaded, isExpansionLoading, expansionError, loadExpansionContent } =
    state;
  useEffect(() => {
    if (enableExpansion && !isExpansionContentLoaded && !isExpansionLoading && !expansionError) {
      void loadExpansionContent();
    }
  }, [
    enableExpansion,
    isExpansionContentLoaded,
    isExpansionLoading,
    expansionError,
    loadExpansionContent,
  ]);
  const hasValidData = !!(
    state.fileDiffMetadata?.oldLines?.length && state.fileDiffMetadata?.newLines?.length
  );
  return enableExpansion && isExpansionContentLoaded && hasValidData;
}

function arePropsEqual(prevProps: DiffViewerProps, nextProps: DiffViewerProps): boolean {
  for (const key of DATA_KEYS) {
    if (prevProps.data[key] !== nextProps.data[key]) return false;
  }
  for (const key of SCALAR_PROP_KEYS) {
    if (prevProps[key] !== nextProps[key]) return false;
  }
  return areCommentsEqual(prevProps.comments, nextProps.comments);
}

export const DiffViewer = memo(function DiffViewer({
  data,
  enableComments = false,
  sessionId,
  onCommentAdd,
  onCommentDelete,
  onCommentRun,
  comments: externalComments,
  className,
  compact = false,
  hideHeader = false,
  onOpenFile,
  onRevert,
  enableAcceptReject = false,
  onRevertBlock,
  wordWrap: wordWrapProp,
  enableExpansion = false,
  baseRef,
}: DiffViewerProps) {
  const [wordWrapLocal, setWordWrap] = useState(false);
  const wordWrap = wordWrapProp ?? wordWrapLocal;
  const [expandUnchanged, setExpandUnchanged] = useState(false);
  const toggleExpandUnchanged = useCallback(() => setExpandUnchanged((v) => !v), []);
  const wrapperRef = useRef<HTMLDivElement>(null);

  const state = useDiffViewerState({
    data,
    enableComments,
    enableAcceptReject,
    sessionId,
    onCommentAdd,
    onCommentDelete,
    onCommentRun,
    externalComments,
    onRevertBlock,
    enableExpansion,
    baseRef,
  });

  const { onLineEnter, onLineLeave, onButtonEnter, onButtonLeave } = useHunkHover({
    wrapperRef,
    changeLineMapRef: state.changeLineMapRef,
    hideTimeoutRef: state.hideTimeoutRef,
  });

  const renderAnnotation = useAnnotationRenderer({
    handleRevertBlock: state.handleRevertBlock,
    onButtonEnter,
    onButtonLeave,
    handleCommentSubmit: state.handleCommentSubmit,
    handleCommentSubmitAndRun: state.handleCommentSubmitAndRun,
    handleCommentUpdate: state.handleCommentUpdate,
    handleCommentDelete: state.handleCommentDelete,
    handleCommentRun: onCommentRun,
    setShowCommentForm: state.setShowCommentForm,
    setSelectedLines: state.setSelectedLines,
    setEditingComment: state.setEditingComment,
  });

  const showHeader = !hideHeader && !compact;
  const canUseExpansion = useAutoLoadExpansion(enableExpansion, state);

  const { options, renderHeaderMetadata, renderHoverUtility } = useDiffOptions({
    filePath: data.filePath,
    diff: data.diff,
    enableComments,
    showHeader,
    wordWrap,
    setWordWrap,
    handleLineSelectionEnd: state.handleLineSelectionEnd,
    onLineEnter,
    onLineLeave,
    onOpenFile,
    onRevert,
    enableExpansion: canUseExpansion,
    expandUnchanged,
    onToggleExpandUnchanged: canUseExpansion ? toggleExpandUnchanged : undefined,
  });

  const controlledSelection = state.showCommentForm ? state.selectedLines : null;

  if (!state.fileDiffMetadata) {
    return (
      <div
        className={cn("rounded-md  bg-muted/20 p-4 text-muted-foreground", "text-xs", className)}
      >
        No diff available
      </div>
    );
  }

  return (
    <div ref={wrapperRef} className={cn("diff-viewer", className)}>
      <FileDiff
        fileDiff={state.fileDiffMetadata}
        options={options}
        selectedLines={controlledSelection}
        lineAnnotations={state.annotations}
        renderAnnotation={renderAnnotation}
        renderHeaderMetadata={renderHeaderMetadata}
        renderHoverUtility={renderHoverUtility}
        className={cn("rounded-md ", "text-xs")}
      />
    </div>
  );
}, arePropsEqual);

/** Compact inline diff viewer for chat messages (Pierre implementation). */
export function DiffViewInline({ data, className }: { data: FileDiffData; className?: string }) {
  return <DiffViewer data={data} compact hideHeader className={className} />;
}
