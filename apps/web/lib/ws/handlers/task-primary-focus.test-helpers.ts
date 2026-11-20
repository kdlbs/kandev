import { vi } from "vitest";
import { createAppStore, type AppState } from "@/lib/state/store";
import type { StoreApi } from "zustand";
import type { TaskSession, TaskSessionResponse, TaskSessionsResponse } from "@/lib/types/http";
import { registerTaskSessionHandlers } from "./agent-session";
import { registerTasksHandlers } from "./tasks";
import { makeMessage, makeTask } from "./tasks.test-helpers";

export const SESSION_STATE_CHANGED = "session.state_changed";
const SNAPSHOT_TIME = "2026-11-19T12:00:00Z";

export function response(
  state: TaskSession["state"],
  id = "old",
  taskId = "t1",
): TaskSessionResponse {
  return {
    session: {
      id,
      task_id: taskId,
      state,
      started_at: SNAPSHOT_TIME,
      updated_at: SNAPSHOT_TIME,
    } as TaskSession,
  };
}
export function deferred() {
  return Promise.withResolvers<TaskSessionResponse>();
}
export function selectedStore(initialState: TaskSession["state"] = "WAITING_FOR_INPUT") {
  const store = createAppStore({
    kanban: {
      workflowId: "wf1",
      steps: [],
      tasks: [
        {
          id: "t1",
          workflowId: "wf1",
          workflowStepId: "step1",
          // i18n-exempt: Deterministic task title in test fixture data.
          title: "Task",
          position: 0,
          primarySessionId: "old",
        },
      ],
    },
  });
  for (const [id, state] of [
    ["old", initialState],
    ["next", "WAITING_FOR_INPUT"],
    ["latest", "WAITING_FOR_INPUT"],
  ] as const) {
    store.getState().setTaskSession({
      id,
      task_id: "t1",
      state,
      started_at: SNAPSHOT_TIME,
      updated_at: "",
    } as TaskSession);
  }
  store.getState().setActiveSessionAuto("t1", "old");
  return store;
}
export function withSiblings(initialState: TaskSession["state"] = "WAITING_FOR_INPUT") {
  const store = selectedStore(initialState);
  const sessions = Object.values(store.getState().taskSessions.items).map((session) => ({
    ...session,
    started_at: SNAPSHOT_TIME,
    updated_at: SNAPSHOT_TIME,
  }));
  store.getState().setTaskSessionsForTask("t1", sessions, {});
  return store;
}
export function promote(store: StoreApi<AppState>, sessionId = "next") {
  registerTasksHandlers(store)["task.updated"]!(makeMessage(makeTask("t1", sessionId)));
}
export function fail(store: StoreApi<AppState>) {
  registerTaskSessionHandlers(store)[SESSION_STATE_CHANGED]!({
    id: "failed",
    type: "notification",
    action: SESSION_STATE_CHANGED,
    payload: { task_id: "t1", session_id: "old", new_state: "FAILED" },
  });
}
export function notifyState(
  store: StoreApi<AppState>,
  state: NonNullable<TaskSession["state"]>,
  updatedAt: string,
) {
  registerTaskSessionHandlers(store)[SESSION_STATE_CHANGED]!({
    id: "state",
    type: "notification",
    action: SESSION_STATE_CHANGED,
    payload: { task_id: "t1", session_id: "old", new_state: state, updated_at: updatedAt },
  });
}
export function notifySibling(store: StoreApi<AppState>) {
  registerTaskSessionHandlers(store)[SESSION_STATE_CHANGED]!({
    id: "new-session",
    type: "notification",
    action: SESSION_STATE_CHANGED,
    payload: { task_id: "t1", session_id: "latest", new_state: "WAITING_FOR_INPUT" },
  });
}
export function membership(...ids: string[]): TaskSessionsResponse {
  return {
    sessions: ids.map((id) => ({
      ...response("WAITING_FOR_INPUT", id).session,
      is_primary: id === ids[0],
    })),
    total: ids.length,
  };
}
// Resolved HTTP chains settle before assertions of no selection change.
export async function flushFocusWork(): Promise<void> {
  vi.useFakeTimers();
  try {
    await vi.advanceTimersByTimeAsync(0);
  } finally {
    vi.useRealTimers();
  }
}
