"use client";

import { useState } from "react";
import { useTranslation } from "react-i18next";
import { IconAlertTriangle, IconLoader2 } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { PanelBody } from "./panel-primitives";
import { DiscardDialog, AmendDialog, ResetDialog } from "./changes-panel-dialogs";
import { ReviewProgressBar } from "./changes-panel-timeline";
import type { ChangesPanelBodyProps } from "./changes-panel-data";
import { ChangesPanelTimelineContent } from "./changes-panel-timeline-content";
import type { ChangesPanelTimelineContentProps } from "./changes-panel-timeline-types";
import { WorkspaceUnavailable } from "./workspace-unavailable";

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
  workspaceBlocked,
}: Pick<ChangesPanelBodyProps, "dialogs" | "isLoading"> & { workspaceBlocked: boolean }) {
  if (workspaceBlocked) return null;
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

function EmptyChangesPanel() {
  const { t } = useTranslation();
  return (
    <div className="flex items-center justify-center h-full text-muted-foreground text-xs">
      {t("task:yourChangedFilesWillAppearHere")}
    </div>
  );
}

function GitStatusNotice(props: ChangesPanelBodyProps) {
  const { t } = useTranslation();
  if (!props.gitStatus.loading && !props.gitStatus.unavailable && !props.gitStatus.detailsPending) {
    return null;
  }
  const hasFailure = props.gitStatus.unavailable;
  return (
    <div
      className="mx-3 mt-2 rounded-md border border-border/60 bg-muted/30 px-2.5 py-2 text-xs"
      data-testid="git-status-notice"
      role={hasFailure ? "alert" : "status"}
      aria-live={hasFailure ? "assertive" : "polite"}
    >
      <div className="flex min-w-0 items-start justify-between gap-2">
        <div className="flex min-w-0 items-start gap-2">
          {hasFailure ? (
            <IconAlertTriangle className="mt-0.5 size-3.5 shrink-0 text-amber-500" />
          ) : (
            <IconLoader2 className="mt-0.5 size-3.5 shrink-0 animate-spin text-muted-foreground" />
          )}
          <div className="min-w-0 space-y-1">
            {props.gitStatus.loading && <p>{t("task:gitStatusChecking")}</p>}
            {hasFailure && (
              <p className="font-medium text-foreground">
                {props.gitStatus.hasPriorData
                  ? t("task:gitStatusRefreshFailedWithData")
                  : t("task:gitStatusUnavailable")}
              </p>
            )}
            {props.gitStatus.failedRepositories.map((repository) => (
              <p key={repository} className="break-words text-muted-foreground">
                {t("task:gitStatusRepositoryUnavailable", { repository })}
              </p>
            ))}
            {props.gitStatus.detailsPending && (
              <p className="text-muted-foreground">{t("task:gitStatusDetailsPending")}</p>
            )}
          </div>
        </div>
        {hasFailure && props.onRetryGitStatus && (
          <Button
            type="button"
            variant="outline"
            className="h-11 min-h-11 shrink-0 px-3 md:h-7 md:min-h-7"
            onClick={props.onRetryGitStatus}
            data-testid="changes-git-status-retry"
          >
            {t("task:gitStatusRetry")}
          </Button>
        )}
      </div>
    </div>
  );
}

function ChangesPanelTimeline(
  props: ChangesPanelTimelineContentProps & Pick<ChangesPanelBodyProps, "gitStatus">,
) {
  if (!props.hasAnything) return props.gitStatus.membershipReady ? <EmptyChangesPanel /> : null;
  return <ChangesPanelTimelineContent {...props} />;
}

export function ChangesPanelBody(props: ChangesPanelBodyProps) {
  const [scrollElement, setScrollElement] = useState<HTMLDivElement | null>(null);
  const workspaceBlocked =
    props.workspaceRestoration && props.workspaceRestoration.status !== "ready";
  const beforeLayoutKey = [
    workspaceBlocked ? (props.workspaceRestoration?.status ?? "blocked") : "ready",
    props.hasPRFiles,
    props.prFiles.length,
    props.hasUnstaged,
    props.hasStaged,
    props.gitStatus.membershipReady,
    props.gitStatus.loading,
    props.gitStatus.unavailable,
    props.gitStatus.detailsPending,
  ].join(":");
  return (
    <PanelBody scroll={false} className="flex flex-col overflow-hidden">
      <ComparisonTargetNotice
        comparisonTargets={props.comparisonTargets}
        comparisonUnavailable={props.comparisonUnavailable}
      />
      <div
        ref={setScrollElement}
        className="flex-1 min-h-0 overflow-y-auto overflow-x-hidden"
        data-testid="changes-panel-scroll-owner"
      >
        <GitStatusNotice {...props} />
        {workspaceBlocked && !props.hasAnything ? (
          <WorkspaceUnavailable
            restoration={props.workspaceRestoration}
            onRetry={props.onRestoreWorkspace}
            retryDisabled={props.restoreWorkspaceDisabled}
          />
        ) : (
          <>
            {workspaceBlocked && (
              <WorkspaceUnavailable
                restoration={props.workspaceRestoration}
                onRetry={props.onRestoreWorkspace}
                retryDisabled={props.restoreWorkspaceDisabled}
                compact
              />
            )}
            <ChangesPanelTimeline
              {...props}
              isLoading={workspaceBlocked ? false : props.isLoading}
              scrollElement={scrollElement}
              beforeLayoutKey={beforeLayoutKey}
            />
          </>
        )}
      </div>
      <ReviewProgressBar
        reviewedCount={props.reviewedCount}
        totalFileCount={props.totalFileCount}
        onOpenReview={props.onOpenReview}
      />
      <ChangesPanelDialogsSection
        dialogs={props.dialogs}
        isLoading={props.isLoading}
        workspaceBlocked={Boolean(workspaceBlocked)}
      />
    </PanelBody>
  );
}
