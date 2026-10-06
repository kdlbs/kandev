"use client";

import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { IconAlertTriangle } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import Link from "@/components/routing/app-link";
import { useTranslation } from "react-i18next";
import type { Repository, RepositoryScript, Task } from "@/lib/types/http";
import type { Terminal } from "@/hooks/domains/session/use-terminals";
import { useRepositories } from "@/hooks/domains/workspace/use-repositories";
import { useSessionAgent } from "@/hooks/domains/session/use-session-agent";
import { useSessionResumption } from "@/hooks/domains/session/use-session-resumption";
import { useSessionAgentctl } from "@/hooks/domains/session/use-session-agentctl";
import { useTaskFocus } from "@/hooks/domains/session/use-task-focus";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import { useTaskRouteSessionHydrated } from "@/components/task/task-route-session-hydration";
import { useEnsureTaskSession } from "@/hooks/domains/session/use-ensure-task-session";
import { useExternalVcsFileLinkHydration } from "@/hooks/domains/workspace/use-external-vcs-file-link";
import { linkToTaskOverview } from "@/lib/links";
import {
  isTemporaryTaskNavigationError,
  readTaskNavigationIdentity,
  useTaskNavigationReadState,
} from "@/lib/state/task-navigation-reads";
import { useWorkflowSnapshotById } from "@/hooks/domains/kanban/use-all-workflow-snapshots";
import { useWorkflowStepsById } from "@/hooks/domains/kanban/use-workflow-steps-by-id";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import { useFeature } from "@/hooks/domains/features/use-feature";
import { useTaskCanvasesStateForTask } from "@/hooks/domains/task/use-task-canvases";
import { FOREGROUND_EVENT_COALESCE_MS, useForegroundRefresh } from "@/hooks/use-foreground-refresh";
import type { Layout } from "react-resizable-panels";
import {
  deriveIsAgentWorking,
  buildArchivedValue,
  hasResolvedTaskDetails,
  resolveEffectiveTask,
  resolveLatestTaskProjection,
  resolveTaskContentState,
  syncActiveTaskSession,
} from "@/components/task/task-page-content-helpers";
import { TaskPageInner } from "@/components/task/task-page-inner";
import { TaskRemovalBoundary } from "@/components/task/task-removal-boundary";
import { GridSpinner } from "@/components/grid-spinner";

type TaskPageContentProps = {
  task: Task | null;
  taskId?: string | null;
  sessionId?: string | null;
  initialRepositories?: Repository[];
  initialScripts?: RepositoryScript[];
  initialTerminals?: Terminal[];
  defaultLayouts?: Record<string, Layout>;
  initialLayout?: string | null;
  officeTaskHref?: string | null;
};

export function useWorkflowStepsMapped(workflowId: string | null | undefined) {
  return useWorkflowStepsById(workflowId);
}

function useTaskWorkflowSnapshot(task: Task | null) {
  useWorkflowSnapshotById(task?.workspace_id ?? null, task?.workflow_id ?? null);
}

function getTaskArchivedState(task: Task | null): boolean | null {
  return task ? task.archived_at != null : null;
}

export function useSessionPanelState(effectiveSessionId: string | null | undefined) {
  const storeSessionState = useAppStore((state) =>
    effectiveSessionId ? (state.taskSessions.items[effectiveSessionId]?.state ?? null) : null,
  );
  const isSessionPassthrough = useAppStore((state) =>
    effectiveSessionId
      ? state.taskSessions.items[effectiveSessionId]?.is_passthrough === true
      : false,
  );
  // Use the task-level workflow step for the top-bar stepper. Individual sessions
  // may lag behind (e.g. a completed session stays at its old step), but the
  // task's step reflects the current workflow position and stays stable across
  // tab switches within the same task.
  const sessionWorkflowStepId = useAppStore((state) => {
    const taskId = state.tasks.activeTaskId;
    if (!taskId) return null;
    return (
      resolveLatestTaskProjection(taskId, state.kanban.tasks, state.kanbanMulti.snapshots)
        ?.workflowStepId ?? null
    );
  });
  const previewOpen = useAppStore((state) =>
    effectiveSessionId ? (state.previewPanel.openBySessionId[effectiveSessionId] ?? false) : false,
  );
  const previewStage = useAppStore((state) =>
    effectiveSessionId
      ? (state.previewPanel.stageBySessionId[effectiveSessionId] ?? "closed")
      : "closed",
  );
  const previewUrl = useAppStore((state) =>
    effectiveSessionId ? (state.previewPanel.urlBySessionId[effectiveSessionId] ?? "") : "",
  );
  const devProcessId = useAppStore((state) =>
    effectiveSessionId ? state.processes.devProcessBySessionId[effectiveSessionId] : undefined,
  );
  const devProcessStatus = useAppStore((state) =>
    devProcessId ? (state.processes.processesById[devProcessId]?.status ?? null) : null,
  );
  return {
    storeSessionState,
    isSessionPassthrough,
    sessionWorkflowStepId,
    previewOpen,
    previewStage,
    previewUrl,
    devProcessId,
    devProcessStatus,
  };
}

export function useMergedAgentState(
  agent: ReturnType<typeof useSessionAgent>,
  resumption: ReturnType<typeof useSessionResumption>,
  sessionPanel: ReturnType<typeof useSessionPanelState>,
  effectiveSessionId: string | null | undefined,
  task: Task | null,
) {
  const isResuming =
    resumption.resumptionState === "checking" || resumption.resumptionState === "resuming";
  const isResumed =
    resumption.resumptionState === "resumed" || resumption.resumptionState === "running";
  const taskSessionState = sessionPanel.storeSessionState ?? agent.taskSessionState;
  const worktreePath = effectiveSessionId
    ? (resumption.worktreePath ?? agent.worktreePath)
    : agent.worktreePath;
  const worktreeBranch = effectiveSessionId
    ? (resumption.worktreeBranch ?? agent.worktreeBranch)
    : agent.worktreeBranch;
  const isAgentWorking = deriveIsAgentWorking(
    taskSessionState,
    agent.isAgentRunning,
    task?.state ?? null,
  );
  return { isResuming, isResumed, taskSessionState, worktreePath, worktreeBranch, isAgentWorking };
}

function TaskLoadingState() {
  const { t } = useTranslation();

  return (
    <div
      className="flex h-full min-h-0 w-full items-center justify-center bg-background px-4"
      data-testid="task-loading-state"
    >
      <div className="flex min-h-24 min-w-0 flex-col items-center justify-center gap-3 text-center text-sm text-muted-foreground">
        <GridSpinner className="text-primary" />
        <span>{t("common:taskLoading")}</span>
      </div>
    </div>
  );
}

export function TaskLoadErrorState({
  temporaryError = false,
  retrying = false,
  onRetry,
}: {
  temporaryError?: boolean;
  retrying?: boolean;
  onRetry?: () => void;
}) {
  const { t } = useTranslation();
  const activeWorkspaceId = useAppStore((state) => state.workspaces.activeId);

  return (
    <div
      className="flex h-full min-h-0 w-full items-center justify-center bg-background px-4"
      data-testid="task-load-error-state"
    >
      <div
        className="flex min-h-24 max-w-sm min-w-0 flex-col items-center justify-center gap-3 text-center text-sm text-muted-foreground"
        role={temporaryError ? "status" : undefined}
        aria-live={temporaryError ? "polite" : undefined}
        aria-busy={retrying}
      >
        <IconAlertTriangle className="h-5 w-5 text-destructive" aria-hidden="true" />
        <div className="space-y-1">
          <div className="font-medium text-foreground">
            {t(temporaryError ? "common:taskReadTemporarilyUnavailable" : "common:taskUnavailable")}
          </div>
          <div>
            {t(
              temporaryError
                ? "common:taskReadTemporarilyUnavailableDescription"
                : "common:taskUnavailableDescription",
            )}
          </div>
        </div>
        {temporaryError && onRetry ? (
          <Button
            size="default"
            className="w-full max-w-xs"
            disabled={retrying}
            onClick={onRetry}
            data-testid="task-read-retry"
          >
            {t(retrying ? "task:retrying" : "task:retry")}
          </Button>
        ) : null}
        <Button
          asChild
          size="default"
          className="w-full max-w-xs px-3"
          data-testid="task-unavailable-overview-link"
        >
          <Link href={linkToTaskOverview({ workspaceId: activeWorkspaceId ?? undefined })}>
            {t("common:backToTaskOverview")}
          </Link>
        </Button>
      </div>
    </div>
  );
}

function isTaskNavigationReadPending(readState: ReturnType<typeof useTaskNavigationReadState>) {
  return readState.phase === "loading" || readState.phase === "retrying";
}

type LoadTaskDetails = (
  refresh?: boolean,
  taskId?: string | null,
  manualRetry?: boolean,
  supersede?: boolean,
  foregroundRecoveryEpisode?: object,
) => Promise<void>;

function useForegroundTaskDetailsRefresh(taskId: string | null, loadTaskDetails: LoadTaskDetails) {
  const episodeRef = useRef<{ startedAt: number; token: object } | null>(null);
  return useCallback(() => {
    const now = Date.now();
    let episode = episodeRef.current;
    if (!episode || now - episode.startedAt >= FOREGROUND_EVENT_COALESCE_MS) {
      episode = { startedAt: now, token: {} };
      episodeRef.current = episode;
    }
    return loadTaskDetails(true, taskId, false, false, episode.token);
  }, [loadTaskDetails, taskId]);
}

function useTaskDetailsReconnectRefresh(
  connectionStatus: string,
  routeDataReady: boolean,
  refreshFromForeground: () => Promise<void>,
) {
  const previousStatus = useRef(connectionStatus);
  const refreshPending = useRef(false);
  useEffect(() => {
    const reconnected = previousStatus.current !== "connected" && connectionStatus === "connected";
    previousStatus.current = connectionStatus;
    if (reconnected) refreshPending.current = true;
    if (connectionStatus === "connected" && routeDataReady && refreshPending.current) {
      refreshPending.current = false;
      void refreshFromForeground();
    }
  }, [connectionStatus, routeDataReady, refreshFromForeground]);
}

export function useTaskDetails(activeTaskId: string | null, initialTask: Task | null) {
  const store = useAppStoreApi();
  const routeDataReady = useTaskRouteSessionHydrated();
  const effectiveTaskId = initialTask?.id ?? activeTaskId ?? null;
  const navigationReadState = useTaskNavigationReadState(store, effectiveTaskId);
  const [taskDetails, setTaskDetails] = useState<Task | null>(null);
  const [taskLoadError, setTaskLoadError] = useState<unknown | null>(null);
  const connectionStatus = useAppStore((state) => state.connection.status);
  const effectiveTaskIdRef = useRef(effectiveTaskId);
  const taskDetailsRequestIdRef = useRef(0);
  effectiveTaskIdRef.current = effectiveTaskId;
  const isCurrentTaskDetailsRequest = (requestId: number, taskId: string) =>
    requestId === taskDetailsRequestIdRef.current && effectiveTaskIdRef.current === taskId;
  const kanbanTask = useAppStore((state) =>
    resolveLatestTaskProjection(effectiveTaskId, state.kanban.tasks, state.kanbanMulti.snapshots),
  );
  const task = useMemo(
    () => resolveEffectiveTask(taskDetails, initialTask, kanbanTask, effectiveTaskId),
    [taskDetails, initialTask, kanbanTask, effectiveTaskId],
  );
  const hasTaskDetails = hasResolvedTaskDetails({
    effectiveTaskId,
    taskDetailsId: taskDetails?.id ?? null,
    initialTaskId: initialTask?.id ?? null,
  });
  const loadTaskDetails = useCallback(
    async (
      refresh = true,
      taskId = effectiveTaskId,
      manualRetry = false,
      supersede = false,
      foregroundRecoveryEpisode?: object,
    ) => {
      if (!taskId || effectiveTaskIdRef.current !== taskId) return;
      const requestedTaskId = taskId;
      const requestId = ++taskDetailsRequestIdRef.current;
      try {
        const response = await readTaskNavigationIdentity(store, requestedTaskId, {
          refresh,
          manualRetry,
          supersede,
          foregroundRecoveryEpisode,
        });
        if (!isCurrentTaskDetailsRequest(requestId, requestedTaskId)) return;
        setTaskDetails(response.task);
        setTaskLoadError(null);
      } catch (error) {
        if (!isCurrentTaskDetailsRequest(requestId, requestedTaskId)) return;
        console.error("[TaskPageContent] Failed to load task details:", error);
        setTaskLoadError(error);
      }
    },
    [effectiveTaskId, store],
  );
  const refreshFromForeground = useForegroundTaskDetailsRefresh(effectiveTaskId, loadTaskDetails);

  useEffect(() => {
    if (
      !routeDataReady ||
      !effectiveTaskId ||
      taskDetails?.id === effectiveTaskId ||
      initialTask?.id === effectiveTaskId
    ) {
      setTaskLoadError(null);
      return;
    }
    setTaskLoadError(null);
    void loadTaskDetails(false, effectiveTaskId);
  }, [routeDataReady, effectiveTaskId, taskDetails?.id, initialTask?.id, loadTaskDetails]);

  useTaskDetailsReconnectRefresh(connectionStatus, routeDataReady, refreshFromForeground);

  useForegroundRefresh(
    refreshFromForeground,
    routeDataReady && Boolean(effectiveTaskId),
    effectiveTaskId,
  );

  const onTaskUnarchived = useCallback(
    (taskId: string) => {
      if (effectiveTaskId !== taskId) return;
      void loadTaskDetails(true, taskId, false, true);
    },
    [effectiveTaskId, loadTaskDetails],
  );

  return {
    task,
    kanbanTask,
    taskLoadError,
    hasTaskDetails,
    navigationReadState,
    onTaskUnarchived,
    refreshTask: loadTaskDetails,
    retryTaskRead: () => loadTaskDetails(true, effectiveTaskId, true),
    isTemporaryTaskReadError:
      isTemporaryTaskNavigationError(taskLoadError) ||
      (navigationReadState.phase === "failed" && navigationReadState.temporary),
  };
}

function useTaskPageData(
  initialTask: Task | null,
  fallbackTaskId: string | null | undefined,
  sessionId: string | null,
  initialRepositories: Repository[],
) {
  const activeTaskId = useAppStore((state) => state.tasks.activeTaskId);
  const setActiveSessionAuto = useAppStore((state) => state.setActiveSessionAuto);
  const setActiveTask = useAppStore((state) => state.setActiveTask);
  const previousRouteTaskId = useRef<string | null | undefined>(undefined);
  // Route synchronization must see the current selection when route data or
  // delayed session hydration changes, but a sidebar selection must not
  // retrigger it and overwrite the session chosen by task selection.
  const activeTaskIdForSyncRef = useRef(activeTaskId);
  activeTaskIdForSyncRef.current = activeTaskId;

  // Validate that activeSessionId belongs to activeTaskId to prevent showing
  // messages from an unrelated session when navigating to a task without sessions.
  const validatedActiveSessionId = useAppStore((state) => {
    const sid = state.tasks.activeSessionId;
    if (!sid || !activeTaskId) return null;
    const session = state.taskSessions.items[sid];
    return session?.task_id === activeTaskId ? sid : null;
  });

  const {
    task,
    taskLoadError,
    hasTaskDetails,
    navigationReadState,
    onTaskUnarchived,
    refreshTask,
    retryTaskRead,
    isTemporaryTaskReadError,
  } = useTaskDetails(fallbackTaskId ?? activeTaskId, initialTask);

  const agent = useSessionAgent(task);
  const ensureSession = useEnsureTaskSession({
    id: task?.id,
    isArchived: task?.archived_at != null,
    archiveStateKnown: task !== null,
    workflowStepId: task?.workflow_step_id,
    workflowId: task?.workflow_id,
  });
  const initialSessionId = sessionId ?? agent.taskSessionId ?? null;
  const effectiveSessionId = validatedActiveSessionId ?? initialSessionId;

  useEffect(() => {
    const applied = syncActiveTaskSession({
      initialTaskId: initialTask?.id,
      fallbackTaskId,
      initialSessionId,
      activeTaskId: activeTaskIdForSyncRef.current,
      previousRouteTaskId: previousRouteTaskId.current,
      setActiveSessionAuto,
      setActiveTask,
    });
    if (applied) previousRouteTaskId.current = initialTask?.id ?? fallbackTaskId;
  }, [initialTask?.id, fallbackTaskId, initialSessionId, setActiveSessionAuto, setActiveTask]);

  const { repositories } = useRepositories(task?.workspace_id ?? null, Boolean(task?.workspace_id));
  const effectiveRepositories = repositories.length ? repositories : initialRepositories;
  const repository = useMemo(
    () =>
      effectiveRepositories.find(
        (item: Repository) => item.id === task?.repositories?.[0]?.repository_id,
      ) ?? null,
    [effectiveRepositories, task?.repositories],
  );

  return {
    task,
    taskLoadError,
    hasTaskDetails,
    navigationReadState,
    retryTaskRead,
    isTemporaryTaskReadError,
    agent,
    effectiveSessionId,
    repository,
    repositories: effectiveRepositories,
    ensureSession,
    onTaskUnarchived,
    refreshTask,
  };
}

function TaskPageContentLive({
  task: initialTask,
  taskId: initialTaskId = null,
  sessionId = null,
  initialRepositories = [],
  initialScripts = [],
  initialTerminals,
  defaultLayouts = {},
  initialLayout,
  officeTaskHref = null,
}: TaskPageContentProps) {
  const [isMounted, setIsMounted] = useState(false);
  const [showDebugOverlay, setShowDebugOverlay] = useState(false);
  const { isMobile } = useResponsiveBreakpoint();
  const canvasesEnabled = useFeature("canvases");
  const connectionStatus = useAppStore((state) => state.connection.status);

  const {
    task,
    taskLoadError,
    hasTaskDetails,
    navigationReadState,
    retryTaskRead,
    isTemporaryTaskReadError,
    agent,
    effectiveSessionId,
    repository,
    repositories,
    ensureSession,
    onTaskUnarchived,
    refreshTask,
  } = useTaskPageData(initialTask, initialTaskId, sessionId, initialRepositories);
  useTaskWorkflowSnapshot(task);
  const taskCanvasesState = useTaskCanvasesStateForTask(task, canvasesEnabled);
  useExternalVcsFileLinkHydration(task, repositories);

  const workflowSteps = useWorkflowStepsMapped(task?.workflow_id);
  const sessionPanel = useSessionPanelState(effectiveSessionId);
  const agentctlStatus = useSessionAgentctl(effectiveSessionId);
  const resumption = useSessionResumption(
    task?.id ?? null,
    effectiveSessionId,
    getTaskArchivedState(task),
    { onTaskArchiveConflict: refreshTask },
  );
  const merged = useMergedAgentState(agent, resumption, sessionPanel, effectiveSessionId, task);
  const archivedValue = useMemo(() => buildArchivedValue(task, repository), [task, repository]);
  // Mark this session as actively focused so the backend lifts polling to fast.
  // Sidebar cards subscribe but never focus, so they stay on the cheap slow tier.
  useTaskFocus(effectiveSessionId);

  useEffect(() => {
    queueMicrotask(() => setIsMounted(true));
  }, []);

  const contentState = resolveTaskContentState({
    isMounted,
    hasTask: Boolean(task),
    hasTaskDetails,
    hasTaskLoadError: Boolean(taskLoadError),
  });

  if (contentState === "loading") return <TaskLoadingState />;
  if (contentState === "error") {
    return (
      <TaskLoadErrorState
        temporaryError={isTemporaryTaskReadError}
        retrying={isTaskNavigationReadPending(navigationReadState)}
        onRetry={retryTaskRead}
      />
    );
  }
  if (!task) return <TaskLoadErrorState />;

  return (
    <TaskPageInner
      task={task}
      effectiveSessionId={effectiveSessionId ?? null}
      repository={repository}
      merged={merged}
      resumption={resumption}
      sessionPanel={sessionPanel}
      agentctlStatus={agentctlStatus}
      connectionStatus={connectionStatus}
      workflowSteps={workflowSteps}
      archivedValue={archivedValue}
      isMobile={isMobile}
      showDebugOverlay={showDebugOverlay}
      onToggleDebugOverlay={() => setShowDebugOverlay((prev) => !prev)}
      initialScripts={initialScripts}
      initialTerminals={initialTerminals}
      defaultLayouts={defaultLayouts}
      initialLayout={initialLayout}
      officeTaskHref={officeTaskHref}
      ensureSession={ensureSession}
      onTaskUnarchived={onTaskUnarchived}
      taskCanvases={taskCanvasesState.canvases}
      taskCanvasesStatus={taskCanvasesState.status}
      taskReadRecovery={{
        temporaryError: isTemporaryTaskReadError,
        retrying: isTaskNavigationReadPending(navigationReadState),
        onRetry: retryTaskRead,
      }}
    />
  );
}

export function TaskPageContent(props: TaskPageContentProps) {
  const taskId = props.taskId ?? props.task?.id ?? null;

  return (
    <TaskRemovalBoundary taskId={taskId}>
      <TaskPageContentLive {...props} />
    </TaskRemovalBoundary>
  );
}
