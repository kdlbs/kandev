"use client";

import { TaskTopBar } from "@/components/task/task-top-bar";
import { TaskLayout } from "@/components/task/task-layout";
import { DebugOverlay } from "@/components/debug-overlay";
import { type Repository, type RepositoryScript, type Task } from "@/lib/types/http";
import type { Terminal } from "@/hooks/domains/session/use-terminals";
import { isDebugUI } from "@/lib/config";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { useAppStore } from "@/components/state-provider";
import type { UseEnsureTaskSessionResult } from "@/hooks/domains/session/use-ensure-task-session";
import { EnsureSessionErrorBanner } from "@/components/task/ensure-session-error";
import type { Layout } from "react-resizable-panels";
import { TaskArchivedProvider } from "./task-archived-context";
import { SessionCommands } from "@/components/session-commands";
import { TaskPRShortcut } from "@/components/task/task-pr-shortcut";
import { useEmbeddedVscodeSupport } from "@/components/task/task-page-editor-capability";
import { VcsDialogsProvider } from "@/components/vcs/vcs-dialogs";
import { PortForwardingVisibilityProvider } from "@/components/task/port-forwarding-visibility-provider";
import {
  buildDebugEntries,
  buildArchivedValue,
  resolveTaskProps,
} from "@/components/task/task-page-content-helpers";
import type { useSessionAgent } from "@/hooks/domains/session/use-session-agent";
import type { useSessionResumption } from "@/hooks/domains/session/use-session-resumption";
import type { useSessionAgentctl } from "@/hooks/domains/session/use-session-agentctl";
import type {
  useWorkflowStepsMapped,
  useSessionPanelState,
  useMergedAgentState,
} from "./task-page-content";
import { useTranslation } from "react-i18next";

export type TaskPageInnerProps = {
  task: Task | null;
  effectiveSessionId: string | null;
  repository: Repository | null;
  agent: ReturnType<typeof useSessionAgent>;
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
  agent: ReturnType<typeof useSessionAgent>;
  merged: ReturnType<typeof useMergedAgentState>;
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
  const { taskProps, agent, merged, workflowSteps, showDebugOverlay, onToggleDebugOverlay } =
    params;
  return {
    taskId: taskProps.taskId,
    activeSessionId: params.effectiveSessionId,
    taskTitle: taskProps.taskTitle,
    onStartAgent: agent.handleStartAgent,
    onStopAgent: agent.handleStopAgent,
    isAgentRunning: agent.isAgentRunning || merged.isResumed,
    isAgentLoading: agent.isAgentLoading || merged.isResuming,
    showDebugOverlay,
    onToggleDebugOverlay,
    workflowSteps,
    currentStepId: resolveCurrentStepId(params.sessionWorkflowStepId, taskProps.workflowStepId),
    workflowId: taskProps.workflowId,
    workspaceId: taskProps.workspaceId,
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
}) {
  const { taskProps, repository, effectiveSessionId, initialScripts, initialTerminals } = params;
  return {
    workspaceId: taskProps.workspaceId,
    workflowId: taskProps.workflowId,
    sessionId: effectiveSessionId,
    repository: repository ?? null,
    initialScripts,
    initialTerminals,
    defaultLayouts: params.defaultLayouts,
    initialLayout: params.initialLayout,
    taskTitle: taskProps.taskTitle,
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
  agent,
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
}: TaskPageInnerProps) {
  const taskProps = resolveTaskProps(task, repository);
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
    agent,
    merged,
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
  });

  return { taskProps, debugEntries, topBarProps, layoutProps };
}

export function TaskPageInner(props: TaskPageInnerProps) {
  const { effectiveSessionId, task, merged, sessionPanel, archivedValue, isMobile, ensureSession } =
    props;
  const { taskProps, debugEntries, topBarProps, layoutProps } = useTaskPageDerivedProps(props);

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
            {!isMobile && <TaskTopBar {...topBarProps} />}
            {ensureSession.status === "error" && (
              <EnsureSessionErrorBanner
                error={ensureSession.error}
                onRetry={ensureSession.retry}
                workspaceId={task?.workspace_id ?? null}
              />
            )}
            <TaskArchivedProvider value={archivedValue}>
              <TaskLayout {...layoutProps} />
            </TaskArchivedProvider>
          </div>
        </VcsDialogsProvider>
      </PortForwardingVisibilityProvider>
    </TooltipProvider>
  );
}
