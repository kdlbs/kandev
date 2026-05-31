"use client";

import { PanelBody } from "./panel-primitives";
import { DiscardDialog, AmendDialog, ResetDialog } from "./changes-panel-dialogs";
import {
  FileListSection,
  CommitsSection,
  ReviewProgressBar,
  PRFilesSection,
} from "./changes-panel-timeline";
import { mergeCommits, firstVisibleSection } from "./changes-panel-helpers";
import type { ChangesPanelBodyProps } from "./changes-panel-data";

function ChangesPanelDialogsSection({
  dialogs,
  isLoading,
}: Pick<ChangesPanelBodyProps, "dialogs" | "isLoading">) {
  return (
    <>
      <DiscardDialog
        open={dialogs.showDiscardDialog}
        onOpenChange={dialogs.setShowDiscardDialog}
        fileToDiscard={dialogs.fileToDiscard}
        filesToDiscard={dialogs.filesToDiscard}
        onConfirm={dialogs.handleDiscardConfirm}
      />
      <AmendDialog
        open={dialogs.amendDialogOpen}
        onOpenChange={dialogs.setAmendDialogOpen}
        amendMessage={dialogs.amendMessage}
        onAmendMessageChange={dialogs.setAmendMessage}
        onAmend={dialogs.handleAmend}
        isLoading={isLoading}
      />
      <ResetDialog
        open={dialogs.resetDialogOpen}
        onOpenChange={dialogs.setResetDialogOpen}
        commitSha={dialogs.resetCommitSha}
        onReset={dialogs.handleReset}
        isLoading={isLoading}
      />
    </>
  );
}

type TimelineProps = Pick<
  ChangesPanelBodyProps,
  | "hasAnything"
  | "hasUnstaged"
  | "hasStaged"
  | "hasCommits"
  | "hasPRFiles"
  | "canPush"
  | "canCreatePR"
  | "existingPrUrl"
  | "unstagedFiles"
  | "stagedFiles"
  | "prFiles"
  | "prCommits"
  | "commits"
  | "pendingStageFiles"
  | "aheadCount"
  | "isLoading"
  | "loadingOperation"
  | "dialogs"
  | "onOpenDiffFile"
  | "onEditFile"
  | "onOpenCommitDetail"
  | "onRevertCommit"
  | "onStageAll"
  | "onUnstageAll"
  | "onStage"
  | "onUnstage"
  | "onBulkStage"
  | "onBulkUnstage"
  | "onBulkDiscard"
  | "onPush"
  | "onForcePush"
  | "onRepoStageAll"
  | "onRepoUnstageAll"
  | "onRepoCommit"
  | "onRepoPush"
  | "onRepoCreatePR"
  | "repoDisplayName"
  | "perRepoStatus"
  | "prByRepo"
>;

type WorkingTreeProps = Pick<
  TimelineProps,
  | "hasUnstaged"
  | "hasStaged"
  | "unstagedFiles"
  | "stagedFiles"
  | "pendingStageFiles"
  | "loadingOperation"
  | "dialogs"
  | "onOpenDiffFile"
  | "onEditFile"
  | "onStageAll"
  | "onUnstageAll"
  | "onStage"
  | "onUnstage"
  | "onBulkStage"
  | "onBulkUnstage"
  | "onBulkDiscard"
  | "onRepoStageAll"
  | "onRepoUnstageAll"
  | "onRepoCommit"
  | "repoDisplayName"
> & { isLastUnstaged: boolean; isLastStaged: boolean };

function WorkingTreeSections(props: WorkingTreeProps) {
  const isBulkOp = props.pendingStageFiles.size === 0;
  return (
    <>
      {props.hasUnstaged && (
        <FileListSection
          variant="unstaged"
          files={props.unstagedFiles}
          pendingStageFiles={props.pendingStageFiles}
          isLast={props.isLastUnstaged}
          actionLabel="Stage all"
          isActionLoading={isBulkOp && props.loadingOperation === "stage"}
          onAction={props.onStageAll}
          onOpenDiff={props.onOpenDiffFile}
          onEditFile={props.onEditFile}
          onStage={props.onStage}
          onUnstage={props.onUnstage}
          onDiscard={props.dialogs.handleDiscardClick}
          onBulkStage={props.onBulkStage}
          onBulkDiscard={props.onBulkDiscard}
          onRepoAction={props.onRepoStageAll}
          repoDisplayName={props.repoDisplayName}
        />
      )}
      {props.hasStaged && (
        <FileListSection
          variant="staged"
          files={props.stagedFiles}
          pendingStageFiles={props.pendingStageFiles}
          isLast={props.isLastStaged}
          actionLabel="Commit"
          isActionLoading={props.loadingOperation === "commit"}
          onAction={() => props.dialogs.openCommitDialog()}
          secondaryActionLabel="Unstage all"
          isSecondaryActionLoading={isBulkOp && props.loadingOperation === "unstage"}
          onSecondaryAction={props.onUnstageAll}
          onOpenDiff={props.onOpenDiffFile}
          onEditFile={props.onEditFile}
          onStage={props.onStage}
          onUnstage={props.onUnstage}
          onDiscard={props.dialogs.handleDiscardClick}
          onBulkUnstage={props.onBulkUnstage}
          onBulkDiscard={props.onBulkDiscard}
          onRepoAction={props.onRepoCommit}
          onRepoSecondaryAction={props.onRepoUnstageAll}
          repoDisplayName={props.repoDisplayName}
        />
      )}
    </>
  );
}

function ChangesPanelTimeline(props: TimelineProps) {
  if (!props.hasAnything) {
    return (
      <div className="flex items-center justify-center h-full text-muted-foreground text-xs">
        Your changed files will appear here
      </div>
    );
  }

  const mergedCommits = mergeCommits(props.commits, props.prCommits);
  const hasMergedCommits = mergedCommits.length > 0;
  const hasLocalChanges = props.hasUnstaged || props.hasStaged;
  const showCommits = props.hasStaged || props.hasCommits;
  const showCommitsList = props.hasStaged || hasMergedCommits;
  const hasSomethingAfterStaged = (props.hasPRFiles && hasLocalChanges) || showCommitsList;
  // Auto-expand the first (topmost) visible section so the panel never opens
  // looking empty (e.g. review mode: PR + Commits both collapsed). Unstaged /
  // Staged keep their always-expanded default; PR and Commits are gated. Large
  // PR diffs (>5 files) skip PR Changes and expand Commits instead.
  const firstSection = firstVisibleSection({
    hasPRFiles: props.hasPRFiles,
    hasUnstaged: props.hasUnstaged,
    hasStaged: props.hasStaged,
    showCommitsList,
    prFileCount: props.prFiles.length,
  });

  return (
    <div className="flex flex-col">
      {props.hasPRFiles && !hasLocalChanges && (
        <div data-testid="pr-files-section">
          <PRFilesSection
            files={props.prFiles}
            isLast={!showCommitsList}
            onOpenDiff={props.onOpenDiffFile}
            repoDisplayName={props.repoDisplayName}
            defaultCollapsed={firstSection !== "pr"}
          />
        </div>
      )}

      <WorkingTreeSections
        {...props}
        isLastUnstaged={!props.hasStaged && !hasSomethingAfterStaged}
        isLastStaged={!hasSomethingAfterStaged}
      />

      {props.hasPRFiles && hasLocalChanges && (
        <div data-testid="pr-files-section">
          <PRFilesSection
            files={props.prFiles}
            isLast={!showCommitsList}
            onOpenDiff={props.onOpenDiffFile}
            repoDisplayName={props.repoDisplayName}
          />
        </div>
      )}

      {showCommitsList && (
        <CommitsSection
          commits={mergedCommits}
          isLast={!showCommits}
          defaultCollapsed={firstSection !== "commits"}
          onOpenCommitDetail={props.onOpenCommitDetail}
          onRevertCommit={props.onRevertCommit}
          onAmendCommit={props.dialogs.handleOpenAmendDialog}
          onResetToCommit={props.dialogs.handleOpenResetDialog}
          onRepoPush={props.onRepoPush}
          onRepoCreatePR={props.onRepoCreatePR}
          repoDisplayName={props.repoDisplayName}
          perRepoStatus={props.perRepoStatus}
          prByRepo={props.prByRepo}
        />
      )}
    </div>
  );
}

export function ChangesPanelBody(props: ChangesPanelBodyProps) {
  return (
    <PanelBody className="flex flex-col">
      <div className="flex-1 min-h-0 overflow-y-auto overflow-x-hidden">
        <ChangesPanelTimeline {...props} />
      </div>
      <ReviewProgressBar
        reviewedCount={props.reviewedCount}
        totalFileCount={props.totalFileCount}
        onOpenReview={props.onOpenReview}
      />
      <ChangesPanelDialogsSection dialogs={props.dialogs} isLoading={props.isLoading} />
    </PanelBody>
  );
}
