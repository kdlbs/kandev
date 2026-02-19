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
  comments?: DiffComment[];
  className?: string;
  compact?: boolean;
  hideHeader?: boolean;
  onOpenFile?: (filePath: string) => void;
  onRevert?: (filePath: string) => void;
  enableAcceptReject?: boolean;
  onRevertBlock?: (filePath: string, info: RevertBlockInfo) => void;
  wordWrap?: boolean;
  editable?: boolean;
  onModifiedContentChange?: (filePath: string, content: string) => void;
}

export const DiffViewerResolved = memo(function DiffViewerResolved(props: DiffViewerResolverProps) {
  const provider = useEditorProvider("diff-viewer");
  if (provider === "monaco") {
    // eslint-disable-next-line @typescript-eslint/no-unused-vars
    const { enableComments, ...rest } = props;
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
