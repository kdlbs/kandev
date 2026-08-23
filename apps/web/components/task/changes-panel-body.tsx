"use client";

import { PanelBody } from "./panel-primitives";
import { DiscardDialog, AmendDialog, ResetDialog } from "./changes-panel-dialogs";
import {
  FileListSection,
  CommitsSection,
  ReviewProgressBar,
  PRFilesSection,
} from "./changes-panel-timeline";
import {
  firstVisibleSection,
  mergeCommits,
  separateCommitHistories,
} from "./changes-panel-helpers";
import type { ChangesPanelBodyProps } from "./changes-panel-data";
import { useTranslation } from "react-i18next";
import { IconAlertTriangle } from "@tabler/icons-react";

function ComparisonTargetNotice({
  comparisonTargets,
  comparisonUnavailable,
}: Pick<ChangesPanelBodyProps, "comparisonTargets" | "comparisonUnavailable">) {
  const { t } = useTranslation();
  if (!comparisonUnavailable) return null;

  const targetLabel = comparisonTargets.join(", ") || t("task:comparisonTargetUnknown");
  return (
    <div
      className="mx-3 mt-2 rounded-md border border-amber-500/30 bg-amber-500/10 px-2.5 py-2 text-xs"
      data-testid="comparison-target-notice"
      role="alert"
    >
      <div className="flex min-w-0 items-start gap-2">
        <IconAlertTriangle className="mt-0.5 h-3.5 w-3.5 shrink-0 text-amber-500" />
        <div className="min-w-0">
          <p className="font-medium text-foreground">{t("task:comparisonTargetUnavailable")}</p>
          <p className="break-words text-muted-foreground">
            {t("task:comparisonTargetUnavailableDescription", { target: targetLabel })}
          </p>
        </div>
      </div>
    </div>
  );
}

function ChangesPanelDialogsSection({
  dialogs,
  isLoading,
}: Pick<ChangesPanelBodyProps, "dialogs" | "isLoading">) {
  return (
    <>
      <DiscardDialog
        open={dialogs.showDiscardDialog}
        onOpenChange={dialogs.handleDiscardOpenChange}
        fileToDiscard={dialogs.fileToDiscard}
        filesToDiscard={dialogs.filesToDiscard}
        anchorRef={dialogs.discardAnchorRef}
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
  | "relation"
  | "resolution"
  | "resolutionTarget"
  | "providerPRNumber"
  | "pushDisabled"
  | "pullDisabled"
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
  | "isLoading"
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
>;

function WorkingTreeSections(props: WorkingTreeProps) {
  const { t } = useTranslation();
  const isBulkOp = props.pendingStageFiles.size === 0;
  return (
    <>
      {props.hasUnstaged && (
        <FileListSection
          variant="unstaged"
          files={props.unstagedFiles}
          pendingStageFiles={props.pendingStageFiles}
          actionLabel={t("task:stageAll")}
          isActionLoading={props.isLoading || (isBulkOp && props.loadingOperation === "stage")}
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
          actionLabel={t("task:commit")}
          isActionLoading={props.isLoading || props.loadingOperation === "commit"}
          onAction={() => props.dialogs.openCommitDialog()}
          secondaryActionLabel={t("task:unstageAll")}
          isSecondaryActionLoading={
            props.isLoading || (isBulkOp && props.loadingOperation === "unstage")
          }
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

function CommitHistorySections({
  props,
  isDiverged,
  defaultCollapsed,
  mergedCommits,
  separated,
}: {
  props: TimelineProps;
  isDiverged: boolean;
  defaultCollapsed: boolean;
  mergedCommits: ReturnType<typeof mergeCommits>;
  separated: ReturnType<typeof separateCommitHistories>;
}) {
  const { t } = useTranslation();
  if (isDiverged) {
    return (
      <>
        {separated.localCommits.length > 0 && (
          <CommitsSection
            commits={separated.localCommits}
            label={t("task:localCheckoutCommits")}
            testId="local-checkout-commits-section"
            defaultCollapsed={defaultCollapsed}
            pushDisabled={props.pushDisabled}
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
        {separated.providerCommits.length > 0 && (
          <CommitsSection
            commits={separated.providerCommits}
            label={t("task:prNumberVersion", { number: props.providerPRNumber ?? "" })}
            testId="current-pr-commits-section"
            defaultCollapsed
            showActions={false}
            onOpenCommitDetail={props.onOpenCommitDetail}
            repoDisplayName={props.repoDisplayName}
            perRepoStatus={props.perRepoStatus}
          />
        )}
      </>
    );
  }

  return (
    <CommitsSection
      commits={mergedCommits}
      defaultCollapsed={defaultCollapsed}
      onOpenCommitDetail={props.onOpenCommitDetail}
      onRevertCommit={props.onRevertCommit}
      onAmendCommit={props.dialogs.handleOpenAmendDialog}
      onResetToCommit={props.dialogs.handleOpenResetDialog}
      onRepoPush={props.onRepoPush}
      onRepoCreatePR={props.onRepoCreatePR}
      repoDisplayName={props.repoDisplayName}
      perRepoStatus={props.perRepoStatus}
      prByRepo={props.prByRepo}
      pushDisabled={props.pushDisabled}
    />
  );
}

function EmptyChangesPanel() {
  const { t } = useTranslation();
  return (
    <div className="flex items-center justify-center h-full text-muted-foreground text-xs">
      {t("task:yourChangedFilesWillAppearHere")}
    </div>
  );
}

function ChangesPanelTimeline(props: TimelineProps) {
  if (!props.hasAnything) {
    return <EmptyChangesPanel />;
  }

  const isDiverged = props.relation.presentation === "separate";
  const separated = isDiverged
    ? separateCommitHistories(props.commits, props.prCommits)
    : { providerCommits: [], localCommits: [] };
  const mergedCommits = isDiverged ? [] : mergeCommits(props.commits, props.prCommits);
  const hasMergedCommits = isDiverged
    ? separated.providerCommits.length > 0 || separated.localCommits.length > 0
    : mergedCommits.length > 0;
  const hasLocalChanges = props.hasUnstaged || props.hasStaged;
  const showCommitsList = props.hasStaged || hasMergedCommits;
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
            onOpenDiff={props.onOpenDiffFile}
            repoDisplayName={props.repoDisplayName}
            defaultCollapsed={firstSection !== "pr"}
          />
        </div>
      )}

      <WorkingTreeSections {...props} />

      {props.hasPRFiles && hasLocalChanges && (
        <div data-testid="pr-files-section">
          <PRFilesSection
            files={props.prFiles}
            onOpenDiff={props.onOpenDiffFile}
            repoDisplayName={props.repoDisplayName}
          />
        </div>
      )}

      {showCommitsList && (
        <CommitHistorySections
          props={props}
          isDiverged={isDiverged}
          defaultCollapsed={firstSection !== "commits"}
          mergedCommits={mergedCommits}
          separated={separated}
        />
      )}
    </div>
  );
}

export function ChangesPanelBody(props: ChangesPanelBodyProps) {
  return (
    <PanelBody className="flex flex-col">
      <ComparisonTargetNotice
        comparisonTargets={props.comparisonTargets}
        comparisonUnavailable={props.comparisonUnavailable}
      />
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
