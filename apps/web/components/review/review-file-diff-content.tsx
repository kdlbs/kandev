"use client";

import { DiffErrorBoundary, FileDiffViewer } from "@/components/diff";
import type { RevertBlockInfo } from "@/components/diff";
import { useTranslation } from "react-i18next";
import type { DiffComment } from "@/lib/diff/types";
import { diffSkipReasonLabel, hasTextualDiff, reviewDiffUnavailableLabel } from "./types";
import type { ReviewFile } from "./types";
import { ReviewDiffStatePlaceholder } from "./review-diff-state-placeholder";

type Translate = ReturnType<typeof useTranslation>["t"];

export type ReviewFileDiffContentProps = {
  shouldRender: boolean;
  file: ReviewFile;
  sessionId: string;
  wordWrap: boolean;
  enableWalkthroughAnnotations: boolean;
  expandUnchanged: boolean;
  enableExpansion: boolean;
  baseRef: string;
  onRevertBlock: (filePath: string, info: RevertBlockInfo) => void;
  onCommentRun: (comment: DiffComment) => void;
  onToggleExpandUnchanged: () => void;
};

function renderDiffContent(opts: ReviewFileDiffContentProps & { t: Translate }) {
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
    t,
  } = opts;
  if (file.diff_state === "pending" || file.diff_state === "unavailable") {
    return <ReviewDiffStatePlaceholder state={file.diff_state} />;
  }
  const hasText = hasTextualDiff(file);
  if (shouldRender && hasText) {
    return (
      <>
        <DiffErrorBoundary filePath={file.path}>
          <FileDiffViewer
            filePath={file.path}
            diff={file.diff}
            status={file.status}
            enableComments
            enableAcceptReject
            enableWalkthroughAnnotations={enableWalkthroughAnnotations}
            onRevertBlock={onRevertBlock}
            onCommentRun={onCommentRun}
            sessionId={sessionId}
            wordWrap={wordWrap}
            enableExpansion={enableExpansion}
            baseRef={baseRef}
            hideHeader
            expandUnchanged={expandUnchanged}
            onToggleExpandUnchanged={onToggleExpandUnchanged}
            repo={file.repository_name ?? ""}
          />
        </DiffErrorBoundary>
        {file.diff_skip_reason === "truncated" && (
          <div className="py-1 text-center text-xs text-muted-foreground border-t">
            {t("review:diffTruncated")}
          </div>
        )}
      </>
    );
  }
  const message = hasText
    ? diffSkipReasonLabel(file.diff_skip_reason)
    : reviewDiffUnavailableLabel(file);
  return (
    <div className="flex items-center justify-center py-12 text-muted-foreground text-sm">
      {message}
    </div>
  );
}

export function ReviewFileDiffContent(props: ReviewFileDiffContentProps) {
  const { t } = useTranslation();
  return renderDiffContent({ ...props, t });
}
