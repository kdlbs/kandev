"use client";

import { useCallback, useEffect, useMemo, useState, useSyncExternalStore } from "react";
import { IconAlertTriangle } from "@tabler/icons-react";
import { useQuery, useQueryClient, type QueryClient } from "@tanstack/react-query";
import { type Repository, type RepositoryScript, type Task } from "@/lib/types/http";
import type { Terminal } from "@/hooks/domains/session/use-terminals";
import { useRepositories } from "@/hooks/domains/workspace/use-repositories";
import { useSessionAgent } from "@/hooks/domains/session/use-session-agent";
import { useSessionResumption } from "@/hooks/domains/session/use-session-resumption";
import { useSessionAgentctl } from "@/hooks/domains/session/use-session-agentctl";
import { useTaskFocus } from "@/hooks/domains/session/use-task-focus";
import { useAppStore } from "@/components/state-provider";
import { useEnsureTaskSession } from "@/hooks/domains/session/use-ensure-task-session";
import { useTasks } from "@/hooks/use-tasks";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import type { Layout } from "react-resizable-panels";
import { taskQueryOptions, workflowStepsQueryOptions } from "@/lib/query/query-options";
import { workflowSnapshotQueryData } from "@/lib/query/workflow-snapshot-cache";
import { isPassthroughSession } from "@/lib/session/is-passthrough-session";
import {
  deriveIsAgentWorking,
  buildArchivedValue,
  hasResolvedTaskDetails,
  resolveEffectiveTask,
  resolveTaskContentState,
  syncActiveTaskSession,
} from "@/components/task/task-page-content-helpers";
import { TaskPageInner } from "@/components/task/task-page-inner";
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

export function useWorkflowStepsMapped(workflowIdOverride?: string | null) {
  const activeTaskId = useAppStore((state) => state.tasks.activeTaskId);
  const taskQuery = useQuery({
    ...taskQueryOptions(activeTaskId ?? ""),
    enabled: Boolean(activeTaskId) && !workflowIdOverride,
  });
  const workflowId = workflowIdOverride ?? taskQuery.data?.workflow_id ?? null;
  const stepsQuery = useQuery({
    ...workflowStepsQueryOptions(workflowId ?? ""),
    enabled: Boolean(workflowId),
  });
  return useMemo(
    () =>
      (stepsQuery.data ?? []).map((s) => ({
        id: s.id,
        name: s.name,
        color: s.color,
        position: s.position,
        events: s.events,
        allow_manual_move: s.allow_manual_move,
        prompt: s.prompt,
        is_start_step: s.is_start_step,
        agent_profile_id: s.agent_profile_id,
      })),
    [stepsQuery.data],
  );
}

export function useSessionPanelState(effectiveSessionId: string | null | undefined) {
  const activeTaskId = useAppStore((state) => state.tasks.activeTaskId);
  const activeTaskQuery = useQuery({
    ...taskQueryOptions(activeTaskId ?? ""),
    enabled: Boolean(activeTaskId),
  });
  const storeSessionState = useAppStore((state) =>
    effectiveSessionId ? (state.taskSessions.items[effectiveSessionId]?.state ?? null) : null,
  );
  const isSessionPassthrough = useAppStore((state) =>
    effectiveSessionId ? isPassthroughSession(state.taskSessions.items[effectiveSessionId]) : false,
  );
  // Use the task-level workflow step for the top-bar stepper. Individual sessions
  // may lag behind (e.g. a completed session stays at its old step), but the
  // task's step reflects the current workflow position and stays stable across
  // tab switches within the same task.
  const sessionWorkflowStepId = activeTaskQuery.data?.workflow_step_id ?? null;
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
  return (
    <div
      className="flex h-screen w-full items-center justify-center bg-background px-4"
      data-testid="task-loading-state"
    >
      <div className="flex min-h-24 min-w-0 flex-col items-center justify-center gap-3 text-center text-sm text-muted-foreground">
        <GridSpinner className="text-primary" />
        <span>Loading task...</span>
      </div>
    </div>
  );
}

function TaskLoadErrorState() {
  return (
    <div
      className="flex h-screen w-full items-center justify-center bg-background px-4"
      data-testid="task-load-error-state"
    >
      <div className="flex min-h-24 max-w-sm min-w-0 flex-col items-center justify-center gap-3 text-center text-sm text-muted-foreground">
        <IconAlertTriangle className="h-5 w-5 text-destructive" aria-hidden="true" />
        <div className="space-y-1">
          <div className="font-medium text-foreground">Task unavailable</div>
          <div>
            We could not load this task. It may have been deleted or you may not have access.
          </div>
        </div>
      </div>
    </div>
  );
}

export function useTaskDetails(activeTaskId: string | null, initialTask: Task | null) {
  const initialTaskId = initialTask?.id ?? null;
  const effectiveTaskId = activeTaskId ?? initialTaskId;
  const snapshotTask = useCachedWorkflowSnapshotTask(effectiveTaskId);
  const taskDetailsQuery = useQuery({
    ...taskQueryOptions(effectiveTaskId ?? ""),
    enabled: shouldFetchActiveTaskDetails(activeTaskId, initialTaskId),
    staleTime: 0,
  });
  const taskDetails = taskDetailsQuery.data?.id === effectiveTaskId ? taskDetailsQuery.data : null;
  const task = useMemo(
    () => resolveEffectiveTask(taskDetails, initialTask, snapshotTask, effectiveTaskId),
    [taskDetails, initialTask, snapshotTask, effectiveTaskId],
  );
  const hasTaskDetails = hasResolvedTaskDetails({
    effectiveTaskId,
    taskDetailsId: taskDetails?.id ?? null,
    initialTaskId,
    snapshotTaskId: snapshotTask?.id ?? null,
  });
  useTasks(task?.workflow_id ?? null);

  useEffect(() => {
    if (!taskDetailsQuery.isError) return;
    console.error("[TaskPageContent] Failed to load task details:", taskDetailsQuery.error);
  }, [taskDetailsQuery.error, taskDetailsQuery.isError]);

  return { task, taskLoadError: hasTaskDetails ? null : taskDetailsQuery.error };
}

function shouldFetchActiveTaskDetails(
  activeTaskId: string | null,
  initialTaskId: string | null,
): boolean {
  return Boolean(activeTaskId) && activeTaskId !== initialTaskId;
}

function useCachedWorkflowSnapshotTask(taskId: string | null): Task | null {
  const queryClient = useQueryClient();
  const subscribe = useCallback(
    (onStoreChange: () => void) => queryClient.getQueryCache().subscribe(onStoreChange),
    [queryClient],
  );
  const getSnapshot = useCallback(
    () => (taskId ? taskFromWorkflowSnapshots(queryClient, taskId) : null),
    [queryClient, taskId],
  );
  return useSyncExternalStore(subscribe, getSnapshot, () => null);
}

function taskFromWorkflowSnapshots(queryClient: QueryClient, taskId: string): Task | null {
  for (const snapshot of workflowSnapshotQueryData(queryClient)) {
    const task = snapshot.tasks.find((item) => item.id === taskId);
    if (task) return task;
  }
  return null;
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

  // Validate that activeSessionId belongs to activeTaskId to prevent showing
  // messages from an unrelated session when navigating to a task without sessions.
  const validatedActiveSessionId = useAppStore((state) => {
    const sid = state.tasks.activeSessionId;
    if (!sid || !activeTaskId) return null;
    const session = state.taskSessions.items[sid];
    return session?.task_id === activeTaskId ? sid : null;
  });

  const { task, taskLoadError } = useTaskDetails(activeTaskId, initialTask);

  const agent = useSessionAgent(task);
  const ensureSession = useEnsureTaskSession(task);
  const initialSessionId = sessionId ?? agent.taskSessionId ?? null;
  const effectiveSessionId = validatedActiveSessionId ?? initialSessionId;

  useEffect(() => {
    syncActiveTaskSession({
      initialTaskId: initialTask?.id,
      fallbackTaskId,
      initialSessionId,
      setActiveSessionAuto,
      setActiveTask,
    });
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
    agent,
    effectiveSessionId,
    repository,
    ensureSession,
  };
}

export function TaskPageContent({
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
  const connectionStatus = useAppStore((state) => state.connection.status);

  const { task, taskLoadError, agent, effectiveSessionId, repository, ensureSession } =
    useTaskPageData(initialTask, initialTaskId, sessionId, initialRepositories);

  const workflowSteps = useWorkflowStepsMapped(task?.workflow_id ?? null);
  const sessionPanel = useSessionPanelState(effectiveSessionId);
  const agentctlStatus = useSessionAgentctl(effectiveSessionId);
  const resumption = useSessionResumption(task?.id ?? null, effectiveSessionId);
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
    hasTaskLoadError: Boolean(taskLoadError),
  });

  if (contentState === "loading") return <TaskLoadingState />;
  if (contentState === "error") return <TaskLoadErrorState />;
  if (!task) return <TaskLoadErrorState />;

  return (
    <TaskPageInner
      task={task}
      effectiveSessionId={effectiveSessionId ?? null}
      repository={repository}
      agent={agent}
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
    />
  );
}
