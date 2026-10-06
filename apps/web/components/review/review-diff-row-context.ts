import type { RefObject } from "react";
import type { ReviewExternalLinkContext } from "./review-diff-header";
import type { ReviewDiffListProps } from "./review-diff-list";

type ReviewDiffRowProps = Pick<
  ReviewDiffListProps,
  | "files"
  | "reviewedFiles"
  | "staleFiles"
  | "sessionId"
  | "autoMarkOnScroll"
  | "wordWrap"
  | "enableWalkthroughAnnotations"
  | "selectedFile"
  | "onToggleReviewed"
  | "onDiscard"
  | "onOpenFile"
  | "onPreviewMarkdown"
  | "previewedFiles"
  | "onToggleMarkdownPreview"
  | "fileRefs"
>;

export type ReviewDiffRowContext = ReviewDiffRowProps & {
  selectedIndex: number;
  showRepoHeaders: boolean;
  scrollContainer: RefObject<HTMLDivElement | null>;
  suppressAutoMark: RefObject<boolean>;
  externalLinkContext: ReviewExternalLinkContext;
};

type ReviewDiffRowDerivedContext = Pick<
  ReviewDiffRowContext,
  | "selectedIndex"
  | "showRepoHeaders"
  | "scrollContainer"
  | "suppressAutoMark"
  | "externalLinkContext"
>;

export function buildReviewDiffRowContext(
  props: ReviewDiffRowProps,
  derived: ReviewDiffRowDerivedContext,
): ReviewDiffRowContext {
  return { ...props, ...derived };
}
