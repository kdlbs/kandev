"use client";

import { useEffect, useMemo, useState } from "react";
import { useQuery } from "@tanstack/react-query";
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
import { isPassthroughSession } from "@/lib/session/is-passthrough-session";
import {
  deriveIsAgentWorking,
  buildArchivedValue,
} from "@/components/task/task-page-content-helpers";
import { TaskPageInner } from "@/components/task/task-page-inner";

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
  effectiveTaskId: string | null,
): Task | null {
  const matchingTaskDetails = taskDetails?.id === effectiveTaskId ? taskDetails : null;
  const matchingInitialTask = initialTask?.id === effectiveTaskId ? initialTask : null;
  return matchingTaskDetails ?? matchingInitialTask ?? null;
}

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

export function useTaskDetails(activeTaskId: string | null, initialTask: Task | null) {
  const effectiveTaskId = activeTaskId ?? initialTask?.id ?? null;
  const taskDetailsQuery = useQuery({
    ...taskQueryOptions(effectiveTaskId ?? ""),
    enabled: Boolean(activeTaskId) && activeTaskId !== initialTask?.id,
    staleTime: 0,
  });
  const taskDetails = taskDetailsQuery.data?.id === effectiveTaskId ? taskDetailsQuery.data : null;
  const task = useMemo(
    () => resolveEffectiveTask(taskDetails, initialTask, effectiveTaskId),
    [taskDetails, initialTask, effectiveTaskId],
  );
  useTasks(task?.workflow_id ?? null);

  useEffect(() => {
    if (!taskDetailsQuery.isError) return;
    console.error("[TaskPageContent] Failed to load task details:", taskDetailsQuery.error);
  }, [taskDetailsQuery.error, taskDetailsQuery.isError]);

  return { task };
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

  if (!isMounted || !task) return <div className="h-screen w-full bg-background" />;

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
