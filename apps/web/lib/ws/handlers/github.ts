import type { StoreApi } from "zustand";
import type { AppState } from "@/lib/state/store";
import type { WsHandlers } from "@/lib/ws/handlers/types";
import type {
  GitHubRateLimitUpdate,
  TaskCIAutomationOptions,
  TaskPR,
  TaskPRDeletedEvent,
} from "@/lib/types/github";

export function registerGitHubHandlers(store: StoreApi<AppState>): WsHandlers {
  return {
    "github.task_pr.updated": (message) => {
      const pr = message.payload as TaskPR;
      if (!pr.task_id || !pr.workspace_id) return;
      const state = store.getState();
      if (!state.workspaces.activeId || state.workspaces.activeId !== pr.workspace_id) return;
      store.getState().setTaskPR(pr.task_id, pr, {
        workspaceId: pr.workspace_id,
        workspaceContextGeneration: state.workspaceContextGeneration,
      });
    },
    "github.task_pr.deleted": (message) => {
      const deleted = message.payload as TaskPRDeletedEvent;
      if (deleted.task_id && deleted.association_id) {
        const state = store.getState();
        if (state.workspaces.activeId && state.workspaces.activeId !== deleted.workspace_id) return;
        const scope = state.workspaces.activeId
          ? {
              workspaceId: state.workspaces.activeId,
              workspaceContextGeneration: state.workspaceContextGeneration,
            }
          : undefined;
        store.getState().removeTaskPR(deleted.task_id, deleted.association_id, scope);
      }
    },
    "github.task_ci_options.updated": (message) => {
      const options = message.payload as TaskCIAutomationOptions;
      if (options.task_id) {
        store.getState().setTaskCIAutomationOptions(options.task_id, options);
      }
    },
    "github.rate_limit.updated": (message) => {
      const update = message.payload as GitHubRateLimitUpdate;
      if (update?.snapshots?.length) {
        store.getState().applyGitHubRateLimitUpdate(update);
      }
    },
  };
}
