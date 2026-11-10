import type { StoreApi } from "zustand";
import type { AppState } from "@/lib/state/store";
import {
  sessionId as toSessionId,
  taskId as toTaskId,
  type RuntimeStorageHealth,
  type TaskSession,
} from "@/lib/types/http";
import { mergeRuntimeStorageHealth } from "@/lib/session-delivery-storage";

export type RuntimeStorageChangedPayload = Record<string, unknown> & {
  task_id?: string;
  session_id?: string;
};

/** Apply one session-authorized advisory or pressure snapshot. */
export function applyRuntimeStorageHealth(
  store: StoreApi<AppState>,
  payload: RuntimeStorageChangedPayload,
): void {
  if (!payload?.task_id || !payload?.session_id) return;
  const taskId = toTaskId(payload.task_id);
  const sessionId = toSessionId(payload.session_id);
  const existing = store.getState().taskSessions.items[sessionId];
  if (!existing) return;
  if (existing.task_id && existing.task_id !== taskId) return;
  const merged = mergeRuntimeStorageHealth(
    existing.runtime_storage_health,
    payload as unknown as RuntimeStorageHealth,
  );
  if (!merged) return;
  store.getState().upsertTaskSessionFromEvent(taskId, {
    ...partialSession(existing),
    runtime_storage_health: merged,
  } as TaskSession);
}

function partialSession(existing: TaskSession): TaskSession {
  return {
    id: existing.id,
    task_id: existing.task_id,
    state: existing.state,
    started_at: existing.started_at ?? "",
    updated_at: existing.updated_at ?? "",
  } as TaskSession;
}

export function registerSessionDeliveryStorageHandlers(store: StoreApi<AppState>) {
  return {
    "task_session.runtime_storage_changed": (message: { payload: RuntimeStorageChangedPayload }) =>
      applyRuntimeStorageHealth(store, message.payload),
  };
}
