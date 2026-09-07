"use client";

import { useCallback, useEffect, useState } from "react";
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
  EnsureSessionErrorBanner,
  SessionRecoveryFeedback,
} from "@/components/task/ensure-session-error";
import { TaskMoveErrorBanner } from "@/components/task/task-move-error-banner";
import type { Layout } from "react-resizable-panels";
import { TaskArchivedProvider } from "./task-archived-context";
import { SessionCommands } from "@/components/session-commands";
import { TaskPRShortcut } from "@/components/task/task-pr-shortcut";
import { useEmbeddedVscodeSupport } from "@/components/task/task-page-editor-capability";
import { VcsDialogsProvider } from "@/components/vcs/vcs-dialogs";
import { PortForwardingVisibilityProvider } from "@/components/task/port-forwarding-visibility-provider";
import { TaskLaunchErrorProvider } from "@/components/task/task-launch-error-context";
import {
  buildDebugEntries,
  buildArchivedValue,
  resolveTaskProps,
  selectWorkspaceRepositories,
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

// Prefer the session-level step (delivered direct via session.state_changed) over the task-level step (routed through the hub broadcast and slightly stale).
function resolveCurrentStepId(
  sessionStepId: string | null,
  taskStepId: string | null,
): string | null {
  return sessionStepId || taskStepId || null;
}

function buildTaskTopBarProps(params: {
  taskProps: ReturnType<typeof resolveTaskProps>;
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
    showDebugOverlay,
    onToggleDebugOverlay,
    workflowSteps,
    currentStepId: resolveCurrentStepId(params.sessionWorkflowStepId, taskProps.workflowStepId),
    workflowId: taskProps.workflowId,
    workspaceId: taskProps.workspaceId,
    projectId: taskProps.projectId,
    issueUrl: taskProps.issueUrl,
    issueNumber: taskProps.issueNumber,
    isArchived: taskProps.isArchived,
    embeddedVscodeSupported: params.embeddedVscodeSupported,
    remoteExecutorType: params.remote.remoteExecutorType,
    officeTaskHref: params.officeTaskHref,
    onTaskUnarchived: params.onTaskUnarchived,
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
  taskCanvases?: Canvas[];
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
    taskCanvases: params.taskCanvases ?? [],
    taskTitle: taskProps.taskTitle,
    repositoryLabel: taskProps.repositoryLabel,
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

/**
 * Derives everything the task page renders from its inputs: the resolved task
 * props plus the three prop bundles handed to the debug overlay, top bar, and
 * layout. Kept out of `TaskPageInner` so that component stays a wiring shell.
 */
function useTaskPageDerivedProps({
  task,
  effectiveSessionId,
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
}: TaskPageInnerProps) {
  const workspaceRepositories = useAppStore((state) =>
    selectWorkspaceRepositories(state.repositories.itemsByWorkspaceId, task?.workspace_id),
  );
  const taskProps = resolveTaskProps(task, repository, workspaceRepositories);
  const remote = resolveRemoteExecutor(resumption.sessionStatus as RemoteExecutorStatus | null);
  const embeddedVscode = useEmbeddedVscodeSupport(effectiveSessionId, resumption.sessionStatus);
  const activeSessionMetadata = useAppStore((state) =>
    effectiveSessionId ? (state.taskSessions.items[effectiveSessionId]?.metadata ?? null) : null,
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
    taskProps,
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
    taskCanvases,
  });

  return { taskProps, debugEntries, topBarProps, layoutProps };
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
  const { taskProps, debugEntries, topBarProps, layoutProps } = useTaskPageDerivedProps(props);
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
          <div className="flex h-full min-h-0 w-full flex-col overflow-hidden bg-background">
            <SessionCommands
              sessionId={effectiveSessionId}
              baseBranch={taskProps.baseBranch}
              isAgentRunning={merged.isAgentWorking}
              hasWorktree={Boolean(merged.worktreeBranch)}
              isPassthrough={sessionPanel.isSessionPassthrough}
              isTaskArchived={archivedValue.isArchived}
            />
            <TaskPRShortcut taskId={taskProps.taskId} />
            <TaskDebugOverlay entries={debugEntries} />
            {!isMobile && (
              <TaskTopBar
                {...topBarProps}
                onMoveStart={clearTaskMoveError}
                onMoveError={reportTaskMoveError}
              />
            )}
            {taskMoveError !== null && <TaskMoveErrorBanner error={taskMoveError} />}
            {ensureSession.status === "error" && (
              <EnsureSessionErrorBanner
                error={ensureSession.error}
                onRetry={ensureSession.retry}
                workspaceId={task?.workspace_id ?? null}
              />
            )}
            <SessionRecoveryFeedback
              error={props.resumption.error}
              notice={props.resumption.notice}
              onRetry={() => void props.resumption.resumeSession()}
              workspaceId={task?.workspace_id ?? null}
            />
            <TaskArchivedProvider value={archivedValue}>
              <TaskLaunchErrorProvider
                value={{
                  taskId: task.id,
                  workspaceId: task.workspace_id,
                  statusSummary: task.status_summary,
                  repositories: task.repositories,
                }}
              >
                <TaskLayout {...layoutProps} />
              </TaskLaunchErrorProvider>
            </TaskArchivedProvider>
          </div>
        </VcsDialogsProvider>
      </PortForwardingVisibilityProvider>
    </TooltipProvider>
  );
}
