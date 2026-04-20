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

  // Mark user-authored writes as seen regardless of active task so the
  // indicator never fires spuriously if the user switches tasks mid-save.
  if (created_by === "user") {
    store.getState().markTaskPlanSeen(task_id);
    return;
  }

  if (task_id !== store.getState().tasks.activeTaskId) return;

  // Agent-authored: reveal plan panel quietly in the center group so the user
  // sees the indicator without losing focus. If the panel is already open the
  // indicator on the tab drives the UI.
  const dockview = useDockviewStore.getState();
  if (dockview.isRestoringLayout) return;
  if (dockview.api?.getPanel("plan")) return;

  dockview.addPlanPanel({ quiet: true, inCenter: true });
}

export function registerTaskPlansHandlers(store: StoreApi<AppState>): WsHandlers {
  return {
    "task.plan.created": (message) => handlePlanUpsert(store, message),
    "task.plan.updated": (message) => handlePlanUpsert(store, message),
    "task.plan.deleted": (message) => {
      const { task_id } = message.payload;
      store.getState().setTaskPlan(task_id, null);
      store.getState().markTaskPlanSeen(task_id);
    },
  };
}
