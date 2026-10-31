import type { Layout } from "react-resizable-panels";
import type { Canvas } from "@/lib/api/domains/canvas-api";
import type { TaskCanvasesLoadStatus } from "@/hooks/domains/task/use-task-canvases";
import type { Terminal } from "@/hooks/domains/session/use-terminals";
import type { useSessionAgentctl } from "@/hooks/domains/session/use-session-agentctl";
import type { useSessionResumption } from "@/hooks/domains/session/use-session-resumption";
import type { Repository, RepositoryScript, Task } from "@/lib/types/http";
import {
  buildDebugEntries,
  resolveTaskProps,
  resolveWorkflowCurrentStepId,
  useTaskActionsMenuBoardRow,
} from "@/components/task/task-page-content-helpers";
import type {
  useMergedAgentState,
  useSessionPanelState,
  useWorkflowStepsMapped,
} from "./task-page-content";

export type RemoteExecutorStatus = {
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

export function resolveRemoteExecutor(status?: RemoteExecutorStatus | null) {
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

export function buildTaskTopBarProps(params: {
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

export function buildTaskLayoutProps(params: {
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

export function maybeBuildDebugEntries(params: {
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
