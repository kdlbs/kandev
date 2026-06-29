"use client";

import { useEffect, useMemo, useState } from "react";
import {
  taskId as toTaskId,
  workflowId as toWorkflowId,
  workspaceId as toWorkspaceId,
  type Repository,
  type RepositoryScript,
  type Task,
} from "@/lib/types/http";
import type { Terminal } from "@/hooks/domains/session/use-terminals";
import type { KanbanState } from "@/lib/state/slices";
import { useRepositories } from "@/hooks/domains/workspace/use-repositories";
import { useSessionAgent } from "@/hooks/domains/session/use-session-agent";
import { useSessionResumption } from "@/hooks/domains/session/use-session-resumption";
import { useSessionAgentctl } from "@/hooks/domains/session/use-session-agentctl";
import { useTaskFocus } from "@/hooks/domains/session/use-task-focus";
import { useAppStore } from "@/components/state-provider";
import { useEnsureTaskSession } from "@/hooks/domains/session/use-ensure-task-session";
import { fetchTask } from "@/lib/api";
import { useTasks } from "@/hooks/use-tasks";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import type { Layout } from "react-resizable-panels";
import {
  deriveIsAgentWorking,
  buildArchivedValue,
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

function resolveEffectiveTask(
  taskDetails: Task | null,
  initialTask: Task | null,
  kanbanTask: KanbanState["tasks"][number] | null,
  effectiveTaskId: string | null,
): Task | null {
  const matchingTaskDetails = taskDetails?.id === effectiveTaskId ? taskDetails : null;
  const matchingInitialTask = initialTask?.id === effectiveTaskId ? initialTask : null;
  const baseTask = matchingTaskDetails ?? matchingInitialTask;

  if (!baseTask && !kanbanTask) return null;
  if (baseTask) return mergeBaseWithKanban(baseTask, kanbanTask);
  if (kanbanTask) return buildTaskFromKanban(kanbanTask, taskDetails, initialTask);
  return null;
}

function mergeBaseWithKanban(
  baseTask: Task,
  kanbanTask: KanbanState["tasks"][number] | null,
): Task {
  if (!kanbanTask) return baseTask;
  return {
    ...baseTask,
    title: kanbanTask.title ?? baseTask.title,
    description: kanbanTask.description ?? baseTask.description,
    workflow_step_id:
      (kanbanTask.workflowStepId as string | undefined) ?? baseTask.workflow_step_id,
    position: kanbanTask.position ?? baseTask.position,
    state: (kanbanTask.state as Task["state"] | undefined) ?? baseTask.state,
    repositories: baseTask.repositories,
  };
}

function buildTaskFromKanban(
  kanbanTask: KanbanState["tasks"][number],
  taskDetails: Task | null,
  initialTask: Task | null,
): Task {
  const prevWorkspaceId = taskDetails?.workspace_id ?? initialTask?.workspace_id;
  const prevBoardId = taskDetails?.workflow_id ?? initialTask?.workflow_id;
  return {
    id: toTaskId(kanbanTask.id),
    title: kanbanTask.title,
    description: kanbanTask.description ?? "",
    workflow_step_id: kanbanTask.workflowStepId,
    position: kanbanTask.position,
    state: kanbanTask.state ?? "CREATED",
    workspace_id: prevWorkspaceId ?? toWorkspaceId(""),
    workflow_id: prevBoardId ?? toWorkflowId(""),
    priority: 0,
    repositories: [],
    created_at: "",
    updated_at: kanbanTask.updatedAt ?? "",
  };
}

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

function syncActiveTaskSession(params: {
  initialTaskId: string | undefined;
  fallbackTaskId: string | null | undefined;
  initialSessionId: string | null;
  setActiveSession: (taskId: string, sessionId: string) => void;
  setActiveTask: (taskId: string) => void;
}) {
  const taskId = params.initialTaskId ?? params.fallbackTaskId;
  if (!taskId) return;
  if (params.initialSessionId) params.setActiveSession(taskId, params.initialSessionId);
  else params.setActiveTask(taskId);
}

function useTaskDetails(activeTaskId: string | null, initialTask: Task | null) {
  const [taskDetails, setTaskDetails] = useState<Task | null>(initialTask);
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
  useTasks(task?.workflow_id ?? null);

  useEffect(() => {
    if (!activeTaskId || taskDetails?.id === activeTaskId) return;
    fetchTask(activeTaskId, { cache: "no-store" })
      .then((response) => setTaskDetails(response))
      .catch((error) => console.error("[TaskPageContent] Failed to load task details:", error));
  }, [
    activeTaskId,
    taskDetails?.id,
    taskDetails?.workspace_id,
    taskDetails?.workflow_id,
    kanbanTask,
    setTaskDetails,
  ]);

  return { task, kanbanTask };
}

function useTaskPageData(
  initialTask: Task | null,
  fallbackTaskId: string | null | undefined,
  sessionId: string | null,
  initialRepositories: Repository[],
) {
  const activeTaskId = useAppStore((state) => state.tasks.activeTaskId);
  const setActiveSession = useAppStore((state) => state.setActiveSession);
  const setActiveTask = useAppStore((state) => state.setActiveTask);

  // Validate that activeSessionId belongs to activeTaskId to prevent showing
  // messages from an unrelated session when navigating to a task without sessions.
  const validatedActiveSessionId = useAppStore((state) => {
    const sid = state.tasks.activeSessionId;
    if (!sid || !activeTaskId) return null;
    const session = state.taskSessions.items[sid];
    return session?.task_id === activeTaskId ? sid : null;
  });

  const { task } = useTaskDetails(activeTaskId, initialTask);

  const agent = useSessionAgent(task);
  const ensureSession = useEnsureTaskSession(task);
  const initialSessionId = sessionId ?? agent.taskSessionId ?? null;
  const effectiveSessionId = validatedActiveSessionId ?? initialSessionId;

  useEffect(() => {
    syncActiveTaskSession({
      initialTaskId: initialTask?.id,
      fallbackTaskId,
      initialSessionId,
      setActiveSession,
      setActiveTask,
    });
  }, [initialTask?.id, fallbackTaskId, initialSessionId, setActiveSession, setActiveTask]);

  const { repositories } = useRepositories(task?.workspace_id ?? null, Boolean(task?.workspace_id));
  const effectiveRepositories = repositories.length ? repositories : initialRepositories;
  const repository = useMemo(
    () =>
      effectiveRepositories.find(
        (item: Repository) => item.id === task?.repositories?.[0]?.repository_id,
      ) ?? null,
    [effectiveRepositories, task?.repositories],
  );

  return { task, agent, effectiveSessionId, repository, ensureSession };
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

  const { task, agent, effectiveSessionId, repository, ensureSession } = useTaskPageData(
    initialTask,
    initialTaskId,
    sessionId,
    initialRepositories,
  );

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

  if (!isMounted || !task) {
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
