import type { StoreApi } from "zustand";
import type { AppState } from "@/lib/state/store";
import type { BackendMessageMap } from "@/lib/types/backend";
import type { WsHandlers } from "@/lib/ws/handlers/types";

type PlanMessage = BackendMessageMap["task.plan.created"] | BackendMessageMap["task.plan.updated"];

function handlePlanUpsert(store: StoreApi<AppState>, message: PlanMessage) {
  const { task_id, id, title, content, created_by, created_at, updated_at } = message.payload;
  const prevPlan = store.getState().taskPlans.byTaskId[task_id];
  store.getState().setTaskPlan(task_id, {
    id,
    task_id,
    title,
    content,
    created_by,
    created_at,
    updated_at,
  });

  // User-authored writes mark the plan as seen — but only when the content
  // actually changed. The plan editor's auto-save on mount can emit a
  // user-authored update with unchanged content (TipTap markdown round-trip
  // normalises whitespace), which would otherwise wipe an unseen agent
  // indicator the moment the panel opens.
  if (created_by === "user" && prevPlan?.content !== content) {
    store.getState().markTaskPlanSeen(task_id);
  }
}

export function registerTaskPlansHandlers(store: StoreApi<AppState>): WsHandlers {
  return {
    "task.plan.created": (message) => handlePlanUpsert(store, message),
    "task.plan.updated": (message) => handlePlanUpsert(store, message),
    "task.plan.deleted": (message) => {
      const { task_id } = message.payload;
      // Intentionally NOT clearTaskPlan: setTaskPlan(null) preserves
      // loadedByTaskId[taskId] = true so useTaskPlan doesn't see !isLoaded
      // and refetch a plan that was just deleted. clearTaskPlan would drop
      // that flag and trigger a wasted HTTP round-trip.
      store.getState().setTaskPlan(task_id, null);
      store.getState().markTaskPlanSeen(task_id);
    },
  };
}
