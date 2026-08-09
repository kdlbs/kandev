import type { StoreApi } from "zustand";
import type { AppState } from "@/lib/state/store";
import { sessionId as toSessionId } from "@/lib/types/http";

/** Apply the backend-owned parked-on-background-work projection to the addressed session. */
export function applyParkedChanged(
  store: StoreApi<AppState>,
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  payload: any,
): void {
  if (!payload?.session_id || typeof payload.parked_on_background_work !== "boolean") return;
  const sessionId = toSessionId(payload.session_id);
  const existing = store.getState().taskSessions.items[sessionId];
  if (!existing) return;
  store.getState().upsertTaskSessionFromEvent(existing.task_id, {
    id: sessionId,
    task_id: existing.task_id,
    state: existing.state,
    started_at: existing.started_at ?? "",
    updated_at: existing.updated_at ?? "",
    parked_on_background_work: payload.parked_on_background_work,
    parked_epoch: payload.parked_epoch,
    parked_revision: payload.parked_revision,
  });
}
