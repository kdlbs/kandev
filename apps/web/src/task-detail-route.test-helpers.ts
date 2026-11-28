import { expect } from "vitest";
import type { StoreApi } from "zustand";
import type { AppState } from "@/lib/state/store";
import type { TaskSession } from "@/lib/types/http";

export function commitWorkflowRecipient(store: StoreApi<AppState>, taskId: string, target: string) {
  const entryIdentity = "entry";
  const destinationStepId = "review";
  const requestId = store.getState().beginWorkflowSessionFocus({
    taskId,
    workflowId: "workflow-1",
    destinationStepId,
    presentationToken: 1,
    navigationRevision: store.getState().taskRemoval.navigationRevision,
  })!;
  store.getState().bindWorkflowSessionFocus({ requestId, presentationToken: 1, entryIdentity });
  store.getState().reconcileWorkflowSessionFocus(taskId, {
    workflowStepId: destinationStepId,
    updatedAt: "2026-11-20T12:00:00Z",
    entryIdentity,
    metadata: {
      workflow_session_route: {
        operation_id: "move",
        destination_step_id: destinationStepId,
        entry_identity: entryIdentity,
        destination_session_id: target,
        phase: "committed",
      },
    },
  });
  expect(store.getState().tasks.activeSessionId).toBe(target);
}

export function taskRouteSelectionState(
  taskId: string,
  activeSessionId: string,
  sessions: TaskSession[],
): Partial<AppState> {
  return {
    tasks: {
      activeTaskId: taskId,
      activeSessionId,
      pinnedSessionId: null,
      lastSessionByTaskId: {},
      resumeSkippedSessionIds: {},
    },
    taskSessions: {
      items: Object.fromEntries(sessions.map((session) => [session.id, session])),
    } as AppState["taskSessions"],
  };
}
