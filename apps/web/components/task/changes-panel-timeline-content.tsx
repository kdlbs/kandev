"use client";

import { useAppStore } from "@/components/state-provider";
import { ChangesWorkingTree } from "./changes-timeline-working-tree";
import {
  useCommitSections,
  useHistoryExpansion,
  useHistoryRowRenderer,
  useHistoryRows,
  useHistorySections,
} from "./changes-panel-timeline-history";
import type {
  ChangesPanelTimelineContentProps,
  WorkingTreeProps,
} from "./changes-panel-timeline-types";
import type { ChangesHistoryTimelineRow } from "./changes-timeline-model";
import type { ChangesTimelineFocusRequest } from "./changes-timeline-viewport";

export function ChangesPanelTimelineContent(props: ChangesPanelTimelineContentProps) {
  return <ChangesPanelTimelineContextContent key={props.contextKey} {...props} />;
}

function ChangesPanelTimelineContextContent(props: ChangesPanelTimelineContentProps) {
  const layout = useAppStore((state) => state.userSettings.changesPanelLayout);
  const detail = {
    state: props.inlineCommitDetails,
    version: props.inlineCommitDetailVersion,
  };
  const commitSections = useCommitSections(props);
  const expansion = useHistoryExpansion(props, commitSections);
  const historySections = useHistorySections({
    props,
    commitSections,
    expansion,
    detailState: detail.state,
    detailStateVersion: detail.version,
    layout,
  });
  const rows = useHistoryRows(historySections, props.hasUnstaged || props.hasStaged);
  const renderHistoryRow = useHistoryRowRenderer(props, commitSections, detail.state, expansion);

  return (
    <div className="flex flex-col">
      <WorkingTreeSections
        props={props}
        contextKey={props.contextKey}
        focusRequest={expansion.focusRequest}
        historyRowsBefore={rows.before}
        historyRowsAfter={rows.after}
        renderHistoryRow={renderHistoryRow}
      />
    </div>
  );
}

function WorkingTreeSections({
  props,
  contextKey,
  focusRequest,
  historyRowsBefore,
  historyRowsAfter,
  renderHistoryRow,
}: {
  props: WorkingTreeProps &
    Pick<ChangesPanelTimelineContentProps, "scrollElement" | "beforeLayoutKey">;
  contextKey: string;
  focusRequest?: ChangesTimelineFocusRequest;
  historyRowsBefore: ChangesHistoryTimelineRow[];
  historyRowsAfter: ChangesHistoryTimelineRow[];
  renderHistoryRow: (row: ChangesHistoryTimelineRow) => React.ReactNode;
}) {
  return (
    <ChangesWorkingTree
      hasUnstaged={props.hasUnstaged}
      hasStaged={props.hasStaged}
      unstagedFiles={props.unstagedFiles}
      stagedFiles={props.stagedFiles}
      pendingStageFiles={props.pendingStageFiles}
      onOpenDiff={props.onOpenDiffFile}
      onEditFile={props.onEditFile}
      onStage={props.onStage}
      onUnstage={props.onUnstage}
      onDiscard={props.dialogs.handleDiscardClick}
      onBulkStage={props.onBulkStage}
      onBulkUnstage={props.onBulkUnstage}
      onBulkDiscard={props.onBulkDiscard}
      onRepoStageAll={props.onRepoStageAll}
      onRepoUnstageAll={props.onRepoUnstageAll}
      onRepoCommit={props.onRepoCommit}
      repoDisplayName={props.repoDisplayName}
      isLoading={props.isLoading}
      scrollElement={props.scrollElement}
      beforeLayoutKey={props.beforeLayoutKey}
      contextKey={contextKey}
      focusRequest={focusRequest}
      historyRowsBefore={historyRowsBefore}
      historyRowsAfter={historyRowsAfter}
      renderHistoryRow={renderHistoryRow}
    />
  );
}
