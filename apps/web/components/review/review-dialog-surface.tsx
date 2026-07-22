"use client";

import { useRef } from "react";
import { Dialog, DialogContent, DialogTitle } from "@kandev/ui/dialog";
import { useReviewSidebarResize } from "@/hooks/use-review-sidebar-resize";
import type { TaskPR } from "@/lib/types/github";
import type { ReviewDialogViewState } from "./review-dialog";
import { ReviewDiffList } from "./review-diff-list";
import { ReviewFileTree } from "./review-file-tree";
import { ReviewPRDiffBoundary } from "./review-dialog-pr-state";
import { ReviewTopBar } from "./review-top-bar";

type ReviewDialogSurfaceProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  sessionId: string;
  baseBranch?: string;
  onOpenFile?: (filePath: string) => void;
  prs: TaskPR[];
  selectedPR: TaskPR | null;
  onSelectPR?: (pr: TaskPR) => void;
  prDiffLoading: boolean;
  prDiffError: string | null;
  onRetryPRDiff?: () => void;
  onRequestWalkthrough: () => void;
  state: ReviewDialogViewState;
};

function ReviewDialogDiffContent({
  sessionId,
  onOpenFile,
  selectedPR,
  prDiffLoading,
  prDiffError,
  onRetryPRDiff,
  state,
}: Pick<
  ReviewDialogSurfaceProps,
  | "sessionId"
  | "onOpenFile"
  | "selectedPR"
  | "prDiffLoading"
  | "prDiffError"
  | "onRetryPRDiff"
  | "state"
>) {
  return (
    <ReviewPRDiffBoundary
      selectedPR={selectedPR}
      loading={prDiffLoading}
      error={prDiffError}
      onRetry={onRetryPRDiff}
    >
      {state.filteredFiles.length > 0 ? (
        <ReviewDiffList
          files={state.filteredFiles}
          selectedFile={state.selectedFile}
          reviewedFiles={state.reviewedFiles}
          staleFiles={state.staleFiles}
          sessionId={sessionId}
          autoMarkOnScroll={state.autoMarkOnScroll}
          wordWrap={state.wordWrap}
          onToggleReviewed={state.handleToggleReviewed}
          onDiscard={state.handleDiscard}
          onOpenFile={onOpenFile}
          fileRefs={state.fileRefs}
        />
      ) : (
        <div className="flex h-full items-center justify-center text-sm text-muted-foreground">
          {state.filter.trim() ? "No files match the filter" : "No changes to review"}
        </div>
      )}
    </ReviewPRDiffBoundary>
  );
}

export function ReviewDialogSurface(props: ReviewDialogSurfaceProps) {
  const { open, onOpenChange, sessionId, state } = props;
  const splitRowRef = useRef<HTMLDivElement>(null);
  const sidebar = useReviewSidebarResize(splitRowRef, open, state.reviewSourceKey);

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent
        className="!max-w-[100vw] !w-[100vw] sm:!max-w-[80vw] sm:!w-[80vw] max-h-[85vh] h-[85vh] p-0 gap-0 flex flex-col shadow-2xl"
        showCloseButton={false}
        overlayClassName="bg-black/40"
      >
        <DialogTitle className="sr-only">Review Changes</DialogTitle>
        <ReviewTopBar
          sessionId={sessionId}
          reviewedCount={state.reviewedFiles.size}
          totalCount={state.allFiles.length}
          commentCount={state.totalCommentCount}
          baseBranch={props.baseBranch}
          splitView={state.splitView}
          onToggleSplitView={state.handleToggleSplitView}
          wordWrap={state.wordWrap}
          onToggleWordWrap={state.setWordWrap}
          onSendComments={state.handleSendComments}
          onClose={() => onOpenChange(false)}
          onRequestWalkthrough={props.onRequestWalkthrough}
          requestWalkthroughDisabled={state.allFiles.length === 0}
          getPendingComments={state.getPendingComments}
          markCommentsSent={state.markCommentsSent}
          prs={props.prs}
          selectedPR={props.selectedPR}
          onSelectPR={props.onSelectPR}
          prDiffLoading={props.prDiffLoading}
        />
        <div key={state.reviewSourceKey} ref={splitRowRef} className="flex min-h-0 flex-1">
          <div
            data-testid="review-dialog-sidebar"
            className="hidden flex-shrink-0 flex-col overflow-hidden border-r border-border sm:flex"
            style={{ width: `${sidebar.width}px` }}
          >
            <ReviewFileTree
              files={state.filteredFiles}
              reviewedFiles={state.reviewedFiles}
              staleFiles={state.staleFiles}
              commentCountByFile={state.commentCountByFile}
              selectedFile={state.selectedFile}
              filter={state.filter}
              onFilterChange={state.setFilter}
              onSelectFile={state.handleSelectFile}
              onToggleReviewed={state.handleToggleReviewed}
            />
          </div>
          <button
            data-testid="review-dialog-sidebar-resize"
            type="button"
            tabIndex={-1}
            aria-label="Resize file list"
            className="group relative hidden w-1 flex-shrink-0 cursor-col-resize bg-border p-0 transition-colors hover:bg-primary sm:block"
            {...sidebar.resizeHandleProps}
          >
            <span className="absolute inset-y-0 -left-1 -right-1" />
          </button>
          <div className="min-w-0 flex-1 overflow-hidden">
            <ReviewDialogDiffContent {...props} />
          </div>
        </div>
      </DialogContent>
    </Dialog>
  );
}
