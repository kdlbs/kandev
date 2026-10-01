"use client";

import { type ReactNode, useCallback, useEffect, useState } from "react";
import { TaskTopBar } from "@/components/task/task-top-bar";
import { TaskLayout } from "@/components/task/task-layout";
import { DebugOverlay } from "@/components/debug-overlay";
import { type Repository, type RepositoryScript, type Task } from "@/lib/types/http";
import type { Terminal } from "@/hooks/domains/session/use-terminals";
import { isDebugUI } from "@/lib/config";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { useAppStore } from "@/components/state-provider";
import type { UseEnsureTaskSessionResult } from "@/hooks/domains/session/use-ensure-task-session";
import type { Layout } from "react-resizable-panels";
import { TaskArchivedProvider } from "./task-archived-context";
import { TaskCommands } from "@/components/task-commands";
import { SessionCommands } from "@/components/session-commands";
import { TaskPRShortcut } from "@/components/task/task-pr-shortcut";
import { useEmbeddedVscodeSupport } from "@/components/task/task-page-editor-capability";
import { VcsDialogsProvider } from "@/components/vcs/vcs-dialogs";
import { PortForwardingVisibilityProvider } from "@/components/task/port-forwarding-visibility-provider";
import { TaskLaunchErrorProvider } from "@/components/task/task-launch-error-context";
import { TaskSharedError } from "@/components/task/task-shared-error";
import {
  buildDebugEntries,
  buildArchivedValue,
  resolveTaskProps,
  resolveWorkflowCurrentStepId,
  resolveTaskPageBootstrapRecoveryError,
  useTaskActionsMenuBoardRow,
  selectWorkspaceRepositories,
  shouldReservePageLevelMobileFeedbackOffset,
} from "@/components/task/task-page-content-helpers";
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
import { AgentProjectTaskProvider } from "./agent-project-task-context";
import { TaskPageEntryFeedback, TaskPageRecoveryFeedback } from "./task-page-feedback";
import { useTaskStatusSummary } from "@/hooks/domains/task/use-task-status-summary";

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
};

type RemoteExecutorStatus = {
  is_remote_executor?: boolean;
  executor_type?: string | null;
  executor_name?: string | null;
  remote_name?: string | null;
  remote_state?: string | null;
  remote_created_at?: string | null;
  remote_checked_at?: string | null;
  remote_status_error?: string | null;
  capabilities?: {
    embedded_vscode?: boolean;
  };
};

function toNullable(value: string | null | undefined): string | null {
  return value ?? null;
}

function resolveRemoteExecutor(status?: RemoteExecutorStatus | null) {
  const remoteExecutorName = status?.remote_name ?? status?.executor_name ?? null;
  return {
    isRemoteExecutor: status?.is_remote_executor ?? false,
    remoteExecutorType: toNullable(status?.executor_type),
    remoteExecutorName,
    remoteState: toNullable(status?.remote_state),
    remoteCreatedAt: toNullable(status?.remote_created_at),
    remoteCheckedAt: toNullable(status?.remote_checked_at),
    remoteStatusError: toNullable(status?.remote_status_error),
  };
}

function resolveCurrentStepId(
  sessionStepId: string | null,
  taskStepId: string | null,
  workflowStepIds: readonly string[],
): string | null {
  return resolveWorkflowCurrentStepId(sessionStepId, taskStepId, workflowStepIds);
}

function buildTaskTopBarProps(params: {
  task: Task | null;
  taskProps: ReturnType<typeof resolveTaskProps>;
  actionsMenuBoardRow: ReturnType<typeof useTaskActionsMenuBoardRow>;
  workflowSteps: ReturnType<typeof useWorkflowStepsMapped>;
  showDebugOverlay: boolean;
  onToggleDebugOverlay: () => void;
  effectiveSessionId: string | null;
  remote: ReturnType<typeof resolveRemoteExecutor>;
  sessionWorkflowStepId: string | null;
  embeddedVscodeSupported: boolean;
  officeTaskHref?: string | null;
  onTaskUnarchived: (taskId: string) => void;
}) {
  const { taskProps, workflowSteps, showDebugOverlay, onToggleDebugOverlay } = params;
  return {
    taskId: taskProps.taskId,
    activeSessionId: params.effectiveSessionId,
    taskTitle: taskProps.taskTitle,
    repositoryLabel: taskProps.repositoryLabel,
    topbarRepository: taskProps.topbarRepository,
    showDebugOverlay,
    onToggleDebugOverlay,
    workflowSteps,
    currentStepId: resolveCurrentStepId(
      params.sessionWorkflowStepId,
      taskProps.workflowStepId,
      workflowSteps.map((step) => step.id),
    ),
    workflowId: taskProps.workflowId,
    taskState: params.task?.state ?? null,
    isAgentProjectTask: Boolean(params.task?.agent_project_id),
    isAgentProjectWorker: Boolean(
      params.task?.agent_project_id && params.task.agent_project_tier !== "coordinator",
    ),
    workspaceId: taskProps.workspaceId,
    projectId: taskProps.projectId,
    issueUrl: taskProps.issueUrl,
    issueNumber: taskProps.issueNumber,
    isArchived: taskProps.isArchived,
    embeddedVscodeSupported: params.embeddedVscodeSupported,
    remoteExecutorType: params.remote.remoteExecutorType,
    officeTaskHref: params.officeTaskHref,
    onTaskUnarchived: params.onTaskUnarchived,
    actionsMenuBoardRow: params.actionsMenuBoardRow,
    // The subject's own last-known values, independent of `actionsMenuBoardRow`:
    // the board excludes archived (and can lag/miss cross-workflow) tasks, so
    // these stay available for the actions menu's plugin context and
    // executor-aware confirmation copy even when the board row is unresolvable.
    subjectWorkflowStepId: taskProps.workflowStepId,
    subjectPrimaryExecutorType: taskProps.primaryExecutorType,
  };
}

function buildTaskLayoutProps(params: {
  taskProps: ReturnType<typeof resolveTaskProps>;
  repository: Repository | null;
  effectiveSessionId: string | null;
  initialScripts: RepositoryScript[];
  initialTerminals?: Terminal[];
  defaultLayouts: Record<string, Layout>;
  merged: ReturnType<typeof useMergedAgentState>;
  remote: ReturnType<typeof resolveRemoteExecutor>;
  initialLayout?: string | null;
  onTaskUnarchived: (taskId: string) => void;
  taskCanvases?: Canvas[];
  taskCanvasesStatus?: TaskCanvasesLoadStatus;
}) {
  const { taskProps, repository, effectiveSessionId, initialScripts, initialTerminals } = params;
  return {
    taskId: taskProps.taskId,
    workspaceId: taskProps.workspaceId,
    workflowId: taskProps.workflowId,
    sessionId: effectiveSessionId,
    repository: repository ?? null,
    initialScripts,
    initialTerminals,
    defaultLayouts: params.defaultLayouts,
    initialLayout: params.initialLayout,
    taskCanvases: params.taskCanvases,
    taskCanvasesStatus: params.taskCanvasesStatus,
    taskTitle: taskProps.taskTitle,
    repositoryLabel: taskProps.repositoryLabel,
    topbarRepository: taskProps.topbarRepository,
    baseBranch: taskProps.baseBranch,
    worktreeBranch: params.merged.worktreeBranch,
    isRemoteExecutor: params.remote.isRemoteExecutor,
    remoteExecutorType: params.remote.remoteExecutorType,
    remoteExecutorName: params.remote.remoteExecutorName,
    remoteState: params.remote.remoteState,
    remoteCreatedAt: params.remote.remoteCreatedAt,
    remoteCheckedAt: params.remote.remoteCheckedAt,
    remoteStatusError: params.remote.remoteStatusError,
    isArchived: taskProps.isArchived,
    onTaskUnarchived: params.onTaskUnarchived,
  };
}

function maybeBuildDebugEntries(params: {
  isVisible: boolean;
  connectionStatus: string;
  task: Task | null;
  effectiveSessionId: string | null | undefined;
  activeSessionMetadata?: Record<string, unknown> | null;
  merged: ReturnType<typeof useMergedAgentState>;
  resumption: ReturnType<typeof useSessionResumption>;
  sessionPanel: ReturnType<typeof useSessionPanelState>;
  agentctlStatus: ReturnType<typeof useSessionAgentctl>;
}) {
  if (!params.isVisible) return null;
  return buildDebugEntries({
    connectionStatus: params.connectionStatus,
    task: params.task,
    effectiveSessionId: params.effectiveSessionId,
    activeSessionMetadata: params.activeSessionMetadata,
    taskSessionState: params.merged.taskSessionState,
    isAgentWorking: params.merged.isAgentWorking,
    resumptionState: params.resumption.resumptionState,
    resumptionError: params.resumption.error,
    agentctlStatus: params.agentctlStatus,
    previewOpen: params.sessionPanel.previewOpen,
    previewStage: params.sessionPanel.previewStage,
    previewUrl: params.sessionPanel.previewUrl,
    devProcessId: params.sessionPanel.devProcessId,
    devProcessStatus: params.sessionPanel.devProcessStatus,
  });
}

function TaskDebugOverlay({ entries }: { entries: ReturnType<typeof maybeBuildDebugEntries> }) {
  const { t } = useTranslation();
  if (!entries) return null;
  return <DebugOverlay title={t("task:taskDebug")} entries={entries} />;
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

function AgentProjectTaskScope({ task, children }: { task: Task; children: ReactNode }) {
  return (
    <AgentProjectTaskProvider
      value={
        task.agent_project_id && task.agent_project_tier
          ? {
              taskId: task.id,
              projectId: task.agent_project_id,
              workspaceId: task.workspace_id,
              tier: task.agent_project_tier,
            }
          : null
      }
    >
      {children}
    </AgentProjectTaskProvider>
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
  }: Pick<
    TaskPageInnerProps,
    "task" | "effectiveSessionId" | "resumption" | "sessionPanel" | "isMobile" | "ensureSession"
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
    { task, effectiveSessionId, resumption, sessionPanel, isMobile, ensureSession },
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
        <VcsDialogsProvider
          sessionId={effectiveSessionId}
          baseBranch={taskProps.baseBranch}
          pullRequestBaseBranch={taskProps.pullRequestTarget}
          pullRequestTargetsByRepository={taskProps.pullRequestTargetsByRepository}
          taskTitle={taskProps.taskTitle}
          displayBranch={merged.worktreeBranch}
        >
          <div
            className="flex h-full min-h-0 w-full flex-col overflow-hidden bg-background"
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
                <AgentProjectTaskScope task={task}>
                  <TaskPageLayoutFeedback
                    layoutProps={layoutProps}
                    isMobile={isMobile}
                    hasPageLevelMobileFeedback={hasPageLevelMobileFeedback}
                  />
                </AgentProjectTaskScope>
              </TaskLaunchErrorProvider>
            </TaskArchivedProvider>
          </div>
        </VcsDialogsProvider>
      </PortForwardingVisibilityProvider>
    </TooltipProvider>
  );
}
