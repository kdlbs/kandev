import type { StoreApi } from "zustand";
import type { AppState } from "@/lib/state/store";
import {
  sessionId as toSessionId,
  taskId as toTaskId,
  type ForegroundActivity,
  type TaskSession,
} from "@/lib/types/http";
import type { TaskSessionActivityChangedPayload } from "@/lib/types/session-events";

/**
 * `session.activity_changed` carries two distinct payload shapes on the same
 * wire event: the ADR-0049 foreground-activity flip (always sends
 * `foreground_activity`, explicit null included, to clear a stale busy
 * signal) and task-05's parked-on-background-work transition (sends only
 * `parked_on_background_work`/`revision`/`parked_epoch` — no
 * `foreground_activity` key at all). Unconditionally defaulting a missing
 * `foreground_activity` to null would clobber a live busy signal on every
 * parked-only event, so this key is only applied when the payload actually
 * carries it.
 */
function pickForegroundActivity(
  payload: TaskSessionActivityChangedPayload,
  existing: TaskSession,
): ForegroundActivity | null | undefined {
  return payload.foreground_activity !== undefined
    ? payload.foreground_activity
    : existing.foreground_activity;
}

function pickParkedOnBackgroundWork(
  payload: TaskSessionActivityChangedPayload,
  existing: TaskSession,
): boolean | undefined {
  return payload.parked_on_background_work !== undefined
    ? payload.parked_on_background_work
    : existing.parked_on_background_work;
}

function pickParkedRevision(
  payload: TaskSessionActivityChangedPayload,
  existing: TaskSession,
): number | undefined {
  return payload.revision !== undefined ? payload.revision : existing.revision;
}

function pickParkedEpoch(
  payload: TaskSessionActivityChangedPayload,
  existing: TaskSession,
): number | undefined {
  return payload.parked_epoch !== undefined ? payload.parked_epoch : existing.parked_epoch;
}

function pickActiveSubagentCount(
  payload: TaskSessionActivityChangedPayload,
  existing: TaskSession,
): number {
  return payload.active_subagent_count !== undefined
    ? payload.active_subagent_count
    : (existing.active_subagent_count ?? 0);
}

function pickSupportsSteering(
  payload: TaskSessionActivityChangedPayload,
  existing: TaskSession,
): boolean | undefined {
  return payload.supports_steering !== undefined
    ? payload.supports_steering
    : existing.supports_steering;
}

/** Apply a fine-grained busy-substate flip (ADR-0049) or a parked-on-background-work
 *  transition (spec: docs/specs/disambiguate-waiting/spec.md) — both share this wire
 *  event. Annotates the existing session row so the composer gate, status indicator,
 *  and parked affordance update; does nothing until the row exists (state_changed
 *  seeds it first). */
export function applyForegroundActivity(
  store: StoreApi<AppState>,
  payload: TaskSessionActivityChangedPayload,
): void {
  if (!payload?.task_id || !payload?.session_id) return;
  const taskId = toTaskId(payload.task_id);
  const sessionId = toSessionId(payload.session_id);
  const existing = store.getState().taskSessions.items[sessionId];
  if (!existing) return;
  // Detached work can outlive the foreground turn, whose coarse state is then
  // WAITING_FOR_INPUT. Terminal/parked sessions reject delayed activity frames;
  // their execution teardown owns the final clear.
  if (existing.state !== "RUNNING" && existing.state !== "WAITING_FOR_INPUT") return;
  if (existing.task_id && existing.task_id !== taskId) return;
  store.getState().upsertTaskSessionFromEvent(taskId, {
    id: sessionId,
    task_id: taskId,
    state: existing.state,
    cancellation_pending: existing.cancellation_pending,
    cancellation_revision: existing.cancellation_revision,
    started_at: existing.started_at ?? "",
    updated_at: existing.updated_at ?? "",
    foreground_activity: pickForegroundActivity(payload, existing),
    active_subagent_count: pickActiveSubagentCount(payload, existing),
    supports_steering: pickSupportsSteering(payload, existing),
    parked_on_background_work: pickParkedOnBackgroundWork(payload, existing),
    revision: pickParkedRevision(payload, existing),
    parked_epoch: pickParkedEpoch(payload, existing),
  });
}

/** Apply the backend-owned cancellation projection to the addressed session. */
export function applyCancellationPending(
  store: StoreApi<AppState>,
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  payload: any,
): void {
  if (
    !payload?.session_id ||
    typeof payload.cancellation_pending !== "boolean" ||
    typeof payload.cancellation_revision !== "number"
  )
    return;
  const sessionId = toSessionId(payload.session_id);
  const existing = store.getState().taskSessions.items[sessionId];
  if (!existing) return;
  store.getState().upsertTaskSessionFromEvent(existing.task_id, {
    id: sessionId,
    task_id: existing.task_id,
    state: existing.state,
    started_at: existing.started_at ?? "",
    updated_at: existing.updated_at ?? "",
    cancellation_pending: payload.cancellation_pending,
    cancellation_revision: payload.cancellation_revision,
  });
}
