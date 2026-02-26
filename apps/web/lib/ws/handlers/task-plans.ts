import type { StoreApi } from "zustand";
import type { AppState } from "@/lib/state/store";
import type { BackendMessageMap } from "@/lib/types/backend";
import type { WsHandlers } from "@/lib/ws/handlers/types";
import { useDockviewStore } from "@/lib/state/dockview-store";

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

  // Auto-open plan panel when agent creates/updates a plan for the active task
  if (created_by === "agent" && task_id === store.getState().tasks.activeTaskId) {
    const dockview = useDockviewStore.getState();
    if (dockview.api?.getPanel("plan")) return;

    const activeSessionId = store.getState().tasks.activeSessionId;
    if (!activeSessionId) return;

    dockview.addPlanPanel();
    store.getState().setActiveDocument(activeSessionId, { type: "plan", taskId: task_id });
  }
}

export function registerTaskPlansHandlers(store: StoreApi<AppState>): WsHandlers {
  return {
    "task.plan.created": (message) => handlePlanUpsert(store, message),
    "task.plan.updated": (message) => handlePlanUpsert(store, message),
    "task.plan.deleted": (message) => {
      const { task_id } = message.payload;
      store.getState().setTaskPlan(task_id, null);
    },
  };
}
