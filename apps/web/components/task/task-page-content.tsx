"use client";

import { useCallback, useEffect, useMemo, useState } from "react";
import { IconAlertTriangle } from "@tabler/icons-react";
import type { Repository, RepositoryScript, Task } from "@/lib/types/http";
import type { Terminal } from "@/hooks/domains/session/use-terminals";
import type { KanbanState } from "@/lib/state/slices";
import { useRepositories } from "@/hooks/domains/workspace/use-repositories";
import { useSessionAgent } from "@/hooks/domains/session/use-session-agent";
import { useSessionResumption } from "@/hooks/domains/session/use-session-resumption";
import { useSessionAgentctl } from "@/hooks/domains/session/use-session-agentctl";
import { useTaskFocus } from "@/hooks/domains/session/use-task-focus";
import { useAppStore } from "@/components/state-provider";
import { useEnsureTaskSession } from "@/hooks/domains/session/use-ensure-task-session";
import { useExternalVcsFileLinkHydration } from "@/hooks/domains/workspace/use-external-vcs-file-link";
import { fetchTask } from "@/lib/api";
import { useTasks } from "@/hooks/use-tasks";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import type { Layout } from "react-resizable-panels";
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

export function useWorkflowStepsMapped() {
  const kanbanSteps = useAppStore((state) => state.kanban.steps);
  return useMemo(
    () =>
      kanbanSteps.map((s) => ({
        id: s.id,
        name: s.title,
        color: s.color,
        position: s.position,
        events: s.events,
        allow_manual_move: s.allow_manual_move,
        prompt: s.prompt,
        is_start_step: s.is_start_step,
        agent_profile_id: s.agent_profile_id,
      })),
    [kanbanSteps],
  );
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
    const task = state.kanban.tasks.find((t: { id: string }) => t.id === taskId);
    return (task?.workflowStepId as string) ?? null;
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
  return (
    <div
      className="flex h-full min-h-0 w-full items-center justify-center bg-background px-4"
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
      className="flex h-full min-h-0 w-full items-center justify-center bg-background px-4"
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

function useTaskDetails(activeTaskId: string | null, initialTask: Task | null) {
  const [taskDetails, setTaskDetails] = useState<Task | null>(initialTask);
  const [taskLoadError, setTaskLoadError] = useState<unknown | null>(null);
  const kanbanTask = useAppStore((state) =>
    activeTaskId
      ? (state.kanban.tasks.find(
          (item: KanbanState["tasks"][number]) => item.id === activeTaskId,
        ) ?? null)
      : null,
  );
  const effectiveTaskId = activeTaskId ?? initialTask?.id ?? null;
  const task = useMemo(
    () => resolveEffectiveTask(taskDetails, initialTask, kanbanTask, effectiveTaskId),
    [taskDetails, initialTask, kanbanTask, effectiveTaskId],
  );
  const hasTaskDetails = hasResolvedTaskDetails({
    effectiveTaskId,
    taskDetailsId: taskDetails?.id ?? null,
    initialTaskId: initialTask?.id ?? null,
  });
  useTasks(task?.workflow_id ?? null);

  useEffect(() => {
    if (!activeTaskId || taskDetails?.id === activeTaskId) {
      setTaskLoadError(null);
      return;
    }
    let cancelled = false;
    setTaskLoadError(null);
    fetchTask(activeTaskId, { cache: "no-store" })
      .then((response) => {
        if (cancelled) return;
        setTaskDetails(response);
        setTaskLoadError(null);
      })
      .catch((error) => {
        if (cancelled) return;
        console.error("[TaskPageContent] Failed to load task details:", error);
        setTaskLoadError(error);
      });
    return () => {
      cancelled = true;
    };
  }, [
    activeTaskId,
    taskDetails?.id,
    taskDetails?.workspace_id,
    taskDetails?.workflow_id,
    kanbanTask,
    setTaskDetails,
  ]);

  const onTaskUnarchived = useCallback((taskId: string) => {
    setTaskDetails((current) =>
      current?.id === taskId ? { ...current, archived_at: null } : current,
    );
  }, []);

  return {
    task,
    kanbanTask,
    taskLoadError: hasTaskDetails ? null : taskLoadError,
    onTaskUnarchived,
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

  // Validate that activeSessionId belongs to activeTaskId to prevent showing
  // messages from an unrelated session when navigating to a task without sessions.
  const validatedActiveSessionId = useAppStore((state) => {
    const sid = state.tasks.activeSessionId;
    if (!sid || !activeTaskId) return null;
    const session = state.taskSessions.items[sid];
    return session?.task_id === activeTaskId ? sid : null;
  });

  const { task, taskLoadError, onTaskUnarchived } = useTaskDetails(activeTaskId, initialTask);

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
    repositories: effectiveRepositories,
    ensureSession,
    onTaskUnarchived,
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

  const {
    task,
    taskLoadError,
    agent,
    effectiveSessionId,
    repository,
    repositories,
    ensureSession,
    onTaskUnarchived,
  } = useTaskPageData(initialTask, initialTaskId, sessionId, initialRepositories);
  useExternalVcsFileLinkHydration(task, repositories);

  const workflowSteps = useWorkflowStepsMapped();
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
      onTaskUnarchived={onTaskUnarchived}
    />
  );
}
