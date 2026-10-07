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

function renderPopulatedDiff(
  opts: ReviewFileDiffContentProps & { t: Translate },
  retainedState: "pending" | "unavailable",
) {
  const {
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
  return (
    <>
      {file.display_stale && <ReviewDiffStatePlaceholder state={retainedState} retainedContent />}
      <DiffErrorBoundary filePath={file.path}>
        <FileDiffViewer
          key={JSON.stringify([
            sessionId,
            file.source,
            file.repository_id ?? file.repository_name ?? "",
            file.change_layer ?? "",
            file.display_scope_key ?? file.base_ref ?? "",
          ])}
          filePath={file.path}
          diff={file.diff}
          status={file.status}
          enableComments={!file.display_stale}
          enableAcceptReject={!file.display_stale}
          enableWalkthroughAnnotations={enableWalkthroughAnnotations && !file.display_stale}
          onRevertBlock={file.display_stale ? undefined : onRevertBlock}
          onCommentRun={onCommentRun}
          sessionId={sessionId}
          wordWrap={wordWrap}
          enableExpansion={enableExpansion && !file.display_stale}
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

function renderStaleFallback(file: ReviewFile, retainedState: "pending" | "unavailable") {
  const message = file.diff_skip_reason
    ? diffSkipReasonLabel(file.diff_skip_reason)
    : reviewDiffUnavailableLabel(file);
  return (
    <>
      <ReviewDiffStatePlaceholder state={retainedState} retainedContent />
      <div className="flex items-center justify-center py-12 text-muted-foreground text-sm">
        {message}
      </div>
    </>
  );
}

function renderEmptyFallback(file: ReviewFile) {
  const message = hasTextualDiff(file)
    ? diffSkipReasonLabel(file.diff_skip_reason)
    : reviewDiffUnavailableLabel(file);
  return (
    <div className="flex items-center justify-center py-12 text-muted-foreground text-sm">
      {message}
    </div>
  );
}

function renderDiffContent(opts: ReviewFileDiffContentProps & { t: Translate }) {
  const { shouldRender, file } = opts;
  const detailUnavailable = file.diff_state === "pending" || file.diff_state === "unavailable";
  const retainedState = file.diff_state === "unavailable" ? "unavailable" : "pending";
  if (detailUnavailable && !file.display_stale) {
    return <ReviewDiffStatePlaceholder state={retainedState} />;
  }
  if (shouldRender && hasTextualDiff(file)) {
    return renderPopulatedDiff(opts, retainedState);
  }
  if (file.display_stale) return renderStaleFallback(file, retainedState);
  return renderEmptyFallback(file);
}

export function ReviewFileDiffContent(props: ReviewFileDiffContentProps) {
  const { t } = useTranslation();
  return renderDiffContent({ ...props, t });
}
