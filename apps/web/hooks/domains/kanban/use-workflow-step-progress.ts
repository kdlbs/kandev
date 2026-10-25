"use client";

import { useCallback, useMemo } from "react";
import { useAppStore } from "@/components/state-provider";
import type { AppState } from "@/lib/state/app-state-types";

export type WorkflowStepProgressStatus =
  | "idle"
  | "moving"
  | "preparing"
  | "starting"
  | "cancelling"
  | "running"
  | "waiting"
  | "idle_agent"
  | "not_started"
  | "completed"
  | "failed"
  | "cancelled";

export type WorkflowStepProgress = {
  status: WorkflowStepProgressStatus;
  isPending: boolean;
  sessionId: string | null;
  agentLabel: string | null;
};

export type WorkflowStepProgressInput = {
  stepId: string;
  currentStepId?: string | null;
  movingToStepId?: string | null;
  taskState?: string | null;
  primarySessionId?: string | null;
  primarySessionState?: string | null;
  cancellationPending?: boolean;
  agentLabel?: string | null;
};

export type WorkflowProgressTaskProjection = {
  id: string;
  state?: string | null;
  primarySessionId?: string | null;
  primarySessionState?: string | null;
  primarySessionCancellationPending?: boolean;
  primaryAgentName?: string | null;
  primaryAgentProfileId?: string | null;
  statusSummary?: {
    primary_session?: { id: string; state: string } | null;
  } | null;
};

export type WorkflowProgressSessionProjection = {
  id: string;
  task_id: string;
  state: string;
  is_primary?: boolean;
  cancellation_pending?: boolean;
  agent_profile_id?: string;
  agent_profile_snapshot?: Record<string, unknown> | null;
};

export type WorkflowProgressAgentProfile = {
  id: string;
  label: string;
};

export type WorkflowProgressEvidenceInput = {
  task: WorkflowProgressTaskProjection | null;
  sessionsById: Record<string, WorkflowProgressSessionProjection>;
  sessionsForTask: WorkflowProgressSessionProjection[];
  agentProfiles: WorkflowProgressAgentProfile[];
  preferTaskProjection?: boolean;
};

export type WorkflowProgressEvidence = {
  sessionId: string | null;
  sessionState: string | null;
  cancellationPending: boolean;
  agentLabel: string | null;
};

const TERMINAL_STATUS_BY_STATE: Record<string, WorkflowStepProgressStatus> = {
  COMPLETED: "completed",
  FAILED: "failed",
  CANCELLED: "cancelled",
};
const EMPTY_SESSIONS_FOR_TASK: WorkflowProgressSessionProjection[] = [];
const EMPTY_SESSIONS_BY_ID: Record<string, WorkflowProgressSessionProjection> = {};

function isSessionForTask(
  session: WorkflowProgressSessionProjection | undefined,
  taskId: string,
): session is WorkflowProgressSessionProjection {
  return Boolean(session && session.task_id === taskId);
}

function snapshotLabel(session: WorkflowProgressSessionProjection | undefined): string | null {
  const label = session?.agent_profile_snapshot?.label;
  return typeof label === "string" && label.length > 0 ? label : null;
}

function projectedSessionId(task: WorkflowProgressTaskProjection): string | null {
  return task.primarySessionId ?? task.statusSummary?.primary_session?.id ?? null;
}

function findPrimarySession(
  task: WorkflowProgressTaskProjection,
  sessionId: string | null,
  sessionsById: Record<string, WorkflowProgressSessionProjection>,
  sessionsForTask: WorkflowProgressSessionProjection[],
): WorkflowProgressSessionProjection | undefined {
  if (!sessionId) {
    const candidates = new Map<string, WorkflowProgressSessionProjection>();
    for (const session of sessionsForTask) {
      if (session.task_id === task.id && session.is_primary) candidates.set(session.id, session);
    }
    for (const session of Object.values(sessionsById)) {
      if (session.task_id === task.id && session.is_primary) candidates.set(session.id, session);
    }
    return candidates.size === 1 ? candidates.values().next().value : undefined;
  }

  const loadedSession = sessionsById[sessionId];
  if (isSessionForTask(loadedSession, task.id)) return loadedSession;

  return (
    sessionsForTask.find((session) => session.id === sessionId && session.task_id === task.id) ??
    Object.values(sessionsById).find(
      (session) => session.id === sessionId && session.task_id === task.id,
    )
  );
}

function profileLabel(
  profileId: string | null | undefined,
  agentProfiles: WorkflowProgressAgentProfile[],
): string | null {
  if (!profileId) return null;
  return agentProfiles.find((profile) => profile.id === profileId)?.label ?? null;
}

function resolvedSessionState(
  loadedSession: WorkflowProgressSessionProjection | undefined,
  task: WorkflowProgressTaskProjection,
  projectedSession: { state: string } | null | undefined,
  preferTaskProjection: boolean,
): string | null {
  if (preferTaskProjection) {
    return task.primarySessionState ?? projectedSession?.state ?? loadedSession?.state ?? null;
  }
  return loadedSession?.state ?? task.primarySessionState ?? projectedSession?.state ?? null;
}

function resolvedAgentLabel(
  loadedSession: WorkflowProgressSessionProjection | undefined,
  task: WorkflowProgressTaskProjection,
  agentProfiles: WorkflowProgressAgentProfile[],
): string | null {
  return (
    snapshotLabel(loadedSession) ??
    profileLabel(loadedSession?.agent_profile_id ?? task.primaryAgentProfileId, agentProfiles) ??
    task.primaryAgentName ??
    null
  );
}

export function resolveWorkflowProgressEvidence({
  task,
  sessionsById,
  sessionsForTask,
  agentProfiles,
  preferTaskProjection = false,
}: WorkflowProgressEvidenceInput): WorkflowProgressEvidence {
  if (!task) {
    return {
      sessionId: null,
      sessionState: null,
      cancellationPending: false,
      agentLabel: null,
    };
  }

  const projectedSession = task.statusSummary?.primary_session;
  const sessionIdFromProjection = projectedSessionId(task);
  const loadedPrimarySession = findPrimarySession(
    task,
    sessionIdFromProjection,
    sessionsById,
    sessionsForTask,
  );

  const sessionId = loadedPrimarySession?.id ?? sessionIdFromProjection;
  const sessionState = resolvedSessionState(
    loadedPrimarySession,
    task,
    projectedSession,
    preferTaskProjection,
  );
  const cancellationPending =
    loadedPrimarySession?.cancellation_pending ?? task.primarySessionCancellationPending ?? false;
  return {
    sessionId,
    sessionState,
    cancellationPending,
    agentLabel: resolvedAgentLabel(loadedPrimarySession, task, agentProfiles),
  };
}

function progressForTerminalState(state: string | null | undefined) {
  return state ? TERMINAL_STATUS_BY_STATE[state] : undefined;
}

function progressStatusForEvidence(
  taskState: string | null | undefined,
  primarySessionState: string | null | undefined,
  cancellationPending: boolean,
): WorkflowStepProgressStatus {
  const terminalTaskStatus = progressForTerminalState(taskState);
  if (terminalTaskStatus) return terminalTaskStatus;

  const terminalSessionStatus = progressForTerminalState(primarySessionState);
  // A task that has explicitly returned to its pre-start state can still
  // retain the terminal session that belonged to its previous step. The task
  // projection owns that placement, so do not paint the new step as terminal.
  const taskIsNotStarted = taskState === "CREATED" || taskState === "TODO";
  if (terminalSessionStatus && !taskIsNotStarted) return terminalSessionStatus;

  if (cancellationPending) return "cancelling";
  if (taskState === "SCHEDULING") return "preparing";
  if (primarySessionState === "STARTING") return "starting";
  if (primarySessionState === "RUNNING") return "running";
  if (taskState === "WAITING_FOR_INPUT" || primarySessionState === "WAITING_FOR_INPUT") {
    return "waiting";
  }
  if (primarySessionState === "IDLE") return "idle_agent";
  if (taskState === "CREATED" || taskState === "TODO" || primarySessionState === "CREATED") {
    return "not_started";
  }
  return "idle";
}

export function deriveWorkflowStepProgress({
  stepId,
  currentStepId,
  movingToStepId,
  taskState,
  primarySessionId,
  primarySessionState,
  cancellationPending = false,
  agentLabel,
}: WorkflowStepProgressInput): WorkflowStepProgress {
  if (movingToStepId === stepId) {
    return {
      status: "moving",
      isPending: true,
      sessionId: null,
      agentLabel: null,
    };
  }

  if (stepId !== currentStepId) {
    return { status: "idle", isPending: false, sessionId: null, agentLabel: null };
  }

  const status = progressStatusForEvidence(taskState, primarySessionState, cancellationPending);

  return {
    status,
    isPending: status === "preparing" || status === "starting",
    sessionId: primarySessionId ?? null,
    agentLabel: agentLabel ?? null,
  };
}

function findTaskById(state: AppState, taskId: string | null | undefined) {
  if (!taskId) return null;
  const task = state.kanban.tasks.find((candidate) => candidate.id === taskId);
  if (task) return task;
  for (const snapshot of Object.values(state.kanbanMulti.snapshots)) {
    const snapshotTask = snapshot.tasks.find((candidate) => candidate.id === taskId);
    if (snapshotTask) return snapshotTask;
  }
  return null;
}

export function useWorkflowStepProgress({
  taskId,
  currentStepId,
  movingToStepId,
  taskProjection,
  preferTaskProjection = false,
  useSessionProjection = true,
}: {
  taskId?: string | null;
  currentStepId?: string | null;
  movingToStepId?: string | null;
  taskProjection?: WorkflowProgressTaskProjection | null;
  preferTaskProjection?: boolean;
  useSessionProjection?: boolean;
}) {
  const taskFromStore = useAppStore(useCallback((state) => findTaskById(state, taskId), [taskId]));
  const task =
    (preferTaskProjection
      ? (taskProjection ?? taskFromStore)
      : (taskFromStore ?? taskProjection)) ?? null;
  const projectedSessionIdValue = task ? projectedSessionId(task) : null;
  const primarySession = useAppStore(
    useCallback(
      (state) => {
        if (!useSessionProjection || !taskId) return null;
        if (projectedSessionIdValue) {
          const session = state.taskSessions.items[projectedSessionIdValue];
          return isSessionForTask(session, taskId) ? session : null;
        }
        const sessionsForTask = state.taskSessionsByTask.itemsByTaskId[taskId] ?? [];
        const candidates = new Map<string, WorkflowProgressSessionProjection>();
        for (const session of sessionsForTask) {
          if (session.task_id === taskId && session.is_primary) candidates.set(session.id, session);
        }
        for (const session of Object.values(state.taskSessions.items)) {
          if (session.task_id === taskId && session.is_primary) candidates.set(session.id, session);
        }
        return candidates.size === 1 ? (candidates.values().next().value ?? null) : null;
      },
      [projectedSessionIdValue, taskId, useSessionProjection],
    ),
  );
  const sessionsById = useMemo(
    () => (primarySession ? { [primarySession.id]: primarySession } : EMPTY_SESSIONS_BY_ID),
    [primarySession],
  );
  const sessionsForTask = useAppStore(
    useCallback(
      (state) =>
        useSessionProjection && taskId
          ? (state.taskSessionsByTask.itemsByTaskId[taskId] ?? EMPTY_SESSIONS_FOR_TASK)
          : EMPTY_SESSIONS_FOR_TASK,
      [taskId, useSessionProjection],
    ),
  );
  const agentProfiles = useAppStore((state) => state.agentProfiles.items);

  const evidence = useMemo(
    () =>
      resolveWorkflowProgressEvidence({
        task,
        sessionsById,
        sessionsForTask,
        agentProfiles,
        preferTaskProjection,
      }),
    [agentProfiles, preferTaskProjection, sessionsById, sessionsForTask, task],
  );

  const agentLabelsByProfileId = useMemo(
    () => Object.fromEntries(agentProfiles.map((profile) => [profile.id, profile.label])),
    [agentProfiles],
  );

  const progressByStepId = useMemo(() => {
    const stepIds = [currentStepId, movingToStepId].filter((stepId): stepId is string =>
      Boolean(stepId),
    );
    return Object.fromEntries(
      [...new Set(stepIds)].map((stepId) => [
        stepId,
        deriveWorkflowStepProgress({
          stepId,
          currentStepId,
          movingToStepId,
          taskState: task?.state,
          primarySessionId: evidence.sessionId,
          primarySessionState: evidence.sessionState,
          cancellationPending: evidence.cancellationPending,
          agentLabel: evidence.agentLabel,
        }),
      ]),
    ) as Record<string, WorkflowStepProgress>;
  }, [currentStepId, evidence, movingToStepId, task?.state]);

  return { progressByStepId, agentLabelsByProfileId };
}
