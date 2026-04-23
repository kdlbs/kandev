import type { StoreApi } from "zustand";
import type { AppState } from "@/lib/state/store";
import type { BackendMessageMap } from "@/lib/types/backend";
import type { WsHandlers } from "@/lib/ws/handlers/types";
import { useDockviewStore } from "@/lib/state/dockview-store";
import type { TaskPlanRevision } from "@/lib/types/http";

type PlanMessage = BackendMessageMap["task.plan.created"] | BackendMessageMap["task.plan.updated"];
type RevisionMessage =
  | BackendMessageMap["task.plan.revision.created"]
  | BackendMessageMap["task.plan.reverted"];

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

  // Auto-open plan panel side-by-side with chat when agent writes a plan
  if (created_by === "agent" && task_id === store.getState().tasks.activeTaskId) {
    const dockview = useDockviewStore.getState();
    if (dockview.isRestoringLayout) return;
    if (dockview.api?.getPanel("plan")) return;

    const activeSessionId = store.getState().tasks.activeSessionId;
    if (!activeSessionId) return;

    dockview.addPlanPanel();
    store.getState().setActiveDocument(activeSessionId, { type: "plan", taskId: task_id });
  }
}

function handleRevisionPush(store: StoreApi<AppState>, message: RevisionMessage) {
  const p = message.payload;
  const rev: TaskPlanRevision = {
    id: p.id,
    task_id: p.task_id,
    revision_number: p.revision_number,
    title: p.title,
    author_kind: p.author_kind,
    author_name: p.author_name,
    revert_of_revision_id: p.revert_of_revision_id ?? null,
    created_at: p.created_at,
    updated_at: p.updated_at,
  };
  store.getState().upsertPlanRevision(p.task_id, rev);
}

export function registerTaskPlansHandlers(store: StoreApi<AppState>): WsHandlers {
  return {
    "task.plan.created": (message) => handlePlanUpsert(store, message),
    "task.plan.updated": (message) => handlePlanUpsert(store, message),
    "task.plan.deleted": (message) => {
      const { task_id } = message.payload;
      store.getState().setTaskPlan(task_id, null);
    },
    "task.plan.revision.created": (message) => handleRevisionPush(store, message),
    "task.plan.reverted": (message) => handleRevisionPush(store, message),
  };
}
