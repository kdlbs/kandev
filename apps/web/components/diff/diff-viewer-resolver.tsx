"use client";

import { memo } from "react";
import { useEditorProvider } from "@/hooks/use-editor-resolver";
import {
  DiffViewer as PierreDiffViewer,
  DiffViewInline as PierreDiffViewInline,
} from "./diff-viewer";
import { MonacoDiffViewer } from "@/components/editors/monaco/monaco-diff-viewer";
import type { FileDiffData, DiffComment } from "@/lib/diff/types";
import type { RevertBlockInfo } from "./diff-viewer";
export type { RevertBlockInfo };

interface DiffViewerResolverProps {
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
  onPreviewMarkdown?: (filePath: string) => void;
  onRevert?: (filePath: string) => void;
  enableAcceptReject?: boolean;
  onRevertBlock?: (filePath: string, info: RevertBlockInfo) => void;
  wordWrap?: boolean;
  editable?: boolean;
  onModifiedContentChange?: (filePath: string, content: string) => void;
  /** Enable diff expansion (pierre-diffs only - Monaco has built-in expansion) */
  enableExpansion?: boolean;
  /** Base git ref for fetching old content (pierre-diffs only) */
  baseRef?: string;
  /** Controlled expand-unchanged state (pierre-diffs only) */
  expandUnchanged?: boolean;
  /** Callback when expand-unchanged is toggled (pierre-diffs only) */
  onToggleExpandUnchanged?: () => void;
  /** Multi-repo subpath for the file (e.g. "kandev"); empty for single-repo. */
  repo?: string;
}

export const DiffViewerResolved = memo(function DiffViewerResolved(props: DiffViewerResolverProps) {
  const provider = useEditorProvider("diff-viewer");
  if (provider === "monaco") {
    // Strip pierre-diffs-only props
    /* eslint-disable @typescript-eslint/no-unused-vars */
    const {
      enableComments,
      baseRef,
      enableExpansion,
      expandUnchanged,
      onToggleExpandUnchanged,
      onPreviewMarkdown,
      repo,
      ...rest
    } = props;
    /* eslint-enable @typescript-eslint/no-unused-vars */
    return <MonacoDiffViewer {...rest} />;
  }
  return <PierreDiffViewer {...props} />;
});

export function DiffViewInlineResolved({
  data,
  className,
}: {
  data: FileDiffData;
  className?: string;
}) {
  const provider = useEditorProvider("chat-diff");
  if (provider === "monaco") {
    return <MonacoDiffViewer data={data} compact hideHeader className={className} />;
  }
  return <PierreDiffViewInline data={data} className={className} />;
}
