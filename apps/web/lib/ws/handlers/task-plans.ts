import type { StoreApi } from "zustand";
import type { AppState } from "@/lib/state/store";
import type { BackendMessageMap } from "@/lib/types/backend";
import type { WsHandlers } from "@/lib/ws/handlers/types";

type PlanMessage = BackendMessageMap["task.plan.created"] | BackendMessageMap["task.plan.updated"];

function handlePlanUpsert(store: StoreApi<AppState>, message: PlanMessage) {
  const { task_id, id, title, content, created_by, created_at, updated_at } = message.payload;
  store.getState().setTaskPlan(task_id, {
    id,
    task_id,
    title,
    content,
    created_by,
    created_at,
    updated_at,
  });

  console.warn("[task-plans-ws] handlePlanUpsert", {
    type: message.type,
    task_id,
    created_by,
    updated_at,
  });

  // User-authored writes: mark seen so the indicator doesn't fire. Panel
  // reveal is handled reactively by usePlanPanelAutoOpen when an agent
  // writes a new version the user hasn't seen.
  if (created_by === "user") {
    console.warn("[task-plans-ws] markSeen (user-authored)", { task_id });
    store.getState().markTaskPlanSeen(task_id);
  }
}

export function registerTaskPlansHandlers(store: StoreApi<AppState>): WsHandlers {
  return {
    "task.plan.created": (message) => handlePlanUpsert(store, message),
    "task.plan.updated": (message) => handlePlanUpsert(store, message),
    "task.plan.deleted": (message) => {
      const { task_id } = message.payload;
      console.warn("[task-plans-ws] markSeen on delete", { task_id });
      store.getState().setTaskPlan(task_id, null);
      store.getState().markTaskPlanSeen(task_id);
    },
  };
}
