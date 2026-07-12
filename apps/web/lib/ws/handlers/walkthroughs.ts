import type { StoreApi } from "zustand";
import type { AppState } from "@/lib/state/store";
import type { BackendMessageMap } from "@/lib/types/backend";
import type { WsHandlers } from "@/lib/ws/handlers/types";

type WalkthroughMessage =
  | BackendMessageMap["task.walkthrough.created"]
  | BackendMessageMap["task.walkthrough.updated"];

function handleWalkthroughUpsert(store: StoreApi<AppState>, message: WalkthroughMessage) {
  const { task_id, id, title, steps, created_by, created_at, updated_at } = message.payload;
  store.getState().setWalkthrough(task_id, {
    id,
    task_id,
    title,
    steps,
    created_by,
    created_at,
    updated_at,
  });
}

export function registerWalkthroughsHandlers(store: StoreApi<AppState>): WsHandlers {
  return {
    "task.walkthrough.created": (message) => handleWalkthroughUpsert(store, message),
    "task.walkthrough.updated": (message) => handleWalkthroughUpsert(store, message),
    "task.walkthrough.deleted": (message) => {
      store.getState().setWalkthrough(message.payload.task_id, null);
      store.getState().markWalkthroughSeen(message.payload.task_id);
    },
  };
}
