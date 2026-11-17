"use client";

import type { RevertBlockInfo } from "@/components/diff";
import type { DiffComment } from "@/lib/diff/types";
import { ReviewFileComments } from "./review-file-comments";
import type { ReviewMarkdownPreview } from "./review-markdown-diff-preview";
import { ReviewMarkdownDiffPreviewContent } from "./review-markdown-diff-preview-content";
import { ReviewFileDiffContent, type ReviewFileDiffContentProps } from "./review-file-diff-content";
import { ReviewHtmlDiffPreview, type ReviewHtmlDiffPreviewProps } from "./review-html-diff-preview";
import type { ReviewFile } from "./types";

type FileCommentsState = {
  creating: boolean;
  onClose: () => void;
};

export type ReviewFileDiffBodyProps = {
  collapsed: boolean;
  file: ReviewFile;
  sessionId: string;
  fileComments: FileCommentsState;
  shouldRender: boolean;
  wordWrap: boolean;
  enableWalkthroughAnnotations: boolean;
  expandUnchanged: boolean;
  enableExpansion: boolean;
  baseRef: string;
  markdownPreview: ReviewMarkdownPreview | null;
  htmlPreview: ReviewHtmlDiffPreviewProps | null;
  onRevertBlock: (filePath: string, info: RevertBlockInfo) => void;
  onCommentRun: (comment: DiffComment) => void;
  onToggleExpandUnchanged: () => void;
};

function getDiffContentProps(props: ReviewFileDiffBodyProps): ReviewFileDiffContentProps {
  const {
    shouldRender,
    file,
    sessionId,
    wordWrap,
    enableWalkthroughAnnotations,
    expandUnchanged,
    enableExpansion,
    baseRef,
    onRevertBlock,
    onCommentRun,
    onToggleExpandUnchanged,
  } = props;
  return {
    shouldRender,
    file,
    sessionId,
    wordWrap,
    enableWalkthroughAnnotations,
    expandUnchanged,
    enableExpansion,
    baseRef,
    onRevertBlock,
    onCommentRun,
    onToggleExpandUnchanged,
  };
}

function ReviewFileDiffBodyContent(props: ReviewFileDiffBodyProps) {
  if (props.htmlPreview) return <ReviewHtmlDiffPreview {...props.htmlPreview} />;
  if (props.markdownPreview) {
    return <ReviewMarkdownDiffPreviewContent preview={props.markdownPreview} />;
  }
  return <ReviewFileDiffContent {...getDiffContentProps(props)} />;
}

export function ReviewFileDiffBody(props: ReviewFileDiffBodyProps) {
  const { collapsed, file, sessionId, fileComments } = props;
  if (collapsed) return null;
  return (
    <>
      <ReviewFileComments
        file={file}
        sessionId={sessionId}
        creating={fileComments.creating}
        onClose={fileComments.onClose}
      />
      <ReviewFileDiffBodyContent {...props} />
    </>
  );
}
