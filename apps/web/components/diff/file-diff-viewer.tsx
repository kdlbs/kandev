"use client";

import { memo, useMemo } from "react";
import { DiffViewerResolved as DiffViewer } from "./diff-viewer-resolver";
import { transformGitDiff } from "@/lib/diff";
import type { DiffComment } from "@/lib/diff/types";
import type { RevertBlockInfo } from "./diff-viewer-resolver";

interface FileDiffViewerProps {
  filePath: string;
  diff: string;
  status?: string;
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
  onPreviewMarkdown?: (filePath: string) => void;
  onRevert?: (filePath: string) => void;
  enableAcceptReject?: boolean;
  onRevertBlock?: (filePath: string, info: RevertBlockInfo) => void;
  wordWrap?: boolean;
  /** Enable diff expansion (show expand up/down buttons at hunk separators) */
  enableExpansion?: boolean;
  /** Base git ref for fetching old content (e.g., "origin/main", "HEAD~1") */
  baseRef?: string;
  /** Controlled expand-unchanged state */
  expandUnchanged?: boolean;
  /** Callback when expand-unchanged is toggled (controlled mode) */
  onToggleExpandUnchanged?: () => void;
  /** Multi-repo subpath for the file (e.g. "kandev"); empty for single-repo. */
  repo?: string;
}

/**
 * Wrapper around DiffViewer that handles data transformation with memoization.
 *
 * Use this component when you have raw diff data (filePath, diff, status).
 * The transformation to FileDiffData is memoized internally, preventing
 * unnecessary re-renders when parent components update.
 *
 * This is the recommended component to use for rendering git diffs.
 */
export const FileDiffViewer = memo(function FileDiffViewer({
  filePath,
  diff,
  status = "M",
  enableComments,
  sessionId,
  onCommentAdd,
  onCommentDelete,
  onCommentRun,
  comments,
  className,
  compact,
  hideHeader,
  onOpenFile,
  onPreviewMarkdown,
  onRevert,
  enableAcceptReject,
  onRevertBlock,
  wordWrap,
  enableExpansion,
  baseRef,
  expandUnchanged,
  onToggleExpandUnchanged,
  repo,
}: FileDiffViewerProps) {
  const data = useMemo(() => transformGitDiff(filePath, diff, status), [filePath, diff, status]);

  return (
    <DiffViewer
      data={data}
      enableComments={enableComments}
      sessionId={sessionId}
      onCommentAdd={onCommentAdd}
      onCommentDelete={onCommentDelete}
      onCommentRun={onCommentRun}
      comments={comments}
      className={className}
      compact={compact}
      hideHeader={hideHeader}
      onOpenFile={onOpenFile}
      onPreviewMarkdown={onPreviewMarkdown}
      onRevert={onRevert}
      enableAcceptReject={enableAcceptReject}
      onRevertBlock={onRevertBlock}
      wordWrap={wordWrap}
      enableExpansion={enableExpansion}
      baseRef={baseRef}
      expandUnchanged={expandUnchanged}
      onToggleExpandUnchanged={onToggleExpandUnchanged}
      repo={repo}
    />
  );
});
