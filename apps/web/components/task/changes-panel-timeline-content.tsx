"use client";

import { useCallback, useRef, useSyncExternalStore } from "react";
import { useTranslation } from "react-i18next";
import { useAppStore } from "@/components/state-provider";
import { useToast } from "@/components/toast-provider";
import { requestCommitDetail, CommitDetailProtocolError } from "./commit-detail-request";
import {
  ChangesInlineCommitState,
  useChangesInlineCommitState,
} from "./changes-inline-commit-state";
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

type ChangesPanelContextIdentity = {
  contextKey: string;
  activeTaskId: string | null;
  activeSessionId: string | null;
};

export function ChangesPanelTimelineContent(props: ChangesPanelTimelineContentProps) {
  const context = useChangesPanelContextIdentity();
  return (
    <ChangesPanelTimelineContextContent key={context.contextKey} {...props} context={context} />
  );
}

function useChangesPanelContextIdentity(): ChangesPanelContextIdentity {
  const activeTaskId = useAppStore((state) => state.tasks.activeTaskId);
  const activeSessionId = useAppStore((state) => state.tasks.activeSessionId);
  const environmentId = useAppStore((state) =>
    activeSessionId ? (state.environmentIdBySessionId[activeSessionId] ?? null) : null,
  );
  return {
    contextKey: JSON.stringify([activeTaskId, activeSessionId, environmentId]),
    activeTaskId,
    activeSessionId,
  };
}

function ChangesPanelTimelineContextContent({
  context,
  ...props
}: ChangesPanelTimelineContentProps & { context: ChangesPanelContextIdentity }) {
  const layout = useAppStore((state) => state.userSettings.changesPanelLayout);
  const detail = useInlineCommitDetails(context);
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
        contextKey={context.contextKey}
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

function useInlineCommitDetails(context: ChangesPanelContextIdentity) {
  const { t } = useTranslation();
  const { toast } = useToast();
  const { activeTaskId, activeSessionId, contextKey } = context;
  const sessionTaskId = useAppStore((state) =>
    activeSessionId ? state.taskSessions.items[activeSessionId]?.task_id : undefined,
  );
  const agentctlReady = useAppStore((state) =>
    activeSessionId
      ? state.sessionAgentctl.itemsBySessionId[activeSessionId]?.status === "ready"
      : false,
  );
  const requestContextRef = useRef({
    activeSessionId,
    taskId: sessionTaskId ?? activeTaskId,
    agentctlReady,
  });
  requestContextRef.current = {
    activeSessionId,
    taskId: sessionTaskId ?? activeTaskId,
    agentctlReady,
  };
  const errorContextRef = useRef({
    toast,
    unexpectedError: t("unexpectedResponseFromTheServer", { ns: "github" }),
    requestFailed: t("requestFailed"),
  });
  errorContextRef.current = {
    toast,
    unexpectedError: t("unexpectedResponseFromTheServer", { ns: "github" }),
    requestFailed: t("requestFailed"),
  };
  const createDetailState = useCallback(
    (key: string) =>
      new ChangesInlineCommitState({
        contextKey: key,
        request: async (target) => {
          const requestContext = requestContextRef.current;
          const result = await requestCommitDetail({
            target,
            ...(target.source === "local" && requestContext.activeSessionId
              ? {
                  local: {
                    sessionId: requestContext.activeSessionId,
                    taskId: requestContext.taskId ?? null,
                    agentctlReady: requestContext.agentctlReady,
                  },
                }
              : {}),
          });
          if (!result.success) {
            throw new CommitDetailProtocolError("invalid_response");
          }
          return result.files ?? {};
        },
        onError: (_target, error) => {
          const errorContext = errorContextRef.current;
          errorContext.toast({
            title: errorContext.requestFailed,
            description: inlineCommitErrorMessage(error, errorContext.unexpectedError),
            variant: "error",
          });
        },
      }),
    [errorContextRef, requestContextRef],
  );
  const state = useChangesInlineCommitState(contextKey, createDetailState);
  const version = useSyncExternalStore(state.subscribe, state.getVersion, state.getVersion);
  return { state, version };
}

function inlineCommitErrorMessage(error: unknown, unexpectedError: string): string {
  if (error instanceof CommitDetailProtocolError) return unexpectedError;
  if (error instanceof Error) return error.message;
  return unexpectedError;
}
