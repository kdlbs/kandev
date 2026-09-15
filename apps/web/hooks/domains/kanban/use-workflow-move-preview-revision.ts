"use client";

import { useCallback } from "react";
import { useAppStore } from "@/components/state-provider";
import type { AppState } from "@/lib/state/store";

type PreviewRevisionTask = AppState["kanban"]["tasks"][number];
type PreviewRevisionStep = AppState["kanban"]["steps"][number];
type PreviewRevisionSession = AppState["taskSessions"]["items"][string];

function matchingTasks(state: AppState, taskId: string | null | undefined) {
  if (!taskId) return [];
  const tasks: Array<{ source: string; task: PreviewRevisionTask }> = [];
  for (const task of state.kanban.tasks) {
    if (task.id === taskId) tasks.push({ source: "active", task });
  }
  for (const [workflowId, snapshot] of Object.entries(state.kanbanMulti.snapshots).sort(
    ([a], [b]) => a.localeCompare(b),
  )) {
    for (const task of snapshot.tasks) {
      if (task.id === taskId) tasks.push({ source: `snapshot:${workflowId}`, task });
    }
  }
  return tasks.map(({ source, task }) => ({ source, task }));
}

function matchingSteps(state: AppState, workflowIds: Set<string>, workflowStepIds: Set<string>) {
  if (workflowIds.size === 0 || workflowStepIds.size === 0) return [];
  const steps: Array<{ source: string; step: PreviewRevisionStep }> = [];
  if (state.kanban.workflowId && workflowIds.has(state.kanban.workflowId)) {
    for (const step of state.kanban.steps) {
      if (workflowStepIds.has(step.id)) steps.push({ source: "active", step });
    }
  }
  for (const [workflowId, snapshot] of Object.entries(state.kanbanMulti.snapshots).sort(
    ([a], [b]) => a.localeCompare(b),
  )) {
    if (!workflowIds.has(workflowId)) continue;
    for (const step of snapshot.steps) {
      if (workflowStepIds.has(step.id)) steps.push({ source: `snapshot:${workflowId}`, step });
    }
  }
  return steps.map(({ source, step }) => ({ source, step }));
}

function matchingSessions(state: AppState, taskId: string | null | undefined) {
  if (!taskId) return [];
  const sessions: Array<{ source: string; session: PreviewRevisionSession }> = [];
  for (const session of state.taskSessionsByTask.itemsByTaskId[taskId] ?? []) {
    sessions.push({ source: "task", session });
  }
  for (const session of Object.values(state.taskSessions.items)) {
    if (session.task_id === taskId) sessions.push({ source: "session", session });
  }
  return sessions
    .sort(
      (left, right) =>
        left.session.id.localeCompare(right.session.id) || left.source.localeCompare(right.source),
    )
    .map(({ source, session }) => ({ source, session }));
}

/**
 * Returns a serializable revision of the state that can change a move
 * prediction. It deliberately includes both active and background workflow
 * projections because a disclosure can remain open while either one receives
 * a WebSocket update.
 */
export function getWorkflowMovePreviewRevision(
  state: AppState,
  taskId: string | null | undefined,
  workflowId: string | null | undefined,
  workflowStepId: string | null | undefined,
): string {
  const tasks = matchingTasks(state, taskId);
  const workflowIds = new Set<string>();
  const workflowStepIds = new Set<string>();
  if (workflowId) workflowIds.add(workflowId);
  if (workflowStepId) workflowStepIds.add(workflowStepId);
  for (const { task } of tasks) {
    if (task.workflowId) workflowIds.add(task.workflowId);
    if (task.workflowStepId) workflowStepIds.add(task.workflowStepId);
  }
  const workflows = state.workflows.items
    .filter((workflow) => workflow.id === workflowId)
    .map((workflow) => ({ ...workflow }));
  const profiles = state.agentProfiles.items
    .slice()
    .sort((left, right) => left.id.localeCompare(right.id))
    .map((profile) => ({ ...profile }));
  const sessions = matchingSessions(state, taskId);
  const sessionModels = Array.from(new Set(sessions.map(({ session }) => session.id)))
    .sort()
    .map((sessionId) => ({
      session_id: sessionId,
      models: state.sessionModels?.bySessionId?.[sessionId] ?? null,
    }));

  return JSON.stringify({
    connection: state.connection.status,
    workspace_context_generation: state.workspaceContextGeneration,
    active_workflow_id: state.kanban.workflowId,
    workflow_id: workflowId ?? "",
    workflow_step_id: workflowStepId ?? "",
    workflows,
    steps: matchingSteps(state, workflowIds, workflowStepIds),
    tasks,
    sessions_loaded: taskId ? state.taskSessionsByTask.loadedByTaskId[taskId] === true : false,
    sessions,
    session_models: sessionModels,
    agent_profiles_version: state.agentProfiles.version,
    profiles,
  });
}

export function useWorkflowMovePreviewRevision(
  taskId: string | null | undefined,
  workflowId: string | null | undefined,
  workflowStepId: string | null | undefined,
): string {
  const selector = useCallback(
    (state: AppState) => getWorkflowMovePreviewRevision(state, taskId, workflowId, workflowStepId),
    [taskId, workflowId, workflowStepId],
  );
  return useAppStore(selector);
}
