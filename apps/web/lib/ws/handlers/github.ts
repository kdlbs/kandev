import type { StoreApi } from "zustand";
import type { AppState } from "@/lib/state/store";
import type { WsHandlers } from "@/lib/ws/handlers/types";
import type { TaskPR } from "@/lib/types/github";

export function registerGitHubHandlers(store: StoreApi<AppState>): WsHandlers {
  return {
    "github.task_pr.updated": (message) => {
      const pr = message.payload as TaskPR;
      if (pr.task_id) {
        store.getState().setTaskPR(pr.task_id, pr);
      }
    },
  };
}
