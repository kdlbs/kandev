"use client";

import { useCallback, useEffect, useState } from "react";
import { PortForwardingManager } from "./port-forward-dialog";
import { TaskTopBar } from "@/components/task/task-top-bar";
import { TaskLayout } from "@/components/task/task-layout";
import { DebugOverlay } from "@/components/debug-overlay";
import { type Repository, type RepositoryScript, type Task } from "@/lib/types/http";
import type { Terminal } from "@/hooks/domains/session/use-terminals";
import { isDebugUI } from "@/lib/config";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { useAppStore } from "@/components/state-provider";
import type { UseEnsureTaskSessionResult } from "@/hooks/domains/session/use-ensure-task-session";
import {
  getSessionRecoveryRetry,
  SessionRecoveryFeedback,
} from "@/components/task/ensure-session-error";
import type { Layout } from "react-resizable-panels";
import { TaskArchivedProvider } from "./task-archived-context";
import { TaskCommands } from "@/components/task-commands";
import { SessionCommands } from "@/components/session-commands";
import { TaskPRShortcut } from "@/components/task/task-pr-shortcut";
import { useEmbeddedVscodeSupport } from "@/components/task/task-page-editor-capability";
import { VcsDialogsProvider } from "@/components/vcs/vcs-dialogs";
import { PortForwardingVisibilityProvider } from "@/components/task/port-forwarding-visibility-provider";
import { TaskLaunchErrorProvider } from "@/components/task/task-launch-error-context";
import { SessionBootstrapRecoveryCard } from "@/components/task/chat/session-bootstrap-recovery-card";
import { TaskSharedError } from "@/components/task/task-shared-error";
import {
  buildArchivedValue,
  resolveTaskProps,
  resolveTaskPageBootstrapRecoveryError,
  useTaskActionsMenuBoardRow,
  selectWorkspaceRepositories,
  shouldReservePageLevelMobileFeedbackOffset,
} from "@/components/task/task-page-content-helpers";
import {
  buildTaskLayoutProps,
  buildTaskTopBarProps,
  maybeBuildDebugEntries,
  resolveRemoteExecutor,
  type RemoteExecutorStatus,
} from "./task-page-inner-helpers";
import type { useSessionResumption } from "@/hooks/domains/session/use-session-resumption";
import type { useSessionAgentctl } from "@/hooks/domains/session/use-session-agentctl";
import type {
  useWorkflowStepsMapped,
  useSessionPanelState,
  useMergedAgentState,
} from "./task-page-content";
import { useTranslation } from "react-i18next";
import type { Canvas } from "@/lib/api/domains/canvas-api";
import type { TaskCanvasesLoadStatus } from "@/hooks/domains/task/use-task-canvases";
import { useTaskStatusSummary } from "@/hooks/domains/task/use-task-status-summary";
import {
  TaskNavigationReadFeedback,
  type TaskNavigationReadRecovery,
} from "@/components/task/task-navigation-read-feedback";
import { TaskPageEntryFeedback } from "@/components/task/task-page-entry-feedback";

import { useAutomaticRecoveryChatOwner } from "@/hooks/domains/session/use-automatic-recovery-chat-owner";

const PAGE_LEVEL_MOBILE_FEEDBACK_STYLE = {
  paddingTop: "calc(3.5rem + 1px + env(safe-area-inset-top, 0px))",
} as const;

export type TaskPageInnerProps = {
  task: Task | null;
  effectiveSessionId: string | null;
  repository: Repository | null;
  merged: ReturnType<typeof useMergedAgentState>;
  resumption: ReturnType<typeof useSessionResumption>;
  sessionPanel: ReturnType<typeof useSessionPanelState>;
  agentctlStatus: ReturnType<typeof useSessionAgentctl>;
  connectionStatus: string;
  workflowSteps: ReturnType<typeof useWorkflowStepsMapped>;
  archivedValue: ReturnType<typeof buildArchivedValue>;
  isMobile: boolean;
  showDebugOverlay: boolean;
  onToggleDebugOverlay: () => void;
  initialScripts: RepositoryScript[];
  initialTerminals?: Terminal[];
  defaultLayouts: Record<string, Layout>;
  initialLayout?: string | null;
  officeTaskHref?: string | null;
  ensureSession: UseEnsureTaskSessionResult;
  onTaskUnarchived: (taskId: string) => void;
  taskCanvases?: Canvas[];
  taskCanvasesStatus?: TaskCanvasesLoadStatus;
  taskReadRecovery?: TaskNavigationReadRecovery;
};

function TaskDebugOverlay({ entries }: { entries: ReturnType<typeof maybeBuildDebugEntries> }) {
  const { t } = useTranslation();
  if (!entries) return null;
  return <DebugOverlay title={t("task:taskDebug")} entries={entries} />;
}

function TaskPageRecoveryFeedback({
  ownedByChat,
  taskId,
  sessionId,
  resumption,
  bootstrapRecoveryError,
  workspaceId,
  isPassthrough,
}: {
  taskId: string;
  sessionId: string | null;
  resumption: TaskPageInnerProps["resumption"];
  ownedByChat: boolean;
  bootstrapRecoveryError: ReturnType<typeof resolveTaskPageBootstrapRecoveryError>;
  workspaceId: string | null;
  isPassthrough: boolean;
}) {
  if (bootstrapRecoveryError && sessionId && isPassthrough) {
    return (
      <SessionBootstrapRecoveryCard
        taskId={taskId}
        sessionId={sessionId}
        workspaceId={workspaceId}
        error={bootstrapRecoveryError}
        automaticRecovery={resumption}
      />
    );
  }
  if (bootstrapRecoveryError) {
    return null;
  }
  return (
    <SessionRecoveryFeedback
      ownedByChat={ownedByChat}
      error={resumption.error}
      notice={resumption.notice}
      recoveryFailure={resumption.recoveryFailure}
      onRetry={getSessionRecoveryRetry(resumption)}
      retryDisabled={
        resumption.resumptionState === "checking" || resumption.resumptionState === "resuming"
      }
      workspaceId={workspaceId}
    />
  );
}

function TaskPageLayoutFeedback({
  layoutProps,
  isMobile,
  hasPageLevelMobileFeedback,
}: {
  layoutProps: ReturnType<typeof buildTaskLayoutProps>;
  isMobile: boolean;
  hasPageLevelMobileFeedback: boolean;
}) {
  return (
    <>
      <TaskSharedError reserveMobileTopBar={isMobile && !hasPageLevelMobileFeedback} />
      <div className="flex min-h-0 flex-1 flex-col">
        <TaskLayout {...layoutProps} hasPageLevelFeedback={hasPageLevelMobileFeedback} />
      </div>
    </>
  );
}

function TaskPageCommandSurfaces({
  taskProps,
  debugEntries,
  merged,
  sessionId,
  isPassthrough,
  isArchived,
}: {
  taskProps: ReturnType<typeof resolveTaskProps>;
  debugEntries: ReturnType<typeof maybeBuildDebugEntries>;
  merged: TaskPageInnerProps["merged"];
  sessionId: string | null;
  isPassthrough: boolean;
  isArchived: boolean;
}) {
  return (
    <>
      <SessionCommands
        sessionId={sessionId}
        baseBranch={taskProps.baseBranch}
        isAgentRunning={merged.isAgentWorking}
        hasWorktree={Boolean(merged.worktreeBranch)}
        isPassthrough={isPassthrough}
        isTaskArchived={isArchived}
      />
      <TaskPRShortcut taskId={taskProps.taskId} />
      <TaskDebugOverlay entries={debugEntries} />
    </>
  );
}

function TaskPageDesktopTopBar({
  isMobile,
  topBarProps,
  onMoveStart,
  onMoveError,
}: {
  isMobile: boolean;
  topBarProps: ReturnType<typeof buildTaskTopBarProps>;
  onMoveStart: () => void;
  onMoveError: (error: unknown) => void;
}) {
  if (isMobile) return null;
  return <TaskTopBar {...topBarProps} onMoveStart={onMoveStart} onMoveError={onMoveError} />;
}

function useTaskPageRecoveryFeedback(
  {
    task,
    effectiveSessionId,
    resumption,
    sessionPanel,
    isMobile,
    ensureSession,
    taskReadRecovery,
  }: Pick<
    TaskPageInnerProps,
    | "task"
    | "effectiveSessionId"
    | "resumption"
    | "sessionPanel"
    | "isMobile"
    | "ensureSession"
    | "taskReadRecovery"
  >,
  taskMoveError: unknown,
) {
  const activeSessionMetadata = useAppStore((state) =>
    effectiveSessionId ? (state.taskSessions.items[effectiveSessionId]?.metadata ?? null) : null,
  );
  const statusSummary = useTaskStatusSummary(task?.id, task?.status_summary);
  const automaticRecoveryOwnedByChat = useAutomaticRecoveryChatOwner({
    taskId: task?.id,
    sessionId: effectiveSessionId,
    recovery: resumption,
    summary: statusSummary,
    disabled: Boolean(task?.archived_at) || sessionPanel.isSessionPassthrough,
  });
  const bootstrapRecoveryError = resolveTaskPageBootstrapRecoveryError(
    statusSummary,
    effectiveSessionId,
    activeSessionMetadata,
  );
  const hasPageLevelMobileFeedback = shouldReservePageLevelMobileFeedbackOffset({
    isMobile,
    hasTaskMoveError: taskMoveError !== null,
    hasEnsureSessionError: ensureSession.status === "error",
    hasBootstrapRecoveryError: bootstrapRecoveryError !== null,
    hasTaskReadRecovery: Boolean(taskReadRecovery?.temporaryError),
    effectiveSessionId,
    isSessionPassthrough: sessionPanel.isSessionPassthrough,
    hasComposerRecoveryOwner: automaticRecoveryOwnedByChat,
    hasResumptionError: Boolean(resumption.error),
    hasResumptionNotice: Boolean(resumption.notice),
    hasStatusUnavailable: resumption.recoveryFailure?.outcome === "status_unavailable",
  });
  return {
    activeSessionMetadata,
    bootstrapRecoveryError,
    automaticRecoveryOwnedByChat,
    hasPageLevelMobileFeedback,
  };
}

/**
 * Derives everything the task page renders from its inputs: the resolved task
 * props plus the three prop bundles handed to the debug overlay, top bar, and
 * layout. Kept out of `TaskPageInner` so that component stays a wiring shell.
 */
function useTaskPageDerivedProps(
  {
    task,
    effectiveSessionId,
    ensureSession,
    isMobile,
    repository,
    merged,
    resumption,
    sessionPanel,
    agentctlStatus,
    connectionStatus,
    workflowSteps,
    showDebugOverlay,
    onToggleDebugOverlay,
    initialScripts,
    initialTerminals,
    defaultLayouts,
    initialLayout,
    officeTaskHref,
    onTaskUnarchived,
    taskCanvases,
    taskCanvasesStatus,
    taskReadRecovery,
  }: TaskPageInnerProps,
  taskMoveError: unknown,
) {
  const workspaceRepositories = useAppStore((state) =>
    selectWorkspaceRepositories(state.repositories.itemsByWorkspaceId, task?.workspace_id),
  );
  const taskProps = resolveTaskProps(task, repository, workspaceRepositories);
  const actionsMenuBoardRow = useTaskActionsMenuBoardRow(task);
  const remote = resolveRemoteExecutor(resumption.sessionStatus as RemoteExecutorStatus | null);
  const embeddedVscode = useEmbeddedVscodeSupport(effectiveSessionId, resumption.sessionStatus);
  const {
    activeSessionMetadata,
    bootstrapRecoveryError,
    automaticRecoveryOwnedByChat,
    hasPageLevelMobileFeedback,
  } = useTaskPageRecoveryFeedback(
    {
      task,
      effectiveSessionId,
      resumption,
      sessionPanel,
      isMobile,
      ensureSession,
      taskReadRecovery,
    },
    taskMoveError,
  );
  const debugEntries = maybeBuildDebugEntries({
    isVisible: isDebugUI() && showDebugOverlay,
    connectionStatus,
    task,
    effectiveSessionId,
    activeSessionMetadata,
    merged,
    resumption,
    sessionPanel,
    agentctlStatus,
  });
  const topBarProps = buildTaskTopBarProps({
    task,
    taskProps,
    actionsMenuBoardRow,
    workflowSteps,
    showDebugOverlay,
    onToggleDebugOverlay,
    effectiveSessionId,
    remote,
    sessionWorkflowStepId: sessionPanel.sessionWorkflowStepId,
    embeddedVscodeSupported: embeddedVscode,
    officeTaskHref,
    onTaskUnarchived,
  });
  const layoutProps = buildTaskLayoutProps({
    taskProps,
    repository,
    effectiveSessionId,
    initialScripts,
    initialTerminals,
    defaultLayouts,
    merged,
    remote,
    initialLayout,
    onTaskUnarchived,
    taskCanvases,
    taskCanvasesStatus,
  });

  return {
    taskProps,
    debugEntries,
    topBarProps,
    layoutProps,
    bootstrapRecoveryError,
    automaticRecoveryOwnedByChat,
    hasPageLevelMobileFeedback,
  };
}

export function TaskPageInner(props: TaskPageInnerProps) {
  const { effectiveSessionId, task, merged, sessionPanel, archivedValue, isMobile, ensureSession } =
    props;
  const [taskMoveError, setTaskMoveError] = useState<unknown>(null);
  const clearTaskMoveError = useCallback(() => setTaskMoveError(null), []);
  const reportTaskMoveError = useCallback((error: unknown) => setTaskMoveError(error), []);
  useEffect(() => {
    setTaskMoveError(null);
  }, [task?.id]);
  const {
    taskProps,
    debugEntries,
    topBarProps,
    layoutProps,
    bootstrapRecoveryError,
    automaticRecoveryOwnedByChat,
    hasPageLevelMobileFeedback,
  } = useTaskPageDerivedProps(props, taskMoveError);
  if (!task) return null;

  return (
    <TooltipProvider>
      <PortForwardingVisibilityProvider
        taskId={taskProps.taskId}
        metadata={task?.metadata}
        sessionId={effectiveSessionId}
        isAgentctlReady={props.agentctlStatus.isReady}
        isArchived={taskProps.isArchived}
      >
        <PortForwardingManager sessionId={effectiveSessionId}>
          <VcsDialogsProvider
            sessionId={effectiveSessionId}
            taskId={taskProps.taskId}
            baseBranch={taskProps.baseBranch}
            pullRequestBaseBranch={taskProps.pullRequestTarget}
            pullRequestTargetsByRepository={taskProps.pullRequestTargetsByRepository}
            taskTitle={taskProps.taskTitle}
            displayBranch={merged.worktreeBranch}
          >
            <div
              className="relative flex h-full min-h-0 w-full flex-col overflow-hidden bg-background"
              style={hasPageLevelMobileFeedback ? PAGE_LEVEL_MOBILE_FEEDBACK_STYLE : undefined}
            >
              <TaskPageCommandSurfaces
                taskProps={taskProps}
                debugEntries={debugEntries}
                merged={merged}
                sessionId={effectiveSessionId}
                isPassthrough={sessionPanel.isSessionPassthrough}
                isArchived={archivedValue.isArchived}
              />
              <TaskPageDesktopTopBar
                isMobile={isMobile}
                topBarProps={topBarProps}
                onMoveStart={clearTaskMoveError}
                onMoveError={reportTaskMoveError}
              />
              <TaskPageEntryFeedback
                taskMoveError={taskMoveError}
                ensureSession={ensureSession}
                workspaceId={task.workspace_id ?? null}
              />
              <TaskNavigationReadFeedback recovery={props.taskReadRecovery} />
              <TaskArchivedProvider value={archivedValue}>
                <TaskCommands task={task} />
                <TaskLaunchErrorProvider
                  value={{
                    taskId: task.id,
                    workspaceId: task.workspace_id,
                    statusSummary: task.status_summary,
                    repositories: task.repositories,
                    automaticRecovery: props.resumption,
                    automaticRecoveryOwnerSessionId: automaticRecoveryOwnedByChat
                      ? effectiveSessionId
                      : null,
                  }}
                >
                  <TaskPageRecoveryFeedback
                    ownedByChat={automaticRecoveryOwnedByChat}
                    taskId={task.id}
                    sessionId={effectiveSessionId}
                    resumption={props.resumption}
                    bootstrapRecoveryError={bootstrapRecoveryError}
                    workspaceId={task?.workspace_id ?? null}
                    isPassthrough={sessionPanel.isSessionPassthrough}
                  />
                  <TaskPageLayoutFeedback
                    layoutProps={layoutProps}
                    isMobile={isMobile}
                    hasPageLevelMobileFeedback={hasPageLevelMobileFeedback}
                  />
                </TaskLaunchErrorProvider>
              </TaskArchivedProvider>
            </div>
          </VcsDialogsProvider>
        </PortForwardingManager>
      </PortForwardingVisibilityProvider>
    </TooltipProvider>
  );
}
