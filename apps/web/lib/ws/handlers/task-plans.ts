import type { StoreApi } from "zustand";
import type { QueryClient } from "@tanstack/react-query";
import type { AppState } from "@/lib/state/store";
import type { BackendMessageMap } from "@/lib/types/backend";
import type { WsHandlers } from "@/lib/ws/handlers/types";
import { qk } from "@/lib/query/keys";
import type { TaskPlanData } from "@/lib/query/query-options/session";

type PlanMessage = BackendMessageMap["task.plan.created"] | BackendMessageMap["task.plan.updated"];

/**
 * Task-plan WS handler. The plan + revisions server data is mirrored into the
 * TanStack Query cache by bridge/session.ts; this Zustand handler is retained
 * ONLY for the client-only "seen" indicator (lastSeenUpdatedAtByTaskId), which
 * stays in Zustand. It reads the previous plan from the TQ cache (the bridge
 * runs alongside this handler) to decide whether a user-authored write changed
 * the content.
 */
function handlePlanUpsert(store: StoreApi<AppState>, qc: QueryClient, message: PlanMessage) {
  const { task_id, content, created_by, updated_at } = message.payload;

  // User-authored writes mark the plan as seen — but only when the content
  // actually changed. The plan editor's auto-save on mount can emit a
  // user-authored update with unchanged content (TipTap markdown round-trip
  // normalises whitespace), which would otherwise wipe an unseen agent
  // indicator the moment the panel opens.
  if (created_by === "user") {
    const prev = qc.getQueryData<TaskPlanData>(qk.taskSession.plans(task_id));
    if (prev?.plan?.content !== content) {
      store.getState().markTaskPlanSeen(task_id, updated_at);
    }
  }
}

export function registerTaskPlansHandlers(store: StoreApi<AppState>, qc: QueryClient): WsHandlers {
  return {
    "task.plan.created": (message) => handlePlanUpsert(store, qc, message),
    "task.plan.updated": (message) => handlePlanUpsert(store, qc, message),
    "task.plan.deleted": (message) => {
      const { task_id } = message.payload;
      // Plan removed — mark seen so no stale indicator lingers on the deleted
      // task. The TQ cache is cleared by the bridge.
      store.getState().markTaskPlanSeen(task_id, "");
    },
    // task.plan.revision.created / task.plan.reverted carry no client-only
    // state — the bridge owns the revisions cache. Register no-ops so the
    // dispatcher doesn't warn about an unhandled type.
    "task.plan.revision.created": () => {},
    "task.plan.reverted": () => {},
  };
}
