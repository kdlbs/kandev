import type { StoreApi } from "zustand";
import type { AppState } from "@/lib/state/store";
import type { BackendMessageMap } from "@/lib/types/backend";
import type { WsHandlers } from "@/lib/ws/handlers/types";
import type { TaskPlanRevision } from "@/lib/types/http";

type PlanMessage = BackendMessageMap["task.plan.created"] | BackendMessageMap["task.plan.updated"];
type RevisionMessage =
  | BackendMessageMap["task.plan.revision.created"]
  | BackendMessageMap["task.plan.reverted"];

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
      // Intentionally NOT clearTaskPlan: setTaskPlan(null) preserves
      // loadedByTaskId[taskId] = true so useTaskPlan doesn't see !isLoaded
      // and refetch a plan that was just deleted. clearTaskPlan would drop
      // that flag and trigger a wasted HTTP round-trip.
      store.getState().setTaskPlan(task_id, null);
      store.getState().markTaskPlanSeen(task_id);
    },
    "task.plan.revision.created": (message) => handleRevisionPush(store, message),
    // `task.plan.reverted` is published alongside `task.plan.revision.created`
    // for the same row by the backend RevertPlan path. Re-running the upsert
    // on this event would be a no-op against the list (same id, same data)
    // but would needlessly evict the revisionContentCache entry and trigger
    // an extra Zustand update. Treat this event as a notification-only signal
    // — register no-op so the dispatcher doesn't warn about an unhandled type.
    "task.plan.reverted": () => {},
  };
}
