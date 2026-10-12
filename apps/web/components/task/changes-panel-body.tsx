"use client";

import { useState } from "react";
import { useTranslation } from "react-i18next";
import { IconAlertTriangle } from "@tabler/icons-react";
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
